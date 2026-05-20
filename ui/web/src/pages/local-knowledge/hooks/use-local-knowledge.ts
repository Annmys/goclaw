import { useCallback, useEffect, useState } from "react";
import { useHttp } from "@/hooks/use-ws";
import { userFriendlyError } from "@/lib/error-utils";
import { toast } from "@/stores/use-toast-store";
import type { LocalKnowledgeSource, LocalKnowledgeSourcesResponse } from "@/types/local-knowledge";

export function useLocalKnowledgeSources() {
  const http = useHttp();
  const [sources, setSources] = useState<LocalKnowledgeSource[]>([]);
  const [loading, setLoading] = useState(false);
  const [fetching, setFetching] = useState(false);

  const refresh = useCallback(async (opts?: { silent?: boolean }) => {
    if (opts?.silent) setFetching(true);
    else setLoading(true);
    try {
      const res = await http.get<LocalKnowledgeSourcesResponse>("/v1/local-knowledge/sources");
      setSources(res.sources ?? []);
    } catch (err) {
      toast.error("本地知识源加载失败", userFriendlyError(err));
    } finally {
      setLoading(false);
      setFetching(false);
    }
  }, [http]);

  useEffect(() => {
    refresh();
  }, [refresh]);

  return { sources, loading, fetching, refresh };
}

