import { useCallback } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useHttp } from "@/hooks/use-ws";
import { useAuthStore } from "@/stores/use-auth-store";
import type {
  WorkflowDefinition,
  WorkflowMatchResult,
  WorkflowFeedbackRequest,
  WorkflowMessage,
  WorkflowRun,
} from "@/types/workflow";

const workflowKeys = {
  definitions: (tenantId: string, userId: string) => ["workflow", "definitions", tenantId, userId] as const,
  runs: (tenantId: string, userId: string) => ["workflow", "runs", tenantId, userId] as const,
  messages: (tenantId: string, userId: string) => ["workflow", "messages", tenantId, userId] as const,
};

export interface StartWorkflowInput {
  workflow_id: string;
  intent?: string;
  file_name?: string;
  file_type?: string;
  input?: Record<string, unknown>;
}

export interface WorkflowUploadResponse {
  path: string;
  mime_type: string;
  filename: string;
}

export interface AppendWorkflowMessageInput {
  role: "user" | "assistant";
  content: string;
  files?: string[];
  run_id?: string;
  kind?: "chat" | "workflow";
}

export function useWorkflow() {
  const http = useHttp();
  const queryClient = useQueryClient();
  const tenantId = useAuthStore((s) => s.tenantId);
  const userId = useAuthStore((s) => s.userId);
  const definitionsKey = workflowKeys.definitions(tenantId, userId);
  const runsKey = workflowKeys.runs(tenantId, userId);
  const messagesKey = workflowKeys.messages(tenantId, userId);

  const definitionsQuery = useQuery({
    queryKey: definitionsKey,
    queryFn: async () => {
      const res = await http.get<{ workflows: WorkflowDefinition[] }>("/v1/workflows");
      return res.workflows ?? [];
    },
    staleTime: 60_000,
  });

  const runsQuery = useQuery({
    queryKey: runsKey,
    queryFn: async () => {
      const res = await http.get<{ runs: WorkflowRun[] }>("/v1/workflow-runs");
      return res.runs ?? [];
    },
    staleTime: 10_000,
  });

  const messagesQuery = useQuery({
    queryKey: messagesKey,
    queryFn: async () => {
      const res = await http.get<{ messages: WorkflowMessage[] }>("/v1/workflow-messages");
      return res.messages ?? [];
    },
    staleTime: 5_000,
  });

  const refresh = useCallback(async () => {
    await Promise.all([
      queryClient.invalidateQueries({ queryKey: definitionsKey }),
      queryClient.invalidateQueries({ queryKey: runsKey }),
      queryClient.invalidateQueries({ queryKey: messagesKey }),
    ]);
  }, [definitionsKey, messagesKey, queryClient, runsKey]);

  const matchWorkflow = useCallback(
    (body: { intent?: string; file_name?: string; file_type?: string; workflow_id?: string }) =>
      http.post<WorkflowMatchResult>("/v1/workflows/match", body),
    [http],
  );

  const startRun = useCallback(
    async (body: StartWorkflowInput) => {
      const res = await http.post<{ run: WorkflowRun }>("/v1/workflow-runs", body);
      await queryClient.invalidateQueries({ queryKey: runsKey });
      await queryClient.invalidateQueries({ queryKey: messagesKey });
      return res.run;
    },
    [http, messagesKey, queryClient, runsKey],
  );

  const appendMessage = useCallback(
    async (body: AppendWorkflowMessageInput) => {
      const res = await http.post<{ message: WorkflowMessage }>("/v1/workflow-messages", body);
      await queryClient.invalidateQueries({ queryKey: messagesKey });
      return res.message;
    },
    [http, messagesKey, queryClient],
  );

  const uploadFile = useCallback(
    (file: File) => {
      const fd = new FormData();
      fd.append("file", file);
      return http.upload<WorkflowUploadResponse>("/v1/media/upload", fd);
    },
    [http],
  );

  const resumeRun = useCallback(
    async (runId: string, fields: Record<string, unknown>) => {
      const res = await http.post<{ run: WorkflowRun }>(`/v1/workflow-runs/${runId}/resume`, { fields });
      await queryClient.invalidateQueries({ queryKey: runsKey });
      await queryClient.invalidateQueries({ queryKey: messagesKey });
      return res.run;
    },
    [http, messagesKey, queryClient, runsKey],
  );

  const submitFeedback = useCallback(
    async (body: WorkflowFeedbackRequest) => {
      const res = await http.post<{ ok: boolean }>("/v1/workflow-feedback", body);
      await queryClient.invalidateQueries({ queryKey: runsKey });
      await queryClient.invalidateQueries({ queryKey: messagesKey });
      return res;
    },
    [http, messagesKey, queryClient, runsKey],
  );

  return {
    definitions: definitionsQuery.data ?? [],
    runs: runsQuery.data ?? [],
    messages: messagesQuery.data ?? [],
    loading: definitionsQuery.isLoading || runsQuery.isLoading || messagesQuery.isLoading,
    fetching: definitionsQuery.isFetching || runsQuery.isFetching || messagesQuery.isFetching,
    refresh,
    matchWorkflow,
    startRun,
    appendMessage,
    uploadFile,
    resumeRun,
    submitFeedback,
  };
}
