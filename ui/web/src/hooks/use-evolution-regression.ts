import { useCallback } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "@/stores/use-toast-store";
import { useHttp } from "@/hooks/use-ws";
import { queryKeys } from "@/lib/query-keys";
import type { EvolutionRegressionRun } from "@/types/evolution";

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
    mutationFn: (input?: { scope?: string; suggestion_id?: string }) =>
      http.post<EvolutionRegressionRun>(`/v1/agents/${agentId}/evolution/regression-tests/run`, input ?? {}),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey });
      queryClient.invalidateQueries({ queryKey: queryKeys.evolution.audit(agentId, { limit: 50 }) });
    },
  });

  const runRegression = useCallback(async () => {
    if (!agentId) return null;
    const result = await mutation.mutateAsync({ scope: "agent_safety" });
    if (result.status === "passed") {
      toast.success("沙箱回归测试通过");
    } else {
      toast.error("沙箱回归测试未通过");
    }
    return result;
  }, [agentId, mutation]);

  return {
    runs: data ?? [],
    latestRun: data?.[0] ?? null,
    loading: isLoading,
    running: mutation.isPending,
    runRegression,
  };
}
