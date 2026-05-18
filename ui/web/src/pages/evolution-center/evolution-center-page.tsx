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
  RotateCcw,
  ShieldCheck,
  Sparkles,
  XCircle,
} from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { PageHeader } from "@/components/shared/page-header";
import { EmptyState } from "@/components/shared/empty-state";
import { TableSkeleton } from "@/components/shared/loading-skeleton";
import { useAgents } from "@/pages/agents/hooks/use-agents";
import { useEvolutionAudit } from "@/hooks/use-evolution-audit";
import { useEvolutionFeedback } from "@/hooks/use-evolution-feedback";
import { useEvolutionMetrics } from "@/hooks/use-evolution-metrics";
import { useEvolutionRegression, type EvolutionRegressionScope } from "@/hooks/use-evolution-regression";
import { useEvolutionSuggestions } from "@/hooks/use-evolution-suggestions";
import type {
  EvolutionAuditEvent,
  EvolutionFeedback,
  EvolutionRegressionRun,
  EvolutionSuggestion,
} from "@/types/evolution";

const pipelineSteps = [
  {
    name: "反馈采集",
    state: "已接入",
    detail: "聊天回复下方的有用、没用、纠错会写入进化指标；负面反馈和纠错会生成待审核建议，不直接修改线上能力。",
  },
  {
    name: "指标沉淀",
    state: "已接入",
    detail: "记录工具调用、检索使用、用户反馈、回归测试、审计事件，作为后续建议生成和回滚判断的数据来源。",
  },
  {
    name: "建议生成",
    state: "部分完成",
    detail: "当前支持阈值、工具策略、新增自定义 skill 草稿、用户纠错类建议；核心 skill 修改仍需要人工复核。",
  },
  {
    name: "沙箱回归",
    state: "已接入",
    detail: "审批前会自动跑回归。支持 Agent 安全回归、核心 skill 冒烟、业务依赖检查、船务清单 golden 输出评分。",
  },
  {
    name: "审批发布",
    state: "部分完成",
    detail: "建议必须由管理员批准后才能应用。新增自定义 skill 可由建议草稿创建；核心 skill、模型、工具、源码仍不允许自动覆盖。",
  },
  {
    name: "审计回滚",
    state: "已接入",
    detail: "反馈、审批、回归、应用、回滚动作都会进入审计记录；阈值类建议支持按 baseline 回滚。",
  },
];

const pendingSteps = [
  {
    name: "候选版本生成",
    state: "待开发",
    detail: "把纠错反馈自动整理成候选 skill 版本或候选规则包，只进入审核区，不直接替换线上版本。",
  },
  {
    name: "业务评分器扩展",
    state: "进行中",
    detail: "船务清单已有 golden 评分基础；还需要补齐标签生成、包装计算、Excel 类型识别等业务回归评分器。",
  },
  {
    name: "核心 skill 审批链",
    state: "待开发",
    detail: "核心 skill 修改需要候选版本、差异查看、回归结果、人工批准、版本发布、可回滚全链路。",
  },
  {
    name: "自动回滚策略",
    state: "待开发",
    detail: "后续根据失败率、纠错率、业务评分下降触发自动回滚建议，仍保留管理员最终确认。",
  },
];

const statusText: Record<string, string> = {
  pending: "待审",
  approved: "已批准",
  rejected: "已拒绝",
  applied: "已应用",
  rolled_back: "已回滚",
  passed: "通过",
  failed: "失败",
  skipped: "跳过",
};

const suggestionTypeText: Record<string, string> = {
  threshold: "阈值调整",
  tool_order: "工具策略",
  skill_add: "新增 skill 草稿",
  feedback_correction: "用户纠错",
};

const regressionScopeText: Record<EvolutionRegressionScope, string> = {
  agent_safety: "Agent 安全回归",
  core_skill_smoke: "核心 skill 冒烟回归",
  business_workflow_smoke: "业务依赖回归",
  business_output_golden: "业务输出 golden 回归",
};

function statusVariant(status: string) {
  if (status === "passed" || status === "applied" || status === "approved") return "outline";
  if (status === "failed" || status === "rejected" || status === "rolled_back") return "destructive";
  return "secondary";
}

function roadmapVariant(status: string) {
  if (status === "已接入") return "outline";
  if (status === "部分完成" || status === "进行中") return "secondary";
  return "default";
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
  const parameters = suggestion.parameters ? JSON.stringify(suggestion.parameters, null, 2) : "";

  return (
    <div className="rounded-lg border p-4">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="min-w-0 flex-1">
          <div className="flex flex-wrap items-center gap-2">
            <Badge variant="outline">{suggestionTypeText[suggestion.suggestion_type] ?? suggestion.suggestion_type}</Badge>
            <Badge variant={statusVariant(suggestion.status)}>{statusText[suggestion.status] ?? suggestion.status}</Badge>
            <span className="text-xs text-muted-foreground">{new Date(suggestion.created_at).toLocaleString()}</span>
          </div>
          <p className="mt-2 text-sm font-medium">{suggestion.suggestion}</p>
          {suggestion.rationale && <p className="mt-1 text-sm text-muted-foreground">{suggestion.rationale}</p>}
          {parameters && (
            <details className="mt-2">
              <summary className="cursor-pointer text-xs text-muted-foreground">查看建议参数</summary>
              <pre className="mt-2 max-h-40 overflow-auto rounded-md bg-muted p-2 text-xs">{parameters}</pre>
            </details>
          )}
        </div>
        <div className="flex shrink-0 gap-2">
          {isPending && (
            <>
              <Button size="sm" onClick={() => onUpdateStatus(suggestion.id, "approved")}>
                批准并执行
              </Button>
              <Button size="sm" variant="outline" onClick={() => onUpdateStatus(suggestion.id, "rejected")}>
                拒绝
              </Button>
            </>
          )}
          {canRollback && (
            <Button size="sm" variant="destructive" onClick={() => onUpdateStatus(suggestion.id, "rolled_back")}>
              <RotateCcw className="mr-1 h-3.5 w-3.5" />
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
      {item.value.message_content && <p className="mt-2 line-clamp-3 text-muted-foreground">{item.value.message_content}</p>}
      {item.value.correction && (
        <div className="mt-2 rounded-md bg-muted p-2">
          <p className="text-xs font-medium">用户纠错内容</p>
          <p className="mt-1 whitespace-pre-wrap">{item.value.correction}</p>
        </div>
      )}
    </div>
  );
}

function RegressionCard({
  run,
  loading,
  running,
  onRun,
}: {
  run: EvolutionRegressionRun | null;
  loading: boolean;
  running: boolean;
  onRun: (scope?: EvolutionRegressionScope) => Promise<EvolutionRegressionRun | null>;
}) {
  const scopes: EvolutionRegressionScope[] = [
    "agent_safety",
    "core_skill_smoke",
    "business_workflow_smoke",
    "business_output_golden",
  ];

  return (
    <Card>
      <CardHeader>
        <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
          <div className="flex items-center gap-2">
            <Beaker className="h-5 w-5 text-primary" />
            <CardTitle>沙箱回归测试</CardTitle>
          </div>
          <div className="flex flex-wrap gap-2">
            {scopes.map((scope) => (
              <Button key={scope} size="sm" variant={scope === "agent_safety" ? "outline" : "secondary"} onClick={() => void onRun(scope)} disabled={running}>
                <RefreshCw className={`mr-1 h-3.5 w-3.5 ${running ? "animate-spin" : ""}`} />
                {regressionScopeText[scope]}
              </Button>
            ))}
          </div>
        </div>
        <CardDescription>
          审批前会自动跑对应回归；也可以在这里手动执行。业务输出 golden 回归会生成测试输出并记录评分。
        </CardDescription>
      </CardHeader>
      <CardContent>
        {loading ? (
          <TableSkeleton rows={4} />
        ) : !run ? (
          <EmptyState icon={Beaker} title="暂无回归记录" description="点击上方按钮后会生成可审计的回归记录。" />
        ) : (
          <div className="space-y-3">
            <div className="flex flex-wrap items-center justify-between gap-2 rounded-lg border p-3">
              <div>
                <div className="flex items-center gap-2">
                  <Badge variant={statusVariant(run.status)}>{statusText[run.status] ?? run.status}</Badge>
                  <span className="text-sm font-medium">{run.scope}</span>
                </div>
                <p className="mt-1 text-xs text-muted-foreground">
                  {new Date(run.completed_at).toLocaleString()}，{run.passed}/{run.total} 通过
                </p>
              </div>
              {run.status === "passed" ? <CheckCircle2 className="h-5 w-5 text-emerald-500" /> : <XCircle className="h-5 w-5 text-destructive" />}
            </div>
            <div className="space-y-2">
              {run.cases.map((item) => (
                <div key={item.name} className="flex items-start justify-between gap-3 rounded-md border px-3 py-2 text-sm">
                  <div>
                    <p className="font-medium">{item.name}</p>
                    <p className="text-xs text-muted-foreground">{item.message}</p>
                  </div>
                  <Badge variant={statusVariant(item.status)}>{statusText[item.status] ?? item.status}</Badge>
                </div>
              ))}
            </div>
          </div>
        )}
      </CardContent>
    </Card>
  );
}

function AuditCard({ events, loading }: { events: EvolutionAuditEvent[]; loading: boolean }) {
  return (
    <Card>
      <CardHeader>
        <div className="flex items-center gap-2">
          <Activity className="h-5 w-5 text-primary" />
          <CardTitle>审计记录</CardTitle>
        </div>
        <CardDescription>记录反馈、审批、回归测试和回滚动作，方便追溯谁在什么时候改了什么。</CardDescription>
      </CardHeader>
      <CardContent>
        {loading ? (
          <TableSkeleton rows={5} />
        ) : events.length === 0 ? (
          <EmptyState icon={Activity} title="暂无审计记录" description="产生反馈、审批或测试后会显示在这里。" />
        ) : (
          <div className="space-y-2">
            {events.map((event, index) => (
              <div key={`${event.created_at}-${index}`} className="rounded-md border px-3 py-2 text-sm">
                <div className="flex flex-wrap items-center justify-between gap-2">
                  <div className="flex flex-wrap items-center gap-2">
                    <Badge variant={event.result === "ok" ? "outline" : "destructive"}>{event.action}</Badge>
                    {event.status && <Badge variant="secondary">{statusText[event.status] ?? event.status}</Badge>}
                    {event.actor && <span className="text-xs text-muted-foreground">操作者：{event.actor}</span>}
                  </div>
                  <span className="text-xs text-muted-foreground">{new Date(event.created_at).toLocaleString()}</span>
                </div>
                {event.suggestion_id && <p className="mt-1 truncate text-xs text-muted-foreground">建议：{event.suggestion_id}</p>}
                {event.message && <p className="mt-1 text-xs text-muted-foreground">{event.message}</p>}
              </div>
            ))}
          </div>
        )}
      </CardContent>
    </Card>
  );
}

function RoadmapList({ title, items }: {
  title: string;
  items: Array<{ name: string; state: string; detail: string }>;
}) {
  return (
    <div>
      <p className="mb-3 text-sm font-semibold">{title}</p>
      <div className="grid gap-3 lg:grid-cols-2">
        {items.map((item) => (
          <div key={item.name} className="rounded-lg border p-4">
            <div className="flex flex-wrap items-center justify-between gap-2">
              <p className="font-medium">{item.name}</p>
              <Badge variant={roadmapVariant(item.state)}>{item.state}</Badge>
            </div>
            <p className="mt-1 text-sm text-muted-foreground">{item.detail}</p>
          </div>
        ))}
      </div>
    </div>
  );
}

function MetricList({ title, empty, items }: {
  title: string;
  empty: string;
  items: Array<{ key: string; left: string; right: string }>;
}) {
  return (
    <div>
      <p className="mb-2 text-sm font-medium">{title}</p>
      {items.length === 0 ? (
        <p className="text-sm text-muted-foreground">{empty}</p>
      ) : (
        <div className="space-y-2">
          {items.map((item) => (
            <div key={item.key} className="flex items-center justify-between gap-3 rounded-md border px-3 py-2 text-sm">
              <span className="truncate">{item.left}</span>
              <span className="shrink-0 text-muted-foreground">{item.right}</span>
            </div>
          ))}
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
  const { latestRun, loading: regressionLoading, running: regressionRunning, runRegression } = useEvolutionRegression(agentId);
  const { events: auditEvents, loading: auditLoading } = useEvolutionAudit(agentId);

  const pendingCount = suggestions.filter((item) => item.status === "pending").length;
  const appliedCount = suggestions.filter((item) => item.status === "applied" || item.status === "approved").length;
  const correctionCount = feedback.filter((item) => item.value.feedback_type === "correction").length;

  return (
    <div className="p-4 sm:p-6 pb-10">
      <PageHeader
        title="智能进化中心"
        description="集中查看用户反馈、进化指标、待审建议、沙箱回归、审批发布和审计回滚。目标是受控进化：先收集问题，再生成建议，再回归验证，最后人工批准发布。"
        actions={
          <div className="flex items-center gap-2">
            <Badge variant="secondary">管理员可见</Badge>
            <Button variant="outline" size="sm" onClick={refresh} disabled={agentsLoading}>
              <RefreshCw className={`mr-1 h-3.5 w-3.5 ${agentsLoading ? "animate-spin" : ""}`} />
              刷新 Agent
            </Button>
          </div>
        }
      />

      <div className="mt-4 grid gap-4 lg:grid-cols-[1.1fr_0.9fr]">
        <Card>
          <CardHeader>
            <div className="flex items-center gap-2">
              <Sparkles className="h-5 w-5 text-amber-500" />
              <CardTitle>自治进化闭环</CardTitle>
            </div>
            <CardDescription>
              反馈采集、指标沉淀、建议生成、沙箱回归、审批发布、审计回滚已经整合到同一条路线中，避免分散在多个孤立卡片里。
            </CardDescription>
          </CardHeader>
          <CardContent className="grid gap-3 sm:grid-cols-4">
            <div className="rounded-lg border p-3">
              <p className="text-xs text-muted-foreground">待审建议</p>
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
            <CardDescription>选择 Agent 后查看它的反馈、指标、建议、回归测试和审计记录。</CardDescription>
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

      <div className="mt-4">
        <Tabs defaultValue="control">
          <TabsList className="flex h-auto w-full flex-wrap justify-start">
            <TabsTrigger value="control">测试与审计</TabsTrigger>
            <TabsTrigger value="signals">指标与反馈</TabsTrigger>
            <TabsTrigger value="suggestions">建议审批</TabsTrigger>
            <TabsTrigger value="roadmap">自治路线</TabsTrigger>
          </TabsList>

          <TabsContent value="control" className="mt-4">
            <div className="grid gap-4 xl:grid-cols-2">
              {agentId ? (
                <>
                  <RegressionCard run={latestRun} loading={regressionLoading} running={regressionRunning} onRun={runRegression} />
                  <AuditCard events={auditEvents} loading={auditLoading} />
                </>
              ) : (
                <Card className="xl:col-span-2">
                  <EmptyState icon={Beaker} title="请选择 Agent" description="选择 Agent 后可运行沙箱回归并查看审计记录。" />
                </Card>
              )}
            </div>
          </TabsContent>

          <TabsContent value="signals" className="mt-4">
            <div className="grid gap-4 xl:grid-cols-2">
              <Card>
                <CardHeader>
                  <div className="flex items-center gap-2">
                    <BarChart3 className="h-5 w-5 text-primary" />
                    <CardTitle>进化指标</CardTitle>
                  </div>
                  <CardDescription>展示工具成功率、调用次数、检索使用率等数据，作为自动建议的依据。</CardDescription>
                </CardHeader>
                <CardContent>
                  {!agentId ? (
                    <EmptyState icon={BarChart3} title="请选择 Agent" description="选择 Agent 后查看指标。" />
                  ) : metricsLoading ? (
                    <TableSkeleton rows={4} />
                  ) : (
                    <div className="space-y-4">
                      <MetricList
                        title="工具指标"
                        empty="暂无工具指标。"
                        items={toolAggs.slice(0, 8).map((item) => ({
                          key: item.tool_name,
                          left: item.tool_name,
                          right: `${item.call_count} 次 / ${(item.success_rate * 100).toFixed(1)}%`,
                        }))}
                      />
                      <MetricList
                        title="检索指标"
                        empty="暂无检索指标。"
                        items={retrievalAggs.slice(0, 8).map((item) => ({
                          key: item.source,
                          left: item.source,
                          right: `${item.query_count} 次 / ${(item.usage_rate * 100).toFixed(1)}%`,
                        }))}
                      />
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
                  <CardDescription>聊天中提交的有用、没用、纠错反馈。负面和纠错反馈会生成待审建议。</CardDescription>
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
            </div>
          </TabsContent>

          <TabsContent value="suggestions" className="mt-4">
            <Card>
              <CardHeader>
                <div className="flex items-center gap-2">
                  <GitBranch className="h-5 w-5 text-primary" />
                  <CardTitle>进化建议审批</CardTitle>
                </div>
                <CardDescription>
                  批准前会先跑对应回归。新增自定义 skill 可以通过草稿创建；核心 skill、模型、工具、数据库和源码级改动必须人工复核。
                </CardDescription>
              </CardHeader>
              <CardContent>
                {!agentId ? (
                  <EmptyState icon={GitBranch} title="请选择 Agent" description="选择 Agent 后查看进化建议。" />
                ) : suggestionsLoading ? (
                  <TableSkeleton rows={4} />
                ) : suggestions.length === 0 ? (
                  <EmptyState icon={CheckCircle2} title="暂无建议" description="当前 Agent 暂无进化建议。" />
                ) : (
                  <div className="space-y-3">
                    {suggestions.map((suggestion) => (
                      <SuggestionCard key={suggestion.id} suggestion={suggestion} onUpdateStatus={updateStatus} />
                    ))}
                  </div>
                )}
              </CardContent>
            </Card>
          </TabsContent>

          <TabsContent value="roadmap" className="mt-4">
            <Card>
              <CardHeader>
                <div className="flex items-center gap-2">
                  <ClipboardList className="h-5 w-5 text-primary" />
                  <CardTitle>完整自治进化路线</CardTitle>
                </div>
                <CardDescription>
                  把反馈采集、指标沉淀、建议生成、沙箱回归、审批发布、审计回滚放到同一条路线中，便于判断哪些已完成、哪些还要继续开发。
                </CardDescription>
              </CardHeader>
              <CardContent className="space-y-6">
                <div className="rounded-lg border bg-muted/30 p-4">
                  <div className="flex items-center gap-2">
                    <ShieldCheck className="h-4 w-4 text-primary" />
                    <p className="text-sm font-medium">安全边界</p>
                  </div>
                  <p className="mt-1 text-sm text-muted-foreground">
                    自定义 skill 可以走轻量审批；核心 skill、模型配置、工具权限、租户权限、数据库和源码改动必须人工审核并保留回滚记录。
                  </p>
                </div>
                <RoadmapList title="已接入和建设中" items={pipelineSteps} />
                <RoadmapList title="还需要继续开发" items={pendingSteps} />
              </CardContent>
            </Card>
          </TabsContent>
        </Tabs>
      </div>
    </div>
  );
}
