import { useEffect } from "react";
import { useNavigate } from "react-router";
import { Plus, Play, Pencil, Trash2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import { PageHeader } from "@/components/shared/page-header";
import { EmptyState } from "@/components/shared/empty-state";
import { ROUTES } from "@/lib/routes";
import { useGraphDefinitions } from "./hooks/use-graph-definitions";

// WorkflowDefinitionsPage is the "流程库" menu: lists saved graph workflows with
// edit / run / delete actions, and a button to create a new one in the builder.
export function WorkflowDefinitionsPage() {
  const navigate = useNavigate();
  const { definitions, isLoading, deleteDefinition, seedEPLTemplate, refetch } = useGraphDefinitions();

  const openNew = () => navigate(ROUTES.WORKFLOW_BUILDER);
  const openEdit = (id: string) => navigate(ROUTES.WORKFLOW_BUILDER_EDIT.replace(":id", id));
  const openRun = (id: string) => navigate(ROUTES.WORKFLOW_CHAT_RUN.replace(":id", id));

  // Ensure the built-in EPL workflow exists in the library (idempotent),
  // so it shows up as a normal flow rather than behind a special button.
  useEffect(() => {
    if (!isLoading && !definitions.some((d) => d.name === "预估箱单制作(EPL)")) {
      seedEPLTemplate().then(() => refetch()).catch(() => {});
    }
  }, [isLoading, definitions, seedEPLTemplate, refetch]);

  return (
    <div className="flex h-full flex-col">
      <PageHeader title="流程库" actions={
        <Button size="sm" onClick={openNew}>
          <Plus className="mr-1 h-4 w-4" />
          新建工作流
        </Button>
      } />

      <div className="min-h-0 flex-1 overflow-y-auto p-4">
        {isLoading ? (
          <div className="text-sm text-muted-foreground">加载中…</div>
        ) : definitions.length === 0 ? (
          <EmptyState title="暂无工作流" description="点击右上角新建一个可视化工作流。" />
        ) : (
          <div className="space-y-2">
            {definitions.map((d) => (
              <div key={d.id} className="flex items-center justify-between rounded border px-3 py-2">
                <div className="min-w-0">
                  <div className="truncate text-sm font-medium">{d.name || "未命名"}</div>
                  <div className="truncate text-xs text-muted-foreground">
                    {(d.graph?.blocks?.length ?? 0)} 节点 · v{d.version ?? 1}
                  </div>
                </div>
                <div className="flex items-center gap-1">
                  <Button size="sm" variant="ghost" onClick={() => openRun(d.id)} title="运行(对话)">
                    <Play className="h-4 w-4" />
                  </Button>
                  <Button size="sm" variant="ghost" onClick={() => openEdit(d.id)} title="编辑">
                    <Pencil className="h-4 w-4" />
                  </Button>
                  <Button size="sm" variant="ghost" onClick={() => deleteDefinition(d.id)} title="删除">
                    <Trash2 className="h-4 w-4" />
                  </Button>
                </div>
              </div>
            ))}
          </div>
        )}
      </div>
    </div>
  );
}
