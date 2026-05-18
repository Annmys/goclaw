import { useCallback } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "@/stores/use-toast-store";
import { useHttp } from "@/hooks/use-ws";
import { queryKeys } from "@/lib/query-keys";
import type { EvolutionSuggestion } from "@/types/evolution";

export function useEvolutionSuggestions(agentId: string, status?: string) {
  const http = useHttp();
  const queryClient = useQueryClient();

  const params: Record<string, string> = { limit: "100" };
  if (status) params.status = status;

  const { data, isLoading } = useQuery({
    queryKey: queryKeys.evolution.suggestions(agentId, { status: status ?? "" }),
    queryFn: () => http.get<EvolutionSuggestion[]>(`/v1/agents/${agentId}/evolution/suggestions`, params),
    enabled: !!agentId,
  });

  const analyzeMutation = useMutation({
    mutationFn: () => http.post<{ status: string; created: number; items: EvolutionSuggestion[] }>(
      `/v1/agents/${agentId}/evolution/suggestions/analyze`,
      {},
    ),
  });

  const analyzeNow = useCallback(async () => {
    if (!agentId) return null;
    try {
      const result = await analyzeMutation.mutateAsync();
      queryClient.invalidateQueries({
        queryKey: queryKeys.evolution.suggestions(agentId, { status: status ?? "" }),
      });
      queryClient.invalidateQueries({ queryKey: queryKeys.evolution.audit(agentId, { limit: 50 }) });
      toast.success(result.created > 0 ? `已生成 ${result.created} 条待审建议` : "已完成分析，暂无新建议");
      return result;
    } catch (error) {
      console.error("evolution suggestion analysis failed", error);
      toast.error(error instanceof Error ? error.message : "建议分析失败");
      throw error;
    }
  }, [agentId, analyzeMutation, queryClient, status]);

  const updateStatus = useCallback(
    async (suggestionId: string, newStatus: "approved" | "rejected" | "rolled_back") => {
      try {
        await http.patch(`/v1/agents/${agentId}/evolution/suggestions/${suggestionId}`, {
          status: newStatus,
        });
        queryClient.invalidateQueries({
          queryKey: queryKeys.evolution.suggestions(agentId, { status: status ?? "" }),
        });
        queryClient.invalidateQueries({ queryKey: queryKeys.evolution.audit(agentId, { limit: 50 }) });
        queryClient.invalidateQueries({ queryKey: queryKeys.evolution.regression(agentId, { limit: 20 }) });
        const label = newStatus === "approved" ? "批准" : newStatus === "rejected" ? "拒绝" : "回滚";
        toast.success(`建议已${label}`);
      } catch (error) {
        console.error("evolution suggestion update failed", error);
        toast.error(error instanceof Error ? error.message : "建议状态更新失败");
        throw error;
      }
    },
    [http, agentId, status, queryClient],
  );

  return {
    suggestions: data ?? [],
    loading: isLoading,
    analyzing: analyzeMutation.isPending,
    analyzeNow,
    updateStatus,
  };
}
