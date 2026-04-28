import { Activity, Beaker, CheckCircle2, ClipboardList, GitBranch, MessageSquareWarning, Sparkles } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { PageHeader } from "@/components/shared/page-header";

const pipeline = [
  {
    title: "用户反馈入口",
    description: "接收聊天中的有用、没用、需要纠正等反馈，作为进化闭环的原始信号。",
    icon: MessageSquareWarning,
    status: "规划中",
  },
  {
    title: "纠错记忆",
    description: "把有效纠错沉淀为即时提示和相似问题召回，避免同类错误反复出现。",
    icon: ClipboardList,
    status: "规划中",
  },
  {
    title: "演进建议",
    description: "聚合反馈、工具指标和历史失败案例，生成可审批的 skill 或配置改进建议。",
    icon: GitBranch,
    status: "已有基础",
  },
  {
    title: "沙箱测试",
    description: "使用历史订单、标准完成文件和失败样例做回归验证，测试不过不允许发布。",
    icon: Beaker,
    status: "规划中",
  },
  {
    title: "审批发布",
    description: "自定义 skill 可自动版本化；核心 skill、工具、模型、租户和源码变更必须审批。",
    icon: CheckCircle2,
    status: "已有基础",
  },
  {
    title: "审计回滚",
    description: "记录每次进化的来源、差异、测试结果、发布人和回滚点。",
    icon: Activity,
    status: "规划中",
  },
];

export function EvolutionCenterPage() {
  return (
    <div className="p-4 sm:p-6 pb-10">
      <PageHeader
        title="智能进化中心"
        description="管理用户反馈、纠错记忆、演进建议、沙箱测试、审批发布和回滚记录。"
        actions={<Badge variant="secondary">管理员可见</Badge>}
      />

      <div className="mt-4 grid gap-4 lg:grid-cols-[1.1fr_0.9fr]">
        <Card>
          <CardHeader>
            <div className="flex items-center gap-2">
              <Sparkles className="h-5 w-5 text-amber-500" />
              <CardTitle>目标闭环</CardTitle>
            </div>
            <CardDescription>
              GoClaw 从业务执行中收集反馈，经过验证后把稳定改进发布回 skill、工具或配置。
            </CardDescription>
          </CardHeader>
          <CardContent>
            <div className="rounded-lg border bg-muted/30 p-4 text-sm leading-7">
              <p>业务执行 → 用户反馈/系统指标 → 纠错暂存 → 记忆召回 → 演进建议 → 沙箱测试 → 审批发布 → 线上回归 → 审计回滚</p>
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>执行边界</CardTitle>
            <CardDescription>先建立控制台入口，后续分批接入 capability-evolver 和反馈链路。</CardDescription>
          </CardHeader>
          <CardContent className="space-y-3 text-sm">
            <div className="rounded-md border p-3">
              <p className="font-medium">允许自动化</p>
              <p className="mt-1 text-muted-foreground">自定义 skill 在测试通过后可以自动生成新版本并记录回滚点。</p>
            </div>
            <div className="rounded-md border p-3">
              <p className="font-medium">必须审批</p>
              <p className="mt-1 text-muted-foreground">核心 skill、工具、模型配置、租户权限、数据库结构和源码变更。</p>
            </div>
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
                  <Badge variant={item.status === "已有基础" ? "outline" : "secondary"}>{item.status}</Badge>
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
    </div>
  );
}
