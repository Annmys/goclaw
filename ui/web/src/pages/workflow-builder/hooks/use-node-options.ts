import { useQuery } from "@tanstack/react-query";
import { useHttp } from "@/hooks/use-ws";

export interface AgentOption {
  id: string; // agent_key
  label: string;
}

export interface ToolOption {
  name: string;
  label: string;
}

// useNodeOptions fetches the real agents and builtin tools available in goclaw,
// so node config dropdowns offer concrete choices instead of free text.
export function useNodeOptions() {
  const http = useHttp();

  const agentsQuery = useQuery({
    queryKey: ["workflow-node-options", "agents"],
    queryFn: async (): Promise<AgentOption[]> => {
      const res = await http.get<{ agents: Array<Record<string, unknown>> }>("/v1/agents");
      return (res.agents ?? []).map((a) => ({
        id: String(a.agent_key ?? a.id ?? ""),
        label: String(a.display_name || a.agent_key || a.id || "未命名"),
      }));
    },
    staleTime: 60_000,
  });

  const toolsQuery = useQuery({
    queryKey: ["workflow-node-options", "tools"],
    queryFn: async (): Promise<ToolOption[]> => {
      const res = await http.get<{ tools: Array<Record<string, unknown>> } | Array<Record<string, unknown>>>(
        "/v1/tools/builtin",
      );
      const list = Array.isArray(res) ? res : (res.tools ?? []);
      return list
        .filter((t) => t.enabled !== false)
        .map((t) => ({
          name: String(t.name ?? ""),
          label: String(t.display_name || t.name || ""),
        }));
    },
    staleTime: 60_000,
  });

  return {
    agents: agentsQuery.data ?? [],
    tools: toolsQuery.data ?? [],
    loadingAgents: agentsQuery.isLoading,
    loadingTools: toolsQuery.isLoading,
  };
}
