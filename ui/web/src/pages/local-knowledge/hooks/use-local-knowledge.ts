import { useCallback, useEffect, useState } from "react";
import { useHttp } from "@/hooks/use-ws";
import { userFriendlyError } from "@/lib/error-utils";
import { toast } from "@/stores/use-toast-store";
import type {
  LocalKnowledgeSource,
  LocalKnowledgeSyncAllResponse,
  LocalKnowledgeSyncResult,
  LocalKnowledgeSourcesResponse,
} from "@/types/local-knowledge";

export function useLocalKnowledgeSources() {
  const http = useHttp();
  const [sources, setSources] = useState<LocalKnowledgeSource[]>([]);
  const [loading, setLoading] = useState(false);
  const [fetching, setFetching] = useState(false);
  const [syncingKey, setSyncingKey] = useState<string | null>(null);

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

  const syncOne = useCallback(async (sourceKey: string) => {
    setSyncingKey(sourceKey);
    try {
      const res = await http.post<LocalKnowledgeSyncResult>(`/v1/local-knowledge/sources/${encodeURIComponent(sourceKey)}/sync`);
      if (res.error) {
        toast.error("本地知识源同步失败", `${res.name}: ${res.error}`);
      } else {
        toast.success("本地知识源同步完成", `${res.name}: ${res.file_count} 个文件，${res.record_count} 条记录`);
      }
      await refresh({ silent: true });
    } catch (err) {
      toast.error("本地知识源同步失败", userFriendlyError(err));
    } finally {
      setSyncingKey(null);
    }
  }, [http, refresh]);

  const syncAll = useCallback(async () => {
    setSyncingKey("__all__");
    try {
      const res = await http.post<LocalKnowledgeSyncAllResponse>("/v1/local-knowledge/sources/sync");
      const failed = (res.results ?? []).filter((item) => item.error);
      if (failed.length > 0) {
        toast.error("本地知识源部分同步失败", `${failed.length} 个数据源失败，请查看表格错误信息`);
      } else {
        toast.success("本地知识源同步完成", `${res.results?.length ?? 0} 个数据源已扫描`);
      }
      await refresh({ silent: true });
    } catch (err) {
      toast.error("本地知识源同步失败", userFriendlyError(err));
    } finally {
      setSyncingKey(null);
    }
  }, [http, refresh]);

  return { sources, loading, fetching, syncingKey, refresh, syncOne, syncAll };
}
