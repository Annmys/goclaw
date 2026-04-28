import { useMemo, useState } from "react";
import {
  Activity,
  BarChart3,
  Beaker,
  CheckCircle2,
  ClipboardList,
  GitBranch,
  MessageSquareWarning,
  RefreshCw,
  Sparkles,
  XCircle,
} from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { PageHeader } from "@/components/shared/page-header";
import { EmptyState } from "@/components/shared/empty-state";
import { TableSkeleton } from "@/components/shared/loading-skeleton";
import { useAgents } from "@/pages/agents/hooks/use-agents";
import { useEvolutionMetrics } from "@/hooks/use-evolution-metrics";
import { useEvolutionSuggestions } from "@/hooks/use-evolution-suggestions";
import type { EvolutionSuggestion } from "@/types/evolution";

const pipeline = [
  {
    title: "用户反馈入口",
    description: "待开发：聊天消息下方接入有用、没用、需要纠正按钮，并记录纠错文本。",
    icon: MessageSquareWarning,
    status: "待开发",
  },
  {
    title: "纠错记忆",
    description: "待开发：把有效纠错写入即时提示和相似问题召回层。",
    icon: ClipboardList,
    status: "待开发",
  },
  {
    title: "演进建议",
    description: "已接入：读取当前 Agent 的 evolution suggestions，可审批、拒绝和回滚。",
    icon: GitBranch,
    status: "已接入",
  },
  {
    title: "沙箱测试",
    description: "待开发：对 skill 草案执行测试订单、历史失败案例和标准文件回归。",
    icon: Beaker,
    status: "待开发",
  },
  {
    title: "审批发布",
    description: "已接入：复用 GoClaw 当前 evolution suggestion 审批接口。",
    icon: CheckCircle2,
    status: "已接入",
  },
  {
    title: "审计回滚",
    description: "部分已有：建议状态包含 applied、rejected、rolled_back，后续补完整审计页面。",
    icon: Activity,
    status: "部分已有",
  },
];

function statusVariant(status: string) {
  if (status === "pending") return "secondary";
  if (status === "applied" || status === "approved") return "outline";
  if (status === "rejected" || status === "rolled_back") return "destructive";
  return "secondary";
}

function SuggestionCard({
  suggestion,
  onUpdateStatus,
}: {
  suggestion: EvolutionSuggestion;
  onUpdateStatus: (id: string, status: "approved" | "rejected" | "rolled_back") => Promise<void>;
}) {
  const isPending = suggestion.status === "pending";
  const canRollback = suggestion.status === "applied" || suggestion.status === "approved";

  return (
    <div className="rounded-lg border p-4">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="min-w-0 flex-1">
          <div className="flex flex-wrap items-center gap-2">
            <Badge variant="outline">{suggestion.suggestion_type}</Badge>
            <Badge variant={statusVariant(suggestion.status)}>{suggestion.status}</Badge>
            <span className="text-xs text-muted-foreground">
              {new Date(suggestion.created_at).toLocaleString()}
            </span>
          </div>
          <p className="mt-2 text-sm font-medium">{suggestion.suggestion}</p>
          {suggestion.rationale && (
            <p className="mt-1 text-sm text-muted-foreground">{suggestion.rationale}</p>
          )}
        </div>
        <div className="flex shrink-0 gap-2">
          {isPending && (
            <>
              <Button size="sm" onClick={() => onUpdateStatus(suggestion.id, "approved")}>
                批准
              </Button>
              <Button size="sm" variant="outline" onClick={() => onUpdateStatus(suggestion.id, "rejected")}>
                拒绝
              </Button>
            </>
          )}
          {canRollback && (
            <Button size="sm" variant="destructive" onClick={() => onUpdateStatus(suggestion.id, "rolled_back")}>
              回滚
            </Button>
          )}
        </div>
      </div>
    </div>
  );
}

export function EvolutionCenterPage() {
  const { agents, loading: agentsLoading, refresh } = useAgents();
  const [agentId, setAgentId] = useState("");
  const [timeRange, setTimeRange] = useState("7d");

  const selectedAgent = useMemo(
    () => agents.find((agent) => agent.id === agentId),
    [agents, agentId],
  );

  const { toolAggs, retrievalAggs, loading: metricsLoading } = useEvolutionMetrics(agentId, timeRange);
  const { suggestions, loading: suggestionsLoading, updateStatus } = useEvolutionSuggestions(agentId);

  const pendingCount = suggestions.filter((item) => item.status === "pending").length;
  const appliedCount = suggestions.filter((item) => item.status === "applied" || item.status === "approved").length;

  return (
    <div className="p-4 sm:p-6 pb-10">
      <PageHeader
        title="智能进化中心"
        description="管理员统一查看用户反馈闭环、演进指标、演进建议、审批发布和后续 capability-evolver 接入状态。"
        actions={
          <div className="flex items-center gap-2">
            <Badge variant="secondary">管理员可见</Badge>
            <Button variant="outline" size="sm" onClick={refresh} disabled={agentsLoading}>
              <RefreshCw className={`mr-1 h-3.5 w-3.5 ${agentsLoading ? "animate-spin" : ""}`} />
              刷新
            </Button>
          </div>
        }
      />

      <div className="mt-4 grid gap-4 lg:grid-cols-[1.1fr_0.9fr]">
        <Card>
          <CardHeader>
            <div className="flex items-center gap-2">
              <Sparkles className="h-5 w-5 text-amber-500" />
              <CardTitle>目标闭环</CardTitle>
            </div>
            <CardDescription>
              业务执行 → 用户反馈/系统指标 → 纠错暂存 → 记忆召回 → 演进建议 → 沙箱测试 → 审批发布 → 线上回归 → 审计回滚
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-3">
            <div className="grid gap-3 sm:grid-cols-3">
              <div className="rounded-lg border p-3">
                <p className="text-xs text-muted-foreground">待审批建议</p>
                <p className="mt-1 text-2xl font-semibold">{pendingCount}</p>
              </div>
              <div className="rounded-lg border p-3">
                <p className="text-xs text-muted-foreground">已应用建议</p>
                <p className="mt-1 text-2xl font-semibold">{appliedCount}</p>
              </div>
              <div className="rounded-lg border p-3">
                <p className="text-xs text-muted-foreground">当前 Agent</p>
                <p className="mt-1 truncate text-sm font-medium">
                  {selectedAgent?.display_name || selectedAgent?.agent_key || "未选择"}
                </p>
              </div>
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>Agent 范围</CardTitle>
            <CardDescription>当前接口按 Agent 查看演进指标和建议，后续再扩展为租户总览。</CardDescription>
          </CardHeader>
          <CardContent className="space-y-3">
            <select
              value={agentId}
              onChange={(event) => setAgentId(event.target.value)}
              className="h-9 w-full rounded-md border bg-background px-3 text-base md:text-sm"
            >
              <option value="">选择 Agent</option>
              {agents.map((agent) => (
                <option key={agent.id} value={agent.id}>
                  {agent.display_name || agent.agent_key}
                </option>
              ))}
            </select>
            <select
              value={timeRange}
              onChange={(event) => setTimeRange(event.target.value)}
              className="h-9 w-full rounded-md border bg-background px-3 text-base md:text-sm"
            >
              <option value="7d">最近 7 天</option>
              <option value="30d">最近 30 天</option>
              <option value="90d">最近 90 天</option>
            </select>
          </CardContent>
        </Card>
      </div>

      <div className="mt-4 grid gap-4 md:grid-cols-2 xl:grid-cols-3">
        {pipeline.map((item) => {
          const Icon = item.icon;
          return (
            <Card key={item.title}>
              <CardHeader className="space-y-3">
                <div className="flex items-center justify-between gap-3">
                  <Icon className="h-5 w-5 text-primary" />
                  <Badge variant={item.status === "已接入" ? "outline" : "secondary"}>{item.status}</Badge>
                </div>
                <div>
                  <CardTitle className="text-base">{item.title}</CardTitle>
                  <CardDescription className="mt-1">{item.description}</CardDescription>
                </div>
              </CardHeader>
            </Card>
          );
        })}
      </div>

      <div className="mt-4 grid gap-4 xl:grid-cols-2">
        <Card>
          <CardHeader>
            <div className="flex items-center gap-2">
              <BarChart3 className="h-5 w-5 text-primary" />
              <CardTitle>演进指标</CardTitle>
            </div>
            <CardDescription>来自当前 GoClaw evolution metrics：工具成功率和检索使用率。</CardDescription>
          </CardHeader>
          <CardContent>
            {!agentId ? (
              <EmptyState icon={BarChart3} title="请选择 Agent" description="选择 Agent 后查看指标。" />
            ) : metricsLoading ? (
              <TableSkeleton rows={4} />
            ) : (
              <div className="space-y-4">
                <div>
                  <p className="mb-2 text-sm font-medium">工具指标</p>
                  {toolAggs.length === 0 ? (
                    <p className="text-sm text-muted-foreground">暂无工具指标。</p>
                  ) : (
                    <div className="space-y-2">
                      {toolAggs.slice(0, 8).map((item) => (
                        <div key={item.tool_name} className="flex items-center justify-between rounded-md border px-3 py-2 text-sm">
                          <span className="truncate">{item.tool_name}</span>
                          <span className="text-muted-foreground">
                            {item.call_count} 次 / {(item.success_rate * 100).toFixed(1)}%
                          </span>
                        </div>
                      ))}
                    </div>
                  )}
                </div>
                <div>
                  <p className="mb-2 text-sm font-medium">检索指标</p>
                  {retrievalAggs.length === 0 ? (
                    <p className="text-sm text-muted-foreground">暂无检索指标。</p>
                  ) : (
                    <div className="space-y-2">
                      {retrievalAggs.slice(0, 8).map((item) => (
                        <div key={item.source} className="flex items-center justify-between rounded-md border px-3 py-2 text-sm">
                          <span className="truncate">{item.source}</span>
                          <span className="text-muted-foreground">
                            {item.query_count} 次 / {(item.usage_rate * 100).toFixed(1)}%
                          </span>
                        </div>
                      ))}
                    </div>
                  )}
                </div>
              </div>
            )}
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <div className="flex items-center gap-2">
              <GitBranch className="h-5 w-5 text-primary" />
              <CardTitle>演进建议审批</CardTitle>
            </div>
            <CardDescription>当前已接入 GoClaw 原生建议审批接口。核心 skill 和源码级变更后续必须继续走人工审核。</CardDescription>
          </CardHeader>
          <CardContent>
            {!agentId ? (
              <EmptyState icon={GitBranch} title="请选择 Agent" description="选择 Agent 后查看演进建议。" />
            ) : suggestionsLoading ? (
              <TableSkeleton rows={4} />
            ) : suggestions.length === 0 ? (
              <EmptyState icon={CheckCircle2} title="暂无建议" description="当前 Agent 暂无演进建议。" />
            ) : (
              <div className="space-y-3">
                {suggestions.map((suggestion) => (
                  <SuggestionCard key={suggestion.id} suggestion={suggestion} onUpdateStatus={updateStatus} />
                ))}
              </div>
            )}
          </CardContent>
        </Card>
      </div>

      <Card className="mt-4">
        <CardHeader>
          <div className="flex items-center gap-2">
            <XCircle className="h-5 w-5 text-muted-foreground" />
            <CardTitle>未完成但必须补齐的能力</CardTitle>
          </div>
          <CardDescription>这部分还没有完成，不能算完整智能进化中心。</CardDescription>
        </CardHeader>
        <CardContent className="grid gap-3 md:grid-cols-3">
          <div className="rounded-md border p-3 text-sm">
            <p className="font-medium">聊天反馈按钮</p>
            <p className="mt-1 text-muted-foreground">需要新增消息级反馈 API、反馈表和前端按钮。</p>
          </div>
          <div className="rounded-md border p-3 text-sm">
            <p className="font-medium">沙箱回归测试</p>
            <p className="mt-1 text-muted-foreground">需要把测试订单、标准文件和失败案例接入自动验证。</p>
          </div>
          <div className="rounded-md border p-3 text-sm">
            <p className="font-medium">capability-evolver 执行链路</p>
            <p className="mt-1 text-muted-foreground">需要作为后台 worker 接入，只生成草案，不直接改核心能力。</p>
          </div>
        </CardContent>
      </Card>
    </div>
  );
}
