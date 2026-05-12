import { useQuery } from "@tanstack/react-query";
import { useHttp } from "@/hooks/use-ws";
import { queryKeys } from "@/lib/query-keys";
import type { EvolutionAuditEvent } from "@/types/evolution";

export function useEvolutionAudit(agentId: string) {
  const http = useHttp();
  const queryKey = queryKeys.evolution.audit(agentId, { limit: 50 });

  const { data, isLoading } = useQuery({
    queryKey,
    queryFn: () =>
      http.get<EvolutionAuditEvent[]>(`/v1/agents/${agentId}/evolution/audit`, {
        limit: "50",
      }),
    enabled: !!agentId,
  });

  return {
    events: data ?? [],
    loading: isLoading,
  };
}
