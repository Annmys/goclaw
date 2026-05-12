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

const pipeline = [
  {
    title: "反馈采集",
    description: "聊天回复下方可提交有用、没用、纠错反馈。负面反馈会进入待审队列，不会直接改核心能力。",
    status: "已接入",
  },
  {
    title: "指标沉淀",
    description: "记录工具调用成功率、耗时、检索使用率，为后续自动建议提供数据依据。",
    status: "已接入",
  },
  {
    title: "建议生成",
    description: "规则引擎会生成阈值、工具、skill 草稿和纠错类建议，当前仍以管理员审核为准。",
    status: "部分完成",
  },
  {
    title: "沙箱回归",
    description: "支持 Agent 安全回归、核心业务 skill 冒烟回归、业务依赖回归和船务清单 golden 样例输出评分。",
    status: "进行中",
  },
  {
    title: "审批发布",
    description: "核心 skill、模型、工具、租户、数据库和源码级变更必须人工审批，不能自动上线。",
    status: "已接入",
  },
  {
    title: "审计回滚",
    description: "反馈、审批、测试和回滚动作会记录审计。阈值类建议已有 baseline 回滚能力。",
    status: "已接入",
  },
];

const autonomousRoadmap = [
  ...pipeline.map((item) => ({
    name: item.title,
    state: item.status,
    detail: item.description,
  })),
  { name: "业务依赖回归", state: "已接入", detail: "已检查船务、标签、包装计算依赖的核心 skill、sqlite 索引、标签模板和共享存储目录。" },
  { name: "业务输出回归测试集", state: "进行中", detail: "已先接入船务清单处理 golden 样例评分；下一步继续补标签生成、包装计算、Excel 类型识别。" },
];

const pendingRoadmap = [
  { name: "候选版本生成", state: "未完成", detail: "后续由进化引擎生成候选 skill 版本，不直接覆盖线上核心 skill。" },
  { name: "自动评分器", state: "部分完成", detail: "船务清单已检查 Excel sheet、合并单元格、列宽、Logo/图片、关键字段、Total 后垃圾行和多包装行保留。" },
  { name: "审批发布链", state: "部分完成", detail: "审批前已接入自动回归阻断；核心 skill 候选版本发布和灰度链路还要继续做。" },
  { name: "监控与自动回滚", state: "部分完成", detail: "阈值类建议可回滚；业务 skill 的失败率、纠错率、评分下降回滚还未完成。" },
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
  threshold: "检索阈值",
  tool_order: "工具策略",
  skill_add: "新增 skill 草稿",
  feedback_correction: "用户纠错",
};

function statusVariant(status: string) {
  if (status === "passed" || status === "applied" || status === "approved") return "outline";
  if (status === "failed" || status === "rejected" || status === "rolled_back") return "destructive";
  return "secondary";
}

function pipelineVariant(status: string) {
  if (status === "已接入") return "outline";
  if (status === "进行中" || status === "部分完成" || status === "开始建设") return "secondary";
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
            <Badge variant="outline">{suggestionTypeText[suggestion.suggestion_type] ?? suggestion.suggestion_type}</Badge>
            <Badge variant={statusVariant(suggestion.status)}>{statusText[suggestion.status] ?? suggestion.status}</Badge>
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
  return (
    <Card>
      <CardHeader>
        <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
          <div className="flex items-center gap-2">
            <Beaker className="h-5 w-5 text-primary" />
            <CardTitle>沙箱回归测试</CardTitle>
          </div>
          <div className="flex flex-wrap gap-2">
            <Button size="sm" variant="outline" onClick={() => void onRun("agent_safety")} disabled={running}>
              <RefreshCw className={`mr-1 h-3.5 w-3.5 ${running ? "animate-spin" : ""}`} />
              Agent 安全回归
            </Button>
            <Button size="sm" onClick={() => void onRun("core_skill_smoke")} disabled={running}>
              <RefreshCw className={`mr-1 h-3.5 w-3.5 ${running ? "animate-spin" : ""}`} />
              核心 skill 冒烟回归
            </Button>
            <Button size="sm" variant="secondary" onClick={() => void onRun("business_workflow_smoke")} disabled={running}>
              <RefreshCw className={`mr-1 h-3.5 w-3.5 ${running ? "animate-spin" : ""}`} />
              业务依赖回归
            </Button>
            <Button size="sm" variant="secondary" onClick={() => void onRun("business_output_golden")} disabled={running}>
              <RefreshCw className={`mr-1 h-3.5 w-3.5 ${running ? "animate-spin" : ""}`} />
              业务输出回归
            </Button>
          </div>
        </div>
        <CardDescription>
          Agent 安全回归检查基础读写链路；核心 skill 冒烟回归检查核心业务 skill 是否存在、可读、版本文件非空；业务依赖回归进一步检查流转单索引、重量表、包装资料、标签模板和共享存储目录；业务输出回归会用真实初始订单生成文件并对比完成样例评分。
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
                  {new Date(run.completed_at).toLocaleString()} · {run.passed}/{run.total} 通过
                </p>
              </div>
              {run.status === "passed" ? (
                <CheckCircle2 className="h-5 w-5 text-emerald-500" />
              ) : (
                <XCircle className="h-5 w-5 text-destructive" />
              )}
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
        description="集中查看用户反馈、演进指标、待审建议、沙箱回归和审计回滚。当前目标是受控进化，核心业务能力必须先测试、再审批、再发布。"
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
              <CardTitle>自治进化目标闭环</CardTitle>
            </div>
            <CardDescription>
              从业务执行、用户反馈、纠错暂存，到建议生成、沙箱回归、人工审批、发布和回滚，逐步形成可控的长期迭代闭环。
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
            <CardDescription>按 Agent 查看反馈、指标、建议、回归测试和审计记录。</CardDescription>
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
                  <CardTitle>演进建议审批</CardTitle>
                </div>
                <CardDescription>审批后才会进入下一步。新增自定义 skill 走轻量审批，核心 skill、业务依赖和源码级改动仍必须人工复核，不能由建议直接覆盖线上。</CardDescription>
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
          </TabsContent>

          <TabsContent value="roadmap" className="mt-4">
            <Card>
              <CardHeader>
                <div className="flex items-center gap-2">
                  <ClipboardList className="h-5 w-5 text-primary" />
                  <CardTitle>完整自治进化路线</CardTitle>
                </div>
                <CardDescription>已接入能力和待开发能力放在同一条路线里看，避免独立卡片占用页面空间。</CardDescription>
              </CardHeader>
              <CardContent className="space-y-6">
                <RoadmapList title="已接入和建设中" items={autonomousRoadmap} />
                <RoadmapList title="还需要继续开发" items={pendingRoadmap} />
              </CardContent>
            </Card>
          </TabsContent>
        </Tabs>
      </div>
    </div>
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
              <Badge variant={pipelineVariant(item.state)}>{item.state}</Badge>
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
