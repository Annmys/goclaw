import { useMemo, useState } from "react";
import {
  AlertCircle,
  CheckCircle2,
  Download,
  FileText,
  Loader2,
  MessageSquare,
  RefreshCw,
  Route,
  Send,
  Settings2,
} from "lucide-react";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { PageHeader } from "@/components/shared/page-header";
import { EmptyState } from "@/components/shared/empty-state";
import { SearchInput } from "@/components/shared/search-input";
import { ChatInput, type AttachedFile } from "@/components/chat/chat-input";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { Separator } from "@/components/ui/separator";
import { useDeferredLoading } from "@/hooks/use-deferred-loading";
import { useMinLoading } from "@/hooks/use-min-loading";
import { cn } from "@/lib/utils";
import { formatDate } from "@/lib/format";
import { useWorkflow } from "./hooks/use-workflow";
import type { WorkflowDefinition, WorkflowMessage, WorkflowMissingField, WorkflowRun, WorkflowStepRun } from "@/types/workflow";

type WorkflowChatMessage = {
  id: string;
  role: "user" | "assistant";
  content: string;
  files?: string[];
  runId?: string;
  kind?: "chat" | "workflow";
  createdAt: number;
};

type LocalWorkflowStage = {
  id: string;
  label: string;
  status: "pending" | "running" | "completed" | "failed";
  detail?: string;
  startedAt?: number;
  completedAt?: number;
};

function statusBadge(status: WorkflowRun["status"] | WorkflowStepRun["status"]) {
  switch (status) {
    case "completed":
      return "success";
    case "waiting_user_input":
    case "paused":
      return "warning";
    case "failed":
      return "destructive";
    case "running":
      return "info";
    default:
      return "secondary";
  }
}

function statusText(status: WorkflowRun["status"]) {
  switch (status) {
    case "completed":
      return "已完成";
    case "waiting_user_input":
      return "等待补齐";
    case "failed":
      return "失败";
    case "running":
      return "执行中";
    default:
      return status;
  }
}

function formatNode(node: WorkflowStepRun) {
  return `${node.node_label}-${node.instance_no}`;
}

function fileTypeFromName(name: string) {
  const ext = name.split(".").pop();
  return ext && ext !== name ? ext.toLowerCase() : "";
}

function hasWorkflowIntent(intent: string) {
  const text = intent.toLowerCase();
  return [
    "workflow",
    "预估箱单",
    "箱单",
    "装箱单",
    "包装清单",
    "包装计算",
    "标签生成",
    "标签",
    "packing",
    "packing list",
    "label",
  ].some((keyword) => text.includes(keyword.toLowerCase()));
}

function missingFields(run: WorkflowRun | null) {
  return run?.steps.find((step) => step.status === "waiting_user_input" && step.missing?.length)?.missing ?? [];
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function readString(record: Record<string, unknown> | null, key: string) {
  const value = record?.[key];
  return typeof value === "string" ? value : "";
}

function readNumber(record: Record<string, unknown> | null, key: string) {
  const value = record?.[key];
  return typeof value === "number" ? value : null;
}

function localStageBadge(status: LocalWorkflowStage["status"]) {
  switch (status) {
    case "completed":
      return "success";
    case "running":
      return "info";
    case "failed":
      return "destructive";
    default:
      return "secondary";
  }
}

function localStageText(status: LocalWorkflowStage["status"]) {
  switch (status) {
    case "completed":
      return "完成";
    case "running":
      return "执行中";
    case "failed":
      return "失败";
    default:
      return "等待";
  }
}

function elapsedMs(startedAt?: number, completedAt?: number) {
  if (!startedAt) return "";
  const end = completedAt ?? Date.now();
  return `${Math.max(0, end - startedAt)}ms`;
}

function workflowResultSummary(run: WorkflowRun) {
  const output = run.output;
  if (!output) {
    return {
      resultFile: "",
      orderNo: "",
      rowCount: 0,
      previewCount: 0,
      warningCount: 0,
      confidenceFloor: "",
    };
  }
  const rows = Array.isArray(output.rows) ? output.rows : [];
  const preview = Array.isArray(output.preview) ? output.preview : [];
  const warnings = Array.isArray(output.warnings) ? output.warnings : [];
  const order = isRecord(output.order) ? output.order : null;
  const quality = isRecord(output.quality) ? output.quality : null;
  const confidence = readNumber(quality, "confidence_floor");
  return {
    resultFile: typeof output.result_file === "string" ? output.result_file : "",
    orderNo: readString(order, "order_no"),
    rowCount: rows.length,
    previewCount: preview.length,
    warningCount: warnings.length,
    confidenceFloor: confidence !== null ? confidence.toFixed(2) : "",
  };
}

function workflowMessageFromApi(message: WorkflowMessage): WorkflowChatMessage {
  return {
    id: message.id,
    role: message.role,
    content: message.content,
    files: message.files,
    runId: message.run_id,
    kind: message.kind,
    createdAt: Date.parse(message.created_at) || Date.now(),
  };
}

function workflowAssistantMessage(run: WorkflowRun): WorkflowChatMessage {
  let content = `任务已接收，当前状态：${statusText(run.status)}。`;
  if (run.status === "waiting_user_input") {
    content = "流程已匹配，当前缺少必要字段，请先完成下面的联动补齐。";
  } else if (run.status === "completed") {
    content = "结果已输出，下面是本次 workflow 的执行记录和结果。";
  } else if (run.status === "failed") {
    content = "workflow 执行失败，下面是失败节点和错误信息。";
  }
  return {
    id: `run-assistant-${run.id}`,
    role: "assistant",
    content,
    runId: run.id,
    kind: "workflow",
    createdAt: Date.parse(run.updated_at) || Date.now(),
  };
}

function MissingFieldEditor({
  fields,
  onSubmit,
  submitting,
}: {
  fields: WorkflowMissingField[];
  onSubmit: (values: Record<string, string>) => void;
  submitting?: boolean;
}) {
  const [values, setValues] = useState<Record<string, string>>({});
  const [selectedFieldKey, setSelectedFieldKey] = useState(fields[0]?.key ?? "");
  const selectedField = fields.find((field) => field.key === selectedFieldKey) ?? fields[0] ?? null;
  const selectedOption = selectedField?.details?.find((detail) => detail.value === values[selectedField.key]);
  const requiredFields = fields.filter((field) => field.required);
  const filledRequiredCount = requiredFields.filter((field) => {
    const value = values[field.key];
    return typeof value === "string" && value.trim().length > 0;
  }).length;
  const missingRequired = filledRequiredCount < requiredFields.length;

  return (
    <div className="grid gap-4 lg:grid-cols-[minmax(0,1fr)_280px]">
      <div className="grid gap-3">
        <div className="flex flex-wrap items-center justify-between gap-2">
          <div className="flex items-center gap-2 text-sm font-medium">
            <AlertCircle className="h-4 w-4 text-warning" />
            需要补齐后继续执行
          </div>
          <Badge variant={missingRequired ? "warning" : "success"}>
            已选 {filledRequiredCount}/{requiredFields.length}
          </Badge>
        </div>
        {fields.map((field) => {
          const currentValue = values[field.key] ?? "";
          const currentDetail = field.details?.find((item) => item.value === currentValue);
          const isSelected = selectedFieldKey === field.key;
          return (
            <div
              key={field.key}
              className={cn(
                "grid gap-3 rounded-lg border bg-background p-3 transition",
                isSelected && "border-primary/50 bg-primary/5",
              )}
            >
              <div className="flex flex-wrap items-center justify-between gap-2">
                <div className="grid gap-0.5">
                  <div className="text-sm font-medium">{field.label}</div>
                  {field.description && <div className="text-xs text-muted-foreground">{field.description}</div>}
                </div>
                <Badge variant={field.required ? "warning" : "secondary"}>{field.required ? "必填" : "可选"}</Badge>
              </div>
              {field.kind === "select" && field.options?.length ? (
                <div className="grid gap-2 sm:grid-cols-3">
                  {field.options.map((option) => {
                    const optionDetail = field.details?.find((item) => item.value === option);
                    const active = currentValue === option;
                    return (
                      <button
                        key={option}
                        type="button"
                        onClick={() => {
                          setSelectedFieldKey(field.key);
                          setValues((prev) => ({ ...prev, [field.key]: option }));
                        }}
                        className={cn(
                          "grid min-h-24 gap-2 rounded-lg border p-3 text-left transition",
                          active ? "border-primary bg-primary/5" : "hover:border-primary/30 hover:bg-muted/30",
                        )}
                      >
                        <div className="flex items-center justify-between gap-2">
                          <div className="text-sm font-medium">{optionDetail?.title ?? option}</div>
                          {active ? <Badge variant="default">已选</Badge> : <Badge variant="outline">选项</Badge>}
                        </div>
                        <div className="text-xs text-muted-foreground line-clamp-2">
                          {optionDetail?.description ?? "点击查看联动说明"}
                        </div>
                        {optionDetail?.highlights?.length ? (
                          <div className="grid gap-1 text-xs text-muted-foreground">
                            {optionDetail.highlights.slice(0, 2).map((item) => (
                              <div key={item}>• {item}</div>
                            ))}
                          </div>
                        ) : null}
                      </button>
                    );
                  })}
                </div>
              ) : (
                <Input
                  value={currentValue}
                  onFocus={() => setSelectedFieldKey(field.key)}
                  onChange={(e) => {
                    setSelectedFieldKey(field.key);
                    setValues((prev) => ({ ...prev, [field.key]: e.target.value }));
                  }}
                  placeholder={`输入${field.label}`}
                />
              )}
              {currentValue && currentDetail ? (
                <div className="rounded-md border bg-muted/20 p-3 text-xs text-muted-foreground">
                  <div className="mb-1 font-medium text-foreground">{currentDetail.title ?? currentValue}</div>
                  <div>{currentDetail.description}</div>
                </div>
              ) : null}
            </div>
          );
        })}
        {missingRequired ? (
          <div className="rounded-md border border-warning/30 bg-warning/10 p-2 text-xs text-warning">
            先把必填项选完，系统会把这些选择写入当前 workflow 后继续执行。
          </div>
        ) : null}
        <Button onClick={() => onSubmit(values)} disabled={submitting || missingRequired} className="gap-2 justify-self-start">
          {submitting ? <Loader2 className="h-4 w-4 animate-spin" /> : <Send className="h-4 w-4" />}
          继续执行
        </Button>
      </div>
      <div className="grid gap-3 rounded-lg border bg-muted/20 p-3">
        <div className="flex items-center justify-between gap-2">
          <div className="text-sm font-medium">联动展示</div>
          {selectedOption ? <Badge variant="outline">已联动</Badge> : <Badge variant="secondary">待选择</Badge>}
        </div>
        {selectedField ? (
          <>
            <div className="text-sm text-muted-foreground">
              当前字段：<span className="text-foreground">{selectedField.label}</span>
            </div>
            {selectedField.description ? <div className="text-sm text-muted-foreground">{selectedField.description}</div> : null}
            {selectedField.kind === "select" && selectedField.details?.length ? (
              <div className="grid gap-2">
                {selectedField.details.map((detail) => {
                  const active = values[selectedField.key] === detail.value;
                  return (
                    <button
                      key={detail.value}
                      type="button"
                      onClick={() => setValues((prev) => ({ ...prev, [selectedField.key]: detail.value }))}
                      className={cn(
                        "grid gap-1 rounded-md border p-3 text-left transition",
                        active ? "border-primary bg-primary/5" : "hover:border-primary/30 hover:bg-background",
                      )}
                    >
                      <div className="flex items-center justify-between gap-2">
                        <span className="text-sm font-medium">{detail.title ?? detail.value}</span>
                        {active ? <Badge variant="default">当前</Badge> : <Badge variant="outline">切换</Badge>}
                      </div>
                      <div className="text-xs text-muted-foreground">{detail.description}</div>
                      {detail.highlights?.length ? (
                        <div className="grid gap-1 text-xs text-muted-foreground">
                          {detail.highlights.map((item) => (
                            <div key={item}>• {item}</div>
                          ))}
                        </div>
                      ) : null}
                    </button>
                  );
                })}
              </div>
            ) : null}
            {selectedOption ? (
              <div className="grid gap-2 rounded-md border bg-background p-3 text-xs text-muted-foreground">
                <div className="mb-1 font-medium text-foreground">{selectedOption.title ?? selectedOption.value}</div>
                <div>{selectedOption.description}</div>
                <div className="rounded border bg-muted/30 p-2">
                  将写入字段：<span className="text-foreground">{selectedField.key}</span> ={" "}
                  <span className="text-foreground">{selectedOption.value}</span>
                </div>
                {selectedOption.highlights?.length ? (
                  <div className="grid gap-1">
                    {selectedOption.highlights.map((item) => (
                      <div key={item}>影响：{item}</div>
                    ))}
                  </div>
                ) : null}
              </div>
            ) : null}
          </>
        ) : (
          <div className="text-sm text-muted-foreground">选择一个字段后，这里会显示它的联动说明和选项详情。</div>
        )}
      </div>
    </div>
  );
}

function WorkflowResult({ run }: { run: WorkflowRun }) {
  if (!run.output) return null;
  const summary = workflowResultSummary(run);
  const resultFile = typeof run.output.result_file === "string" ? run.output.result_file : "";
  const rows = Array.isArray(run.output.rows) ? run.output.rows : [];
  const preview = Array.isArray(run.output.preview) ? run.output.preview : [];
  const warnings = Array.isArray(run.output.warnings) ? run.output.warnings : [];
  const order = isRecord(run.output.order) ? run.output.order : null;
  const orderNo = readString(order, "order_no");

  return (
    <div className="grid gap-3 rounded-lg border bg-muted/20 p-3 text-sm">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div className="flex items-center gap-2 font-medium">
          <CheckCircle2 className="h-4 w-4 text-success" />
          输出结果
        </div>
        <Badge variant="success">已生成</Badge>
      </div>
      <div className="grid gap-2 sm:grid-cols-4">
        <div className="rounded-md border bg-background p-2">
          <div className="text-xs text-muted-foreground">订单号</div>
          <div className="truncate font-medium">{summary.orderNo || "-"}</div>
        </div>
        <div className="rounded-md border bg-background p-2">
          <div className="text-xs text-muted-foreground">结果行数</div>
          <div className="font-medium">{summary.rowCount || summary.previewCount || 0}</div>
        </div>
        <div className="rounded-md border bg-background p-2">
          <div className="text-xs text-muted-foreground">最低置信度</div>
          <div className="font-medium">{summary.confidenceFloor || "-"}</div>
        </div>
        <div className="rounded-md border bg-background p-2">
          <div className="text-xs text-muted-foreground">警告</div>
          <div className="font-medium">{summary.warningCount}</div>
        </div>
      </div>
      {orderNo && (
        <div className="text-muted-foreground">
          订单号：<span className="text-foreground">{orderNo}</span>
        </div>
      )}
      {resultFile && (
        <div className="flex items-start gap-2 rounded-md border bg-background p-2 text-muted-foreground">
          <Download className="mt-0.5 h-4 w-4 shrink-0 text-primary" />
          <div className="min-w-0">
            <div className="text-xs">输出文件</div>
            <div className="truncate text-foreground">{resultFile}</div>
          </div>
        </div>
      )}
      {summary.confidenceFloor && (
        <div className="text-muted-foreground">
          最低置信度：<span className="text-foreground">{summary.confidenceFloor}</span>
        </div>
      )}
      {warnings.length > 0 && (
        <div className="grid gap-1 rounded-md border border-amber-500/20 bg-amber-500/10 p-2 text-xs text-amber-700 dark:text-amber-300">
          {warnings.map((warning) => (
            <div key={warning}>{warning}</div>
          ))}
        </div>
      )}
      {rows.length > 0 && <pre className="max-h-48 overflow-auto rounded-md bg-background p-2 text-xs">{JSON.stringify(rows, null, 2)}</pre>}
      {preview.length > 0 && <pre className="max-h-48 overflow-auto rounded-md bg-background p-2 text-xs">{JSON.stringify(preview, null, 2)}</pre>}
      {!resultFile && rows.length === 0 && preview.length === 0 && (
        <pre className="max-h-48 overflow-auto rounded-md bg-background p-2 text-xs">{JSON.stringify(run.output, null, 2)}</pre>
      )}
    </div>
  );
}

function WorkflowOutputNotice({ run }: { run: WorkflowRun }) {
  if (run.status !== "completed" || !run.output) return null;
  const summary = workflowResultSummary(run);
  const count = summary.rowCount || summary.previewCount;

  return (
    <div className="grid gap-3 rounded-lg border border-success/30 bg-success/5 p-3 text-sm">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div className="flex items-center gap-2 font-medium">
          <CheckCircle2 className="h-4 w-4 text-success" />
          结果已输出
        </div>
        <Badge variant="success">完成</Badge>
      </div>
      <div className="grid gap-2 text-muted-foreground sm:grid-cols-2">
        {summary.orderNo ? (
          <div>
            订单号：<span className="text-foreground">{summary.orderNo}</span>
          </div>
        ) : null}
        <div>
          输出数量：<span className="text-foreground">{count}</span>
        </div>
        {summary.confidenceFloor ? (
          <div>
            最低置信度：<span className="text-foreground">{summary.confidenceFloor}</span>
          </div>
        ) : null}
        <div>
          警告：<span className="text-foreground">{summary.warningCount}</span>
        </div>
      </div>
      {summary.resultFile ? (
        <div className="flex items-start gap-2 rounded-md border bg-background p-2 text-muted-foreground">
          <Download className="mt-0.5 h-4 w-4 shrink-0 text-primary" />
          <div className="min-w-0">
            <div className="text-xs">结果文件</div>
            <div className="truncate text-foreground">{summary.resultFile}</div>
          </div>
        </div>
      ) : null}
    </div>
  );
}

function WorkflowStepFeed({ run }: { run: WorkflowRun }) {
  const summary = workflowResultSummary(run);
  return (
    <div className="grid gap-2 rounded-lg border bg-background p-3 text-sm">
      <div className="flex items-center gap-2 font-medium">
        <CheckCircle2 className="h-4 w-4 text-success" />
        {run.status === "completed" ? "结果已生成" : "流程已接收"}
      </div>
      <div className="grid gap-2 text-xs text-muted-foreground sm:grid-cols-2">
        <div>
          当前状态：<span className="text-foreground">{statusText(run.status)}</span>
        </div>
        <div>
          当前步骤：<span className="text-foreground">{run.steps.find((step) => step.status !== "completed")?.node_name ?? "已完成"}</span>
        </div>
        {summary.orderNo ? (
          <div>
            订单号：<span className="text-foreground">{summary.orderNo}</span>
          </div>
        ) : null}
        <div>
          任务文件数：<span className="text-foreground">{run.artifacts?.length ?? 0}</span>
        </div>
      </div>
      {run.status === "waiting_user_input" ? (
        <div className="rounded-md border border-warning/30 bg-warning/10 p-2 text-xs text-warning">
          任务已暂停，等待补齐字段后继续。
        </div>
      ) : null}
      {run.status === "running" ? (
        <div className="rounded-md border bg-muted/20 p-2 text-xs text-muted-foreground">
          流程正在执行中，结果生成后会在这里直接显示。
        </div>
      ) : null}
      {run.status === "completed" ? (
        <div className="rounded-md border border-success/30 bg-success/10 p-2 text-xs text-success">
          结果已输出，包含 {summary.rowCount || summary.previewCount || 0} 条记录。
        </div>
      ) : null}
    </div>
  );
}

function WorkflowProgress({ run, compact = false }: { run: WorkflowRun; compact?: boolean }) {
  const completedCount = run.steps.filter((step) => step.status === "completed").length;
  const activeStep =
    run.steps.find((step) => step.status === "waiting_user_input") ??
    run.steps.find((step) => step.status === "running") ??
    run.steps.find((step) => step.status === "failed") ??
    run.steps.find((step) => step.status !== "completed") ??
    run.steps[run.steps.length - 1] ??
    null;
  const recentEvents = (run.events ?? []).slice(-3);
  const waitingFields = activeStep?.missing ?? [];

  return (
    <div className={cn("grid gap-3 rounded-lg border bg-muted/20 p-3 text-sm", compact && "bg-background")}>
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div className="flex items-center gap-2 font-medium">
          {run.status === "running" ? <Loader2 className="h-4 w-4 animate-spin text-primary" /> : <Route className="h-4 w-4 text-primary" />}
          进度记录
        </div>
        <Badge variant={statusBadge(run.status)}>{statusText(run.status)}</Badge>
      </div>
      <div className="grid gap-1 text-xs text-muted-foreground">
        <div>
          已完成 <span className="text-foreground">{completedCount}</span> / {run.steps.length}
        </div>
        {activeStep ? (
          <div>
            当前步骤：<span className="text-foreground">{formatNode(activeStep)}</span> · {activeStep.node_name}
          </div>
        ) : null}
        {run.status === "waiting_user_input" && waitingFields.length > 0 ? (
          <div>等待补齐：{waitingFields.map((field) => field.label).join("、")}</div>
        ) : null}
      </div>
      {!compact && (
        <div className="grid gap-2">
          {run.steps.map((step) => (
            <div key={step.id} className="rounded-md border bg-background p-2.5">
              <div className="flex flex-wrap items-center gap-2">
                <Badge variant={statusBadge(step.status)}>{step.status}</Badge>
                <span className="text-sm font-medium">{formatNode(step)}</span>
                <span className="text-sm text-muted-foreground">{step.node_name}</span>
              </div>
              {step.status === "waiting_user_input" && step.missing?.length ? (
                <div className="mt-2 text-xs text-warning">
                  缺少：{step.missing.map((field) => field.label).join("、")}
                </div>
              ) : null}
              {step.output && <pre className="mt-2 max-h-28 overflow-auto rounded bg-muted p-2 text-xs">{JSON.stringify(step.output, null, 2)}</pre>}
              {step.error && <div className="mt-2 text-xs text-destructive">{step.error}</div>}
            </div>
          ))}
        </div>
      )}
      {recentEvents.length > 0 && (
        <div className="grid gap-2 rounded-md border bg-background p-2">
          <div className="text-xs font-medium text-muted-foreground">最近事件</div>
          {recentEvents.map((event) => (
            <div key={event.id} className="grid gap-0.5 text-xs">
              <div className="flex flex-wrap items-center gap-2">
                <Badge variant="outline">{event.type}</Badge>
                {event.node_id ? <span className="text-muted-foreground">{event.node_id}</span> : null}
              </div>
              <div>{event.message}</div>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}

function WorkflowLocalProgress({ stages }: { stages: LocalWorkflowStage[] }) {
  if (stages.length === 0) return null;
  return (
    <div className="grid gap-3 rounded-lg border bg-background p-3 text-sm">
      <div className="flex items-center gap-2 font-medium">
        <Loader2 className="h-4 w-4 animate-spin text-primary" />
        任务过程
      </div>
      <div className="grid gap-2">
        {stages.map((stage) => (
          <div key={stage.id} className="flex flex-wrap items-center gap-2 rounded-md border bg-muted/20 px-3 py-2">
            <Badge variant={localStageBadge(stage.status)}>{localStageText(stage.status)}</Badge>
            <span className="text-sm font-medium">{stage.label}</span>
            {stage.detail ? <span className="text-xs text-muted-foreground">{stage.detail}</span> : null}
            {stage.startedAt ? <span className="ml-auto text-xs text-muted-foreground">{elapsedMs(stage.startedAt, stage.completedAt)}</span> : null}
          </div>
        ))}
      </div>
    </div>
  );
}

function WorkflowFeedbackBox({
  run,
  submitting,
  onSubmit,
}: {
  run: WorkflowRun;
  submitting?: boolean;
  onSubmit: (message: string, stepId?: string) => void;
}) {
  const [message, setMessage] = useState("");
  const [stepId, setStepId] = useState(run.steps.find((step) => step.status === "failed")?.id ?? run.steps[run.steps.length - 1]?.id ?? "");
  return (
    <div className="grid gap-3 rounded-lg border bg-muted/20 p-3">
      <div className="text-sm font-medium">用户反馈修缮</div>
      <Textarea
        value={message}
        onChange={(e) => setMessage(e.target.value)}
        placeholder="例如：结果里少了某一列、格式不对、某一步字段填错了"
        className="min-h-24"
      />
      <Select value={stepId || "auto"} onValueChange={(value) => setStepId(value === "auto" ? "" : value)}>
        <SelectTrigger className="w-full">
          <SelectValue placeholder="关联步骤（可选）" />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value="auto">自动关联最近步骤</SelectItem>
          {run.steps.map((step) => (
            <SelectItem key={step.id} value={step.id}>
              {formatNode(step)} {step.node_name}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
      <Button
        onClick={() => {
          const text = message.trim();
          if (!text) return;
          onSubmit(text, stepId || undefined);
          setMessage("");
        }}
        disabled={submitting || message.trim().length === 0}
        className="justify-self-start gap-2"
      >
        {submitting ? <Loader2 className="h-4 w-4 animate-spin" /> : <Send className="h-4 w-4" />}
        提交反馈
      </Button>
    </div>
  );
}

function WorkflowDetailsDialog({
  open,
  onOpenChange,
  definitions,
  runs,
  selectedWorkflowId,
  search,
  activeRunId,
  showSkeleton,
  onSearchChange,
  onWorkflowChange,
  onRunChange,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  definitions: WorkflowDefinition[];
  runs: WorkflowRun[];
  selectedWorkflowId: string;
  search: string;
  activeRunId: string | null;
  showSkeleton: boolean;
  onSearchChange: (value: string) => void;
  onWorkflowChange: (value: string) => void;
  onRunChange: (runId: string) => void;
}) {
  const filteredDefinitions = useMemo(() => {
    const q = search.toLowerCase();
    return definitions.filter(
      (def) => def.name.toLowerCase().includes(q) || def.description.toLowerCase().includes(q) || def.domain.toLowerCase().includes(q),
    );
  }, [definitions, search]);

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-3xl max-h-[85vh] flex flex-col">
        <DialogHeader className="flex-row items-center justify-between gap-2">
          <DialogTitle className="flex items-center gap-2 truncate text-base">
            <Route className="h-4 w-4" />
            流程与执行记录
          </DialogTitle>
        </DialogHeader>
        <Tabs defaultValue="runs" className="min-h-0 flex-1 overflow-hidden">
          <TabsList>
            <TabsTrigger value="runs">运行记录</TabsTrigger>
            <TabsTrigger value="definitions">流程定义</TabsTrigger>
          </TabsList>
          <TabsContent value="runs" className="mt-4 min-h-0 flex-1 overflow-y-auto pr-1">
            <div className="grid gap-3">
              {runs.length === 0 ? (
                <EmptyState icon={Route} title="没有运行记录" description="先在主界面发送一个任务。" />
              ) : (
                runs.map((run) => (
                  <button
                    key={run.id}
                    type="button"
                    onClick={() => onRunChange(run.id)}
                    className={cn(
                      "rounded-xl border bg-card p-4 text-left transition hover:border-primary/40 hover:bg-muted/20",
                      activeRunId === run.id && "border-primary/50 bg-primary/5",
                    )}
                  >
                    <div className="flex flex-wrap items-center gap-2">
                      <Badge variant={statusBadge(run.status)}>{statusText(run.status)}</Badge>
                      <span className="font-medium">{run.workflow_name}</span>
                      <span className="text-xs text-muted-foreground">{run.id}</span>
                      <span className="text-xs text-muted-foreground">{formatDate(run.created_at)}</span>
                    </div>
                    <div className="mt-3 grid gap-2">
                      {run.steps.map((step) => (
                        <div key={step.id} className="rounded-md border bg-background p-3">
                          <div className="flex flex-wrap items-center gap-2">
                            <Badge variant={statusBadge(step.status)}>{step.status}</Badge>
                            <span className="text-sm font-medium">{formatNode(step)}</span>
                            <span className="text-sm text-muted-foreground">{step.node_name}</span>
                          </div>
                          {step.output && <pre className="mt-2 max-h-32 overflow-auto rounded bg-muted p-2 text-xs">{JSON.stringify(step.output, null, 2)}</pre>}
                          {step.error && <div className="mt-2 text-xs text-destructive">{step.error}</div>}
                        </div>
                      ))}
                    </div>
                  </button>
                ))
              )}
            </div>
          </TabsContent>
          <TabsContent value="definitions" className="mt-4 min-h-0 flex-1 overflow-y-auto pr-1">
            <div className="grid gap-3">
              <SearchInput value={search} onChange={onSearchChange} placeholder="搜索流程" className="max-w-none" />
              <Select value={selectedWorkflowId || "auto"} onValueChange={(value) => onWorkflowChange(value === "auto" ? "" : value)}>
                <SelectTrigger className="w-full">
                  <SelectValue placeholder="自动匹配 workflow" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="auto">自动匹配流程</SelectItem>
                  {filteredDefinitions.map((def) => (
                    <SelectItem key={def.id} value={def.id}>
                      {def.name} v{def.version}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              {showSkeleton ? (
                <div className="rounded-xl border p-6 text-sm text-muted-foreground">加载中...</div>
              ) : filteredDefinitions.length === 0 ? (
                <EmptyState icon={Route} title="没有流程定义" description="请先在后端注册 workflow 定义。" />
              ) : (
                filteredDefinitions.map((def) => (
                  <Card key={def.id} className="rounded-xl">
                    <CardHeader className="pb-3">
                      <div className="flex items-center justify-between gap-2">
                        <CardTitle className="text-base">{def.name}</CardTitle>
                        <Badge variant="outline">v{def.version}</Badge>
                      </div>
                      <CardDescription>{def.description}</CardDescription>
                    </CardHeader>
                    <CardContent className="grid gap-3">
                      <div className="flex flex-wrap gap-2">
                        <Badge variant="secondary">{def.domain}</Badge>
                        <Badge variant={def.active ? "success" : "secondary"}>{def.active ? "active" : "inactive"}</Badge>
                        <Badge variant="outline">{def.output.adapter}</Badge>
                      </div>
                      <Separator />
                      <div className="grid gap-2">
                        {def.nodes.map((node) => (
                          <div key={node.id} className="flex flex-wrap items-center gap-2 rounded-md border px-3 py-2 text-sm">
                            <Badge variant="outline">
                              {node.type_label}-{node.instance_no}
                            </Badge>
                            <span className="font-medium">{node.name}</span>
                            <span className="text-xs text-muted-foreground">{node.type}</span>
                          </div>
                        ))}
                      </div>
                    </CardContent>
                  </Card>
                ))
              )}
            </div>
          </TabsContent>
        </Tabs>
      </DialogContent>
    </Dialog>
  );
}

export function WorkflowPage() {
  const { t } = useTranslation("workflow");
  const { definitions, runs, messages, loading, fetching, refresh, matchWorkflow, startRun, appendMessage, uploadFile, resumeRun, submitFeedback } = useWorkflow();
  const spinning = useMinLoading(fetching || loading);
  const showSkeleton = useDeferredLoading(loading && definitions.length === 0);
  const [search, setSearch] = useState("");
  const [selectedWorkflowId, setSelectedWorkflowId] = useState<string>("");
  const [files, setFiles] = useState<AttachedFile[]>([]);
  const [busy, setBusy] = useState(false);
  const [activeRunId, setActiveRunId] = useState<string | null>(null);
  const [detailsOpen, setDetailsOpen] = useState(false);
  const [localRun, setLocalRun] = useState<WorkflowRun | null>(null);
  const [localStages, setLocalStages] = useState<LocalWorkflowStage[]>([]);
  const [optimisticMessages, setOptimisticMessages] = useState<WorkflowChatMessage[]>([]);

  const currentRun = runs.find((run) => run.id === activeRunId) ?? (localRun?.id === activeRunId ? localRun : null);
  const fieldsToFill = missingFields(currentRun);

  const workflowMessages = useMemo(() => {
    const apiMessages = messages.map(workflowMessageFromApi);
    return [...apiMessages, ...optimisticMessages].sort((a, b) => a.createdAt - b.createdAt);
  }, [messages, optimisticMessages]);

  const pushOptimisticMessage = (message: Omit<WorkflowChatMessage, "id" | "createdAt">) => {
    const local: WorkflowChatMessage = {
      ...message,
      id: `local-${Date.now()}-${Math.random().toString(36).slice(2)}`,
      createdAt: Date.now(),
    };
    setOptimisticMessages((prev) => [...prev, local]);
    return local;
  };

  const removeOptimisticMessage = (id: string) => {
    setOptimisticMessages((prev) => prev.filter((message) => message.id !== id));
  };

  const persistMessage = async (message: {
    role: "user" | "assistant";
    content: string;
    files?: string[];
    run_id?: string;
    kind?: "chat" | "workflow";
  }) => {
    const local = pushOptimisticMessage({
      role: message.role,
      content: message.content,
      files: message.files,
      runId: message.run_id,
      kind: message.kind ?? "chat",
    });
    await appendMessage(message);
    removeOptimisticMessage(local.id);
  };

  const appendAssistantMessage = async (content: string, run?: WorkflowRun) => {
    await persistMessage({
      role: "assistant",
      content,
      run_id: run?.id,
      kind: run ? "workflow" : "chat",
    });
  };

  const resolveWorkflowId = async (intent: string, fileName: string, fileType: string) => {
    if (selectedWorkflowId) return selectedWorkflowId;
    const matched = await matchWorkflow({ intent, file_name: fileName, file_type: fileType });
    if (!matched.matched || matched.candidates.length === 0) {
      throw new Error(matched.message || "没有匹配到可执行 workflow");
    }
    if (matched.needs_choice) {
      setDetailsOpen(true);
      throw new Error("匹配到多个流程，请在右上角“流程”里选择具体流程后再发送。");
    }
    const candidate = matched.candidates[0];
    if (!candidate) {
      throw new Error("没有匹配到可执行 workflow");
    }
    return candidate.workflow_id;
  };

  const handleSend = async (messageText: string, sendFiles?: AttachedFile[]) => {
    const intent = messageText.trim();
    const pendingFiles = [...(sendFiles ?? [])];
    if (!intent && pendingFiles.length === 0) return;
    setBusy(true);
    const now = Date.now();
    const hasFiles = pendingFiles.length > 0;
    const shouldTryWorkflow = hasFiles || Boolean(selectedWorkflowId) || hasWorkflowIntent(intent);
    setLocalStages([]);
    if (shouldTryWorkflow) {
      setLocalStages([
        {
          id: "upload",
          label: "上传文件",
          status: hasFiles ? "running" : "completed",
          detail: hasFiles ? `${pendingFiles.length} 个文件` : "无附件",
          startedAt: now,
          completedAt: hasFiles ? undefined : now,
        },
        { id: "match", label: "匹配流程", status: "pending" },
        { id: "start", label: "创建执行记录", status: "pending" },
        { id: "execute", label: "执行预估箱单流程", status: "pending" },
      ]);
    }

    try {
      await persistMessage({
        role: "user",
        content: intent || "处理上传文件",
        files: pendingFiles.map((item) => item.file.name),
        kind: "chat",
      });
      await appendAssistantMessage(shouldTryWorkflow ? "任务已接收，正在匹配流程。" : "消息已收到。没有识别到明确的 workflow 任务，所以不会打开执行面板。");
      if (!shouldTryWorkflow) {
        return;
      }

      const uploads = hasFiles ? await Promise.all(pendingFiles.map((item) => uploadFile(item.file))) : [];
      const uploadedAt = Date.now();
      setLocalStages((prev) =>
        prev.map((stage) =>
          stage.id === "upload"
            ? { ...stage, status: "completed", completedAt: uploadedAt }
            : stage.id === "match"
              ? { ...stage, status: "running", startedAt: uploadedAt }
              : stage,
        ),
      );
      const primaryUpload = uploads[0];
      const primaryFile = pendingFiles[0]?.file;
      const fileName = primaryUpload?.filename ?? primaryFile?.name ?? "";
      const fileType = fileTypeFromName(fileName) || primaryUpload?.mime_type || primaryFile?.type || "";
      const workflowId = await resolveWorkflowId(intent, fileName, fileType);
      const matchedAt = Date.now();
      setLocalStages((prev) =>
        prev.map((stage) =>
          stage.id === "match"
            ? { ...stage, status: "completed", detail: workflowId, completedAt: matchedAt }
            : stage.id === "start"
              ? { ...stage, status: "running", startedAt: matchedAt }
              : stage,
        ),
      );
      const run = await startRun({
        workflow_id: workflowId,
        intent,
        file_name: fileName,
        file_type: fileType,
        input: {
          message: intent,
          media: uploads.map((upload) => ({
            path: upload.path,
            filename: upload.filename,
            mime_type: upload.mime_type,
          })),
        },
      });
      const completedAt = Date.now();
      setLocalStages((prev) =>
        prev.map((stage) =>
          stage.id === "start"
            ? { ...stage, status: "completed", completedAt }
            : stage.id === "execute"
              ? { ...stage, status: run.status === "failed" ? "failed" : "completed", detail: statusText(run.status), startedAt: stage.startedAt ?? completedAt, completedAt }
              : stage,
        ),
      );
      setLocalRun(run);
      setActiveRunId(run.id);
      await appendAssistantMessage(workflowAssistantMessage(run).content, run);
    } catch (err) {
      const failedAt = Date.now();
      setLocalStages((prev) =>
        prev.map((stage) => (stage.status === "running" || stage.status === "pending" ? { ...stage, status: "failed", completedAt: failedAt } : stage)),
      );
      const message = err instanceof Error ? err.message : "启动 workflow 失败";
      await appendAssistantMessage(message);
    } finally {
      setBusy(false);
    }
  };
  const handleResume = async (values: Record<string, string>) => {
    if (!currentRun) return;
    setBusy(true);
    try {
      const next = await resumeRun(currentRun.id, values);
      setLocalRun(next);
      setActiveRunId(next.id);
      await appendAssistantMessage(workflowAssistantMessage(next).content, next);
    } catch (err) {
      await appendAssistantMessage(err instanceof Error ? err.message : "继续执行 workflow 失败");
    } finally {
      setBusy(false);
    }
  };

  const handleFeedback = async (messageText: string, stepId?: string) => {
    if (!currentRun) return;
    setBusy(true);
    try {
      await submitFeedback({ run_id: currentRun.id, step_id: stepId, message: messageText });
      await appendAssistantMessage("反馈已提交，流程维护官会据此修缮流程。", currentRun);
    } catch (err) {
      await appendAssistantMessage(err instanceof Error ? err.message : "提交反馈失败");
    } finally {
      setBusy(false);
    }
  };

  const workflowThreads = useMemo(() => {
    return [...workflowMessages]
      .map((msg) => {
        const runForMessage = msg.runId
          ? runs.find((run) => run.id === msg.runId) ?? (localRun?.id === msg.runId ? localRun : null)
          : null;
        return { msg, runForMessage };
      });
  }, [currentRun, localRun, runs, workflowMessages]);

  return (
    <div className="flex h-full min-h-[calc(100vh-4rem)] flex-col bg-background">
      <div className="border-b px-4 py-3 sm:px-6">
        <PageHeader
          title={t("title", "Workflow")}
          actions={
            <div className="flex items-center gap-2">
              <Button variant="outline" size="sm" onClick={() => refresh()} disabled={spinning} className="gap-1">
                <RefreshCw className={cn("h-3.5 w-3.5", fetching && "animate-spin")} />
                刷新
              </Button>
              <Button variant="outline" size="sm" onClick={() => setDetailsOpen(true)} className="gap-1">
                <Settings2 className="h-3.5 w-3.5" />
                流程
              </Button>
            </div>
          }
        />
      </div>

      <main className="mx-auto flex w-full max-w-4xl flex-1 flex-col overflow-hidden">
        <div className="flex-1 overflow-y-auto px-4 py-5 sm:px-6">
          {workflowThreads.length === 0 ? (
            <EmptyState
              icon={MessageSquare}
              title="发送文件开始 workflow"
              className="min-h-[45vh]"
            />
          ) : (
            <div className="grid gap-4">
              {workflowThreads.map(({ msg, runForMessage }) => (
                <div key={msg.id} className={cn("flex", msg.role === "user" ? "justify-end" : "justify-start")}>
                  <div
                    className={cn(
                      "max-w-[88%] rounded-2xl px-4 py-3 text-sm shadow-sm",
                      msg.role === "user" ? "bg-primary text-primary-foreground" : "border bg-card",
                    )}
                  >
                    <div className="whitespace-pre-wrap">{msg.content}</div>
                    {msg.files?.length ? (
                      <div className="mt-2 grid gap-1">
                        {msg.files.map((name) => (
                          <div key={name} className="inline-flex items-center gap-1 text-xs opacity-80">
                            <FileText className="h-3.5 w-3.5" />
                            <span className="truncate">{name}</span>
                          </div>
                        ))}
                      </div>
                    ) : null}
                    {msg.role === "assistant" && runForMessage && (
                      <div className="mt-3 grid gap-2">
                        <div className="flex flex-wrap items-center gap-2">
                          <Badge variant={statusBadge(runForMessage.status)}>{statusText(runForMessage.status)}</Badge>
                          <Badge variant="outline">{runForMessage.workflow_name}</Badge>
                        </div>
                        <WorkflowStepFeed run={runForMessage} />
                        <WorkflowProgress run={runForMessage} compact={runForMessage.status === "completed"} />
                        {runForMessage.status === "completed" && (
                          <>
                            <WorkflowOutputNotice run={runForMessage} />
                            <WorkflowResult run={runForMessage} />
                          </>
                        )}
                      </div>
                    )}
                  </div>
                </div>
              ))}
              {currentRun && fieldsToFill.length > 0 && (
                <div className="flex justify-start">
                  <div className="w-full max-w-[88%] rounded-2xl border bg-card px-4 py-3 shadow-sm">
                    <MissingFieldEditor fields={fieldsToFill} submitting={busy} onSubmit={handleResume} />
                  </div>
                </div>
              )}
              {currentRun && currentRun.status === "completed" && (
                <div className="flex justify-start">
                  <div className="w-full max-w-[88%] rounded-2xl border bg-card px-4 py-3 shadow-sm">
                    <WorkflowOutputNotice run={currentRun} />
                    <WorkflowFeedbackBox run={currentRun} submitting={busy} onSubmit={handleFeedback} />
                  </div>
                </div>
              )}
              {busy && localStages.length > 0 && (
                <div className="flex justify-start">
                  <div className="grid max-w-[88%] gap-3 rounded-2xl border bg-card px-4 py-3 text-sm text-muted-foreground shadow-sm">
                    <div className="inline-flex items-center gap-2">
                      <Loader2 className="h-4 w-4 animate-spin" />
                      正在处理...
                    </div>
                    <WorkflowLocalProgress stages={localStages} />
                  </div>
                </div>
              )}
            </div>
          )}
        </div>
        <ChatInput
          files={files}
          isBusy={busy}
          disabled={loading}
          onFilesChange={setFiles}
          onSend={handleSend}
          onAbort={() => undefined}

        />
      </main>

      <WorkflowDetailsDialog
        open={detailsOpen}
        onOpenChange={setDetailsOpen}
        definitions={definitions}
        runs={runs}
        selectedWorkflowId={selectedWorkflowId}
        search={search}
        activeRunId={activeRunId}
        showSkeleton={showSkeleton}
        onSearchChange={setSearch}
        onWorkflowChange={setSelectedWorkflowId}
        onRunChange={(runId) => {
          setActiveRunId(runId);
          setDetailsOpen(false);
        }}
      />
    </div>
  );
}


