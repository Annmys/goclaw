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
  ShieldCheck,
  Sparkles,
} from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { PageHeader } from "@/components/shared/page-header";
import { EmptyState } from "@/components/shared/empty-state";
import { TableSkeleton } from "@/components/shared/loading-skeleton";
import { useAgents } from "@/pages/agents/hooks/use-agents";
import { useEvolutionFeedback } from "@/hooks/use-evolution-feedback";
import { useEvolutionMetrics } from "@/hooks/use-evolution-metrics";
import { useEvolutionSuggestions } from "@/hooks/use-evolution-suggestions";
import type { EvolutionFeedback, EvolutionSuggestion } from "@/types/evolution";

const pipeline = [
  {
    title: "聊天反馈入口",
    description: "助手回复下方已接入有用、没用、纠错按钮，反馈进入进化中心。",
    icon: MessageSquareWarning,
    status: "已接入",
  },
  {
    title: "纠错暂存",
    description: "纠错和负面反馈先进入待审批建议，不直接修改核心 skill 或配置。",
    icon: ClipboardList,
    status: "已接入",
  },
  {
    title: "演进建议审批",
    description: "复用 GoClaw 原生 evolution suggestions，可批准、拒绝和回滚。",
    icon: GitBranch,
    status: "已接入",
  },
  {
    title: "沙箱回归测试",
    description: "后续要接入船务清单等标准测试订单，审批前自动跑回归。",
    icon: Beaker,
    status: "待开发",
  },
  {
    title: "审批发布",
    description: "核心 skill、模型、工具、源码级变更必须管理员审批，不自动发布。",
    icon: ShieldCheck,
    status: "已接入",
  },
  {
    title: "审计回滚",
    description: "建议状态支持 pending、approved、rejected、applied、rolled_back。",
    icon: Activity,
    status: "部分完成",
  },
];

function statusVariant(status: string) {
  if (status === "pending") return "secondary";
  if (status === "applied" || status === "approved") return "outline";
  if (status === "rejected" || status === "rolled_back") return "destructive";
  return "secondary";
}

function pipelineVariant(status: string) {
  if (status === "已接入") return "outline";
  if (status === "部分完成") return "secondary";
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

function FeedbackCard({ item }: { item: EvolutionFeedback }) {
  const typeLabel = {
    useful: "有用",
    not_useful: "没用",
    correction: "纠错",
  }[item.value.feedback_type] ?? item.value.feedback_type;

  return (
    <div className="rounded-lg border p-3 text-sm">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div className="flex flex-wrap items-center gap-2">
          <Badge variant={item.value.feedback_type === "useful" ? "outline" : "secondary"}>{typeLabel}</Badge>
          {item.value.requires_approval && <Badge variant="secondary">待复核</Badge>}
          {item.value.user_id && <span className="text-xs text-muted-foreground">用户：{item.value.user_id}</span>}
        </div>
        <span className="text-xs text-muted-foreground">{new Date(item.created_at).toLocaleString()}</span>
      </div>
      <p className="mt-2 truncate text-xs text-muted-foreground">会话：{item.session_key}</p>
      {item.value.message_content && (
        <p className="mt-2 line-clamp-3 text-muted-foreground">{item.value.message_content}</p>
      )}
      {item.value.correction && (
        <div className="mt-2 rounded-md bg-muted p-2">
          <p className="text-xs font-medium">用户纠错</p>
          <p className="mt-1 whitespace-pre-wrap">{item.value.correction}</p>
        </div>
      )}
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
  const { feedback, loading: feedbackLoading } = useEvolutionFeedback(agentId, timeRange);

  const pendingCount = suggestions.filter((item) => item.status === "pending").length;
  const appliedCount = suggestions.filter((item) => item.status === "applied" || item.status === "approved").length;
  const correctionCount = feedback.filter((item) => item.value.feedback_type === "correction").length;

  return (
    <div className="p-4 sm:p-6 pb-10">
      <PageHeader
        title="智能进化中心"
        description="管理员统一查看用户反馈、演进指标、待审批建议和安全发布状态。capability-evolver 只作为后台草稿生成能力，不直接改核心能力。"
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
              业务执行到用户反馈，再到纠错暂存、建议生成、沙箱测试、人工审批、发布和回滚。当前版本已完成反馈采集和审批入口，沙箱自动回归仍需后续补齐。
            </CardDescription>
          </CardHeader>
          <CardContent className="grid gap-3 sm:grid-cols-4">
            <div className="rounded-lg border p-3">
              <p className="text-xs text-muted-foreground">待审批建议</p>
              <p className="mt-1 text-2xl font-semibold">{pendingCount}</p>
            </div>
            <div className="rounded-lg border p-3">
              <p className="text-xs text-muted-foreground">已批准/应用</p>
              <p className="mt-1 text-2xl font-semibold">{appliedCount}</p>
            </div>
            <div className="rounded-lg border p-3">
              <p className="text-xs text-muted-foreground">纠错反馈</p>
              <p className="mt-1 text-2xl font-semibold">{correctionCount}</p>
            </div>
            <div className="rounded-lg border p-3">
              <p className="text-xs text-muted-foreground">当前 Agent</p>
              <p className="mt-1 truncate text-sm font-medium">
                {selectedAgent?.display_name || selectedAgent?.agent_key || "未选择"}
              </p>
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>Agent 范围</CardTitle>
            <CardDescription>当前按 Agent 查看反馈、指标和建议，后续再扩展租户总览。</CardDescription>
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
                  <Badge variant={pipelineVariant(item.status)}>{item.status}</Badge>
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

      <div className="mt-4 grid gap-4 xl:grid-cols-3">
        <Card>
          <CardHeader>
            <div className="flex items-center gap-2">
              <BarChart3 className="h-5 w-5 text-primary" />
              <CardTitle>演进指标</CardTitle>
            </div>
            <CardDescription>来自 GoClaw evolution metrics，显示工具成功率和检索使用率。</CardDescription>
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
              <MessageSquareWarning className="h-5 w-5 text-primary" />
              <CardTitle>用户反馈记录</CardTitle>
            </div>
            <CardDescription>聊天中提交的有用、没用、纠错反馈。负面和纠错反馈会生成待审批建议。</CardDescription>
          </CardHeader>
          <CardContent>
            {!agentId ? (
              <EmptyState icon={MessageSquareWarning} title="请选择 Agent" description="选择 Agent 后查看反馈。" />
            ) : feedbackLoading ? (
              <TableSkeleton rows={4} />
            ) : feedback.length === 0 ? (
              <EmptyState icon={CheckCircle2} title="暂无反馈" description="当前 Agent 暂无用户反馈。" />
            ) : (
              <div className="space-y-3">
                {feedback.map((item) => (
                  <FeedbackCard key={item.id} item={item} />
                ))}
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
            <CardDescription>审批后才会进入下一步。核心 skill 和源码级改动仍必须人工复核。</CardDescription>
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
    </div>
  );
}
