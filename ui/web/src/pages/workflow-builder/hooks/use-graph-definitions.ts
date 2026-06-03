import { useCallback } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useHttp } from "@/hooks/use-ws";
import { useAuthStore } from "@/stores/use-auth-store";
import type { GraphDefinition, WorkflowGraph } from "@/types/workflow-graph";
import type { WorkflowRun } from "@/types/workflow";

const keys = {
  definitions: (tenantId: string, userId: string) => ["workflow-graph", "definitions", tenantId, userId] as const,
  definition: (id: string) => ["workflow-graph", "definition", id] as const,
};

export interface SaveGraphDefinitionInput {
  id?: string;
  name: string;
  description?: string;
  graph: WorkflowGraph;
  active?: boolean;
}

// useGraphDefinitions wraps the sim-style graph definition CRUD + run endpoints
// added on the backend (/v1/workflow-definitions...). It reuses goclaw's auth
// context (tenant/user) via the shared http client.
export function useGraphDefinitions() {
  const http = useHttp();
  const queryClient = useQueryClient();
  const tenantId = useAuthStore((s) => s.tenantId);
  const userId = useAuthStore((s) => s.userId);
  const listKey = keys.definitions(tenantId, userId);

  const definitionsQuery = useQuery({
    queryKey: listKey,
    queryFn: async () => {
      const res = await http.get<{ definitions: GraphDefinition[] }>("/v1/workflow-definitions");
      return res.definitions ?? [];
    },
  });

  const getDefinition = useCallback(
    async (id: string): Promise<GraphDefinition> => {
      const res = await http.get<{ definition: GraphDefinition }>(`/v1/workflow-definitions/${id}`);
      return res.definition;
    },
    [http],
  );

  const saveDefinition = useCallback(
    async (input: SaveGraphDefinitionInput): Promise<GraphDefinition> => {
      const body = {
        name: input.name,
        description: input.description ?? "",
        graph: input.graph,
        active: input.active ?? true,
      };
      const res = input.id
        ? await http.put<{ definition: GraphDefinition }>(`/v1/workflow-definitions/${input.id}`, body)
        : await http.post<{ definition: GraphDefinition }>("/v1/workflow-definitions", body);
      await queryClient.invalidateQueries({ queryKey: listKey });
      return res.definition;
    },
    [http, queryClient, listKey],
  );

  const deleteDefinition = useCallback(
    async (id: string): Promise<void> => {
      await http.delete(`/v1/workflow-definitions/${id}`);
      await queryClient.invalidateQueries({ queryKey: listKey });
    },
    [http, queryClient, listKey],
  );

  const runDefinition = useCallback(
    async (id: string, input?: Record<string, unknown>): Promise<WorkflowRun> => {
      const res = await http.post<{ run: WorkflowRun; error?: string }>(
        `/v1/workflow-definitions/${id}/run`,
        { input: input ?? {} },
      );
      return res.run;
    },
    [http],
  );

  // generateGraph asks the backend AI copilot (goclaw agent) to author or edit a
  // workflow graph from a natural-language prompt. `current` enables incremental
  // edits of the on-canvas graph.
  const generateGraph = useCallback(
    async (prompt: string, current?: WorkflowGraph): Promise<{ graph: WorkflowGraph; explanation: string }> => {
      return http.post<{ graph: WorkflowGraph; explanation: string }>("/v1/workflow-generate", {
        prompt,
        current,
      });
    },
    [http],
  );

  return {
    definitions: definitionsQuery.data ?? [],
    isLoading: definitionsQuery.isLoading,
    refetch: definitionsQuery.refetch,
    getDefinition,
    saveDefinition,
    deleteDefinition,
    runDefinition,
    generateGraph,
  };
}
