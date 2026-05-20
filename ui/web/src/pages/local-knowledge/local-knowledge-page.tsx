import { Database, RefreshCw, FolderOpen, AlertTriangle } from "lucide-react";
import { useTranslation } from "react-i18next";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { PageHeader } from "@/components/shared/page-header";
import { EmptyState } from "@/components/shared/empty-state";
import { TableSkeleton } from "@/components/shared/loading-skeleton";
import { formatDate } from "@/lib/format";
import { cn } from "@/lib/utils";
import { useDeferredLoading } from "@/hooks/use-deferred-loading";
import { useMinLoading } from "@/hooks/use-min-loading";
import { useLocalKnowledgeSources } from "./hooks/use-local-knowledge";
import type { LocalKnowledgeSource } from "@/types/local-knowledge";

function statusVariant(source: LocalKnowledgeSource): "success" | "warning" | "secondary" {
  if (!source.enabled) return "secondary";
  if (source.last_error) return "warning";
  return "success";
}

function formatCount(value: number | null | undefined) {
  return new Intl.NumberFormat().format(value ?? 0);
}

function PathBlock({ label, value }: { label: string; value?: string }) {
  if (!value) return null;
  return (
    <div className="min-w-0">
      <div className="text-[11px] uppercase tracking-wide text-muted-foreground">{label}</div>
      <div className="mt-0.5 truncate font-mono text-xs text-foreground" title={value}>{value}</div>
    </div>
  );
}

export function LocalKnowledgePage() {
  const { t } = useTranslation("local-knowledge");
  const { t: tc } = useTranslation("common");
  const { sources, loading, fetching, refresh } = useLocalKnowledgeSources();
  const spinning = useMinLoading(fetching || loading);
  const showSkeleton = useDeferredLoading(loading && sources.length === 0);

  const enabledCount = sources.filter((s) => s.enabled).length;
  const scheduledCount = sources.filter((s) => s.sync_mode === "scheduled").length;

  return (
    <div className="p-4 sm:p-6 pb-10">
      <PageHeader
        title={t("title")}
        description={t("description")}
        actions={
          <Button variant="outline" size="sm" onClick={() => refresh({ silent: true })} disabled={spinning} className="gap-1">
            <RefreshCw className={cn("h-3.5 w-3.5", spinning && "animate-spin")} />
            {tc("refresh", "刷新")}
          </Button>
        }
      />

      <div className="mt-4 grid gap-3 sm:grid-cols-3">
        <div className="rounded-xl border bg-card p-4">
          <div className="text-sm text-muted-foreground">{t("summary.total")}</div>
          <div className="mt-2 text-2xl font-semibold">{sources.length}</div>
        </div>
        <div className="rounded-xl border bg-card p-4">
          <div className="text-sm text-muted-foreground">{t("summary.enabled")}</div>
          <div className="mt-2 text-2xl font-semibold">{enabledCount}</div>
        </div>
        <div className="rounded-xl border bg-card p-4">
          <div className="text-sm text-muted-foreground">{t("summary.scheduled")}</div>
          <div className="mt-2 text-2xl font-semibold">{scheduledCount}</div>
        </div>
      </div>

      <div className="mt-5 rounded-xl border bg-card">
        <div className="border-b px-4 py-3">
          <div className="flex items-center gap-2 text-sm font-medium">
            <Database className="h-4 w-4 text-muted-foreground" />
            {t("table.title")}
          </div>
        </div>

        {showSkeleton ? (
          <div className="p-4">
            <TableSkeleton rows={5} />
          </div>
        ) : sources.length === 0 ? (
          <EmptyState icon={FolderOpen} title={t("empty.title")} description={t("empty.description")} />
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b bg-muted/40">
                  <th className="px-4 py-3 text-left font-medium">{t("table.source")}</th>
                  <th className="px-4 py-3 text-left font-medium">{t("table.paths")}</th>
                  <th className="px-4 py-3 text-left font-medium">{t("table.mode")}</th>
                  <th className="px-4 py-3 text-left font-medium">{t("table.counts")}</th>
                  <th className="px-4 py-3 text-left font-medium">{t("table.lastSync")}</th>
                </tr>
              </thead>
              <tbody>
                {sources.map((source) => (
                  <tr key={source.id} className="border-b last:border-0 hover:bg-muted/30">
                    <td className="px-4 py-3 align-top">
                      <div className="font-medium">{source.name}</div>
                      <div className="mt-1 font-mono text-xs text-muted-foreground">{source.source_key}</div>
                      <div className="mt-2 flex flex-wrap gap-1.5">
                        <Badge variant={statusVariant(source)}>
                          {source.enabled ? t("status.enabled") : t("status.disabled")}
                        </Badge>
                        <Badge variant="outline">{source.tenant_scope}</Badge>
                      </div>
                      {source.description && (
                        <p className="mt-2 max-w-xs text-xs text-muted-foreground">{source.description}</p>
                      )}
                    </td>
                    <td className="px-4 py-3 align-top">
                      <div className="grid min-w-[260px] gap-2">
                        <PathBlock label={t("paths.windows")} value={source.path_windows} />
                        <PathBlock label={t("paths.container")} value={source.path_container} />
                      </div>
                    </td>
                    <td className="px-4 py-3 align-top">
                      <div className="flex flex-col gap-1.5">
                        <Badge variant="secondary">{source.sync_mode}</Badge>
                        <Badge variant="info">{source.index_target}</Badge>
                      </div>
                    </td>
                    <td className="px-4 py-3 align-top text-xs">
                      <div>{t("counts.files")}: {formatCount(source.file_count)}</div>
                      <div className="mt-1">{t("counts.records")}: {formatCount(source.record_count)}</div>
                      {source.content_hash && (
                        <div className="mt-1 max-w-[160px] truncate font-mono text-muted-foreground" title={source.content_hash}>
                          {source.content_hash}
                        </div>
                      )}
                    </td>
                    <td className="px-4 py-3 align-top">
                      <div className="whitespace-nowrap text-xs text-muted-foreground">
                        {source.last_sync_at ? formatDate(source.last_sync_at) : t("never")}
                      </div>
                      {source.last_success_at && (
                        <div className="mt-1 whitespace-nowrap text-xs text-emerald-600 dark:text-emerald-400">
                          {t("lastSuccess")}: {formatDate(source.last_success_at)}
                        </div>
                      )}
                      {source.last_error && (
                        <div className="mt-2 flex max-w-[260px] items-start gap-1.5 rounded-md bg-amber-500/10 p-2 text-xs text-amber-700 dark:text-amber-300">
                          <AlertTriangle className="mt-0.5 h-3.5 w-3.5 shrink-0" />
                          <span className="break-words">{source.last_error}</span>
                        </div>
                      )}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>
    </div>
  );
}

