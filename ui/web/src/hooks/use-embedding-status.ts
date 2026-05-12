import { useState, useEffect, useCallback } from "react";
import { useHttp } from "./use-ws";
import { useAuthStore } from "@/stores/use-auth-store";

interface EmbeddingStatus {
  configured: boolean;
  provider?: string;
  provider_name?: string;
  model?: string;
}

export function useEmbeddingStatus(enabled = true) {
  const http = useHttp();
  const tenantId = useAuthStore((s) => s.tenantId);
  const [status, setStatus] = useState<EmbeddingStatus | null>(null);
  const [loading, setLoading] = useState(enabled);

  const refresh = useCallback(async () => {
    if (!enabled) {
      setStatus(null);
      setLoading(false);
      return;
    }
    try {
      const res = await http.get<EmbeddingStatus>("/v1/embedding/status");
      setStatus(res);
    } catch {
      setStatus({ configured: false });
    } finally {
      setLoading(false);
    }
  }, [enabled, http]);

  useEffect(() => {
    refresh();
  }, [refresh, tenantId]);

  return { status, loading, refresh };
}
