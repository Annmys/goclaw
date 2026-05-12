import { useCallback } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "@/stores/use-toast-store";
import { useHttp } from "@/hooks/use-ws";
import { queryKeys } from "@/lib/query-keys";
import type { EvolutionRegressionRun } from "@/types/evolution";

export type EvolutionRegressionScope =
  | "agent_safety"
  | "core_skill_smoke"
  | "business_workflow_smoke"
  | "business_output_golden";

export function useEvolutionRegression(agentId: string) {
  const http = useHttp();
  const queryClient = useQueryClient();
  const queryKey = queryKeys.evolution.regression(agentId, { limit: 20 });

  const { data, isLoading } = useQuery({
    queryKey,
    queryFn: () =>
      http.get<EvolutionRegressionRun[]>(`/v1/agents/${agentId}/evolution/regression-tests`, {
        limit: "20",
      }),
    enabled: !!agentId,
  });

  const mutation = useMutation({
    mutationFn: (input?: { scope?: EvolutionRegressionScope; suggestion_id?: string }) =>
      http.post<EvolutionRegressionRun>(`/v1/agents/${agentId}/evolution/regression-tests/run`, input ?? {}),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey });
      queryClient.invalidateQueries({ queryKey: queryKeys.evolution.audit(agentId, { limit: 50 }) });
    },
  });

  const runRegression = useCallback(
    async (scope: EvolutionRegressionScope = "agent_safety") => {
      if (!agentId) return null;
      const result = await mutation.mutateAsync({ scope });
      const labelByScope: Record<EvolutionRegressionScope, string> = {
        agent_safety: "Agent 安全回归",
        core_skill_smoke: "核心业务 skill 冒烟回归",
        business_workflow_smoke: "业务工作流依赖回归",
        business_output_golden: "业务输出 golden 回归",
      };
      const label = labelByScope[scope];
      if (result.status === "passed") {
        toast.success(`${label}通过`);
      } else {
        toast.error(`${label}未通过`);
      }
      return result;
    },
    [agentId, mutation],
  );

  return {
    runs: data ?? [],
    latestRun: data?.[0] ?? null,
    loading: isLoading,
    running: mutation.isPending,
    runRegression,
  };
}
