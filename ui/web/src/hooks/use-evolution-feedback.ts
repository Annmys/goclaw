import { useCallback, useMemo } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "@/stores/use-toast-store";
import { useHttp } from "@/hooks/use-ws";
import { queryKeys } from "@/lib/query-keys";
import type { CreateEvolutionFeedbackInput, EvolutionFeedback } from "@/types/evolution";

export function useEvolutionFeedback(agentId: string, timeRange = "30d") {
  const http = useHttp();
  const queryClient = useQueryClient();

  const since = useMemo(() => {
    const d = new Date();
    d.setDate(d.getDate() - (timeRange === "90d" ? 90 : timeRange === "7d" ? 7 : 30));
    d.setHours(0, 0, 0, 0);
    return d.toISOString();
  }, [timeRange]);

  const queryKey = queryKeys.evolution.feedback(agentId, { timeRange });
  const { data, isLoading } = useQuery({
    queryKey,
    queryFn: () =>
      http.get<EvolutionFeedback[]>(`/v1/agents/${agentId}/evolution/feedback`, {
        since,
        limit: "100",
      }),
    enabled: !!agentId,
  });

  const mutation = useMutation({
    mutationFn: (input: CreateEvolutionFeedbackInput) =>
      http.post(`/v1/agents/${agentId}/evolution/feedback`, input),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey });
      queryClient.invalidateQueries({ queryKey: queryKeys.evolution.suggestions(agentId, { status: "" }) });
      queryClient.invalidateQueries({ queryKey: queryKeys.evolution.audit(agentId, { limit: 50 }) });
    },
  });

  const submitFeedback = useCallback(
    async (input: CreateEvolutionFeedbackInput, options?: { silent?: boolean }) => {
      if (!agentId) return;
      try {
        await mutation.mutateAsync(input);
        if (!options?.silent) toast.success("反馈已记录");
      } catch (error) {
        console.error("evolution feedback failed", error);
        if (!options?.silent) toast.error("反馈记录失败");
        throw error;
      }
    },
    [agentId, mutation],
  );

  return {
    feedback: data ?? [],
    loading: isLoading,
    submitting: mutation.isPending,
    submitFeedback,
  };
}
