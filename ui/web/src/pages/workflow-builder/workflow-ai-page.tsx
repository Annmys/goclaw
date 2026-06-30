import { useCallback, useRef, useState, useEffect } from "react";
import { useNavigate } from "react-router";
import { Sparkles, Send, ArrowRight, RotateCcw, ChevronDown } from "lucide-react";
import { Button } from "@/components/ui/button";
import { PageHeader } from "@/components/shared/page-header";
import { ROUTES } from "@/lib/routes";
import { useGraphDefinitions } from "./hooks/use-graph-definitions";
import { useAuthStore } from "@/stores/use-auth-store";
import type { WorkflowGraph, GraphDefinition } from "@/types/workflow-graph";

interface ChatTurn {
  role: "user" | "assistant";
  text: string;
  graph?: WorkflowGraph;
}

const EXAMPLES = [
  { label: "新建流程", prompt: "接收用户消息,用智能体判断是否投诉,是投诉转人工,否则自动回复" },
  { label: "修改流程", prompt: "把预估箱单流程的G.W.计算改成自动查重量表" },
  { label: "提问", prompt: "预估箱单流程目前有哪些步骤?哪些是确定性的?" },
];

// WorkflowAIPage — "流程管理":新建流程、修改已有流程、对流程提问,三合一对话界面。
export function WorkflowAIPage() {
  const navigate = useNavigate();
  const { definitions, generateGraph, saveDefinition } = useGraphDefinitions();
  const userId = useAuthStore((s) => s.userId);
  const scrollRef = useRef<HTMLDivElement>(null);

  // Persistent chat history
  const storageKey = `wf-manage:${userId || "anon"}`;
  const [turns, setTurns] = useState<ChatTurn[]>(() => {
    try { const r = localStorage.getItem(storageKey); return r ? JSON.parse(r) : []; }
    catch { return []; }
  });
  useEffect(() => {
    try { localStorage.setItem(storageKey, JSON.stringify(turns)); } catch {}
  }, [turns, storageKey]);

  const [input, setInput] = useState("");
  const [busy, setBusy] = useState(false);
  const [selectedDef, setSelectedDef] = useState<GraphDefinition | null>(null);
  const [showDefPicker, setShowDefPicker] = useState(false);

  // The graph context: either selected existing workflow or the latest generated one
  const contextGraph = selectedDef?.graph || [...turns].reverse().find((t) => t.graph)?.graph;

  useEffect(() => {
    scrollRef.current?.scrollTo({ top: scrollRef.current.scrollHeight, behavior: "smooth" });
  }, [turns, busy]);

  const send = useCallback(
    async (text: string) => {
      const prompt = text.trim();
      if (!prompt || busy) return;
      setInput("");
      setTurns((t) => [...t, { role: "user", text: prompt }]);
      setBusy(true);
      try {
        const res = await generateGraph(prompt, contextGraph);
        setTurns((t) => [
          ...t,
          {
            role: "assistant",
            text: res.explanation || `已生成包含 ${res.graph.blocks.length} 个节点的工作流。`,
            graph: res.graph,
          },
        ]);
      } catch (err) {
        setTurns((t) => [...t, { role: "assistant", text: `处理失败:${(err as Error).message}` }]);
      } finally {
        setBusy(false);
      }
    },
    [busy, generateGraph, contextGraph],
  );

  const openInBuilder = useCallback(
    async (graph: WorkflowGraph, name: string) => {
      const saved = await saveDefinition({ name: name || "流程管理生成", graph });
      navigate(ROUTES.WORKFLOW_BUILDER_EDIT.replace(":id", saved.id));
    },
    [saveDefinition, navigate],
  );

  // Direct save: when modifying an existing workflow, save changes directly back to library
  const saveDirectly = useCallback(
    async (graph: WorkflowGraph) => {
      if (!selectedDef) return;
      await saveDefinition({ id: selectedDef.id, name: selectedDef.name, graph });
      setTurns((t) => [...t, { role: "assistant", text: `✅ 已保存修改到「${selectedDef.name}」流程库。` }]);
    },
    [selectedDef, saveDefinition],
  );

  const clearChat = () => {
    setTurns([]);
    setSelectedDef(null);
  };

  return (
    <div className="flex h-full flex-col">
      <PageHeader
        title="流程管理"
        actions={
          <div className="flex items-center gap-2">
            {/* Workflow selector: pick an existing workflow to modify */}
            <div className="relative">
              <Button size="sm" variant="outline" onClick={() => setShowDefPicker((v) => !v)}>
                {selectedDef ? selectedDef.name : "选择已有流程"}
                <ChevronDown className="ml-1 h-3 w-3" />
              </Button>
              {showDefPicker && (
                <div className="absolute right-0 top-full z-50 mt-1 max-h-60 w-64 overflow-y-auto rounded border bg-card shadow-lg">
                  <button
                    className="w-full px-3 py-2 text-left text-sm hover:bg-accent"
                    onClick={() => { setSelectedDef(null); setShowDefPicker(false); }}
                  >
                    新建流程(无上下文)
                  </button>
                  {definitions.map((d) => (
                    <button
                      key={d.id}
                      className="w-full truncate px-3 py-2 text-left text-sm hover:bg-accent"
                      onClick={() => { setSelectedDef(d); setShowDefPicker(false); }}
                    >
                      {d.name || "未命名"}
                    </button>
                  ))}
                </div>
              )}
            </div>
            {turns.length > 0 && (
              <Button size="sm" variant="outline" onClick={clearChat}>
                <RotateCcw className="mr-1 h-4 w-4" />
                清空
              </Button>
            )}
          </div>
        }
      />

      {/* Context indicator */}
      {selectedDef && (
        <div className="border-b bg-muted/50 px-4 py-1.5 text-xs text-muted-foreground">
          当前操作流程:「{selectedDef.name}」({selectedDef.graph?.blocks?.length || 0} 节点)— 你的修改建议会基于此流程
        </div>
      )}

      <div ref={scrollRef} className="min-h-0 flex-1 overflow-y-auto px-4 py-4">
        {turns.length === 0 ? (
          <div className="mx-auto max-w-2xl pt-10 text-center">
            <Sparkles className="mx-auto mb-3 h-10 w-10 text-violet-500" />
            <h2 className="mb-1 text-lg font-semibold">流程管理</h2>
            <p className="mb-6 text-sm text-muted-foreground">
              新建流程、修改已有流程、或对流程提问。选择一个已有流程后,你的指令会基于它修改。
            </p>
            <div className="space-y-2 text-left">
              {EXAMPLES.map((ex) => (
                <button
                  key={ex.prompt}
                  onClick={() => send(ex.prompt)}
                  className="flex w-full items-center gap-2 rounded-md border px-3 py-2 text-sm hover:bg-accent"
                >
                  <Sparkles className="h-3.5 w-3.5 shrink-0 text-violet-500" />
                  <span className="text-muted-foreground">[{ex.label}]</span>
                  {ex.prompt}
                </button>
              ))}
            </div>
          </div>
        ) : (
          <div className="mx-auto max-w-2xl space-y-3">
            {turns.map((turn, i) => (
              <div key={i} className={turn.role === "user" ? "flex justify-end" : "flex justify-start"}>
                <div
                  className={
                    turn.role === "user"
                      ? "max-w-[80%] rounded-lg bg-primary px-3 py-2 text-sm text-primary-foreground"
                      : "max-w-[85%] rounded-lg bg-muted px-3 py-2 text-sm"
                  }
                >
                  <div className="whitespace-pre-wrap">{turn.text}</div>
                  {turn.graph ? (
                    <div className="mt-2 rounded border bg-background p-2">
                      <div className="mb-1.5 text-xs text-muted-foreground">
                        工作流:{turn.graph.blocks.length} 节点 · {turn.graph.connections.length} 连线
                      </div>
                      <div className="mb-2 flex flex-wrap gap-1">
                        {turn.graph.blocks.slice(0, 12).map((b) => (
                          <span key={b.id} className="rounded bg-muted px-1.5 py-0.5 text-[10px]">
                            {b.metadata?.name || b.metadata?.id}
                          </span>
                        ))}
                      </div>
                      <div className="flex gap-2">
                        <Button size="sm" onClick={() => openInBuilder(turn.graph!, turn.text.slice(0, 30))}>
                          在画布中打开
                          <ArrowRight className="ml-1 h-3.5 w-3.5" />
                        </Button>
                        {selectedDef && (
                          <Button size="sm" variant="outline" onClick={() => saveDirectly(turn.graph!)}>
                            保存到流程库
                          </Button>
                        )}
                      </div>
                    </div>
                  ) : null}
                </div>
              </div>
            ))}
            {busy ? (
              <div className="flex justify-start">
                <div className="rounded-lg bg-muted px-3 py-2 text-sm text-muted-foreground">处理中…</div>
              </div>
            ) : null}
          </div>
        )}
      </div>

      <div className="border-t p-3">
        <div className="mx-auto flex max-w-2xl items-center gap-2">
          <input
            className="min-w-0 flex-1 rounded-md border bg-background px-3 py-2 text-sm"
            value={input}
            onChange={(e) => setInput(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter" && !e.shiftKey) {
                e.preventDefault();
                send(input);
              }
            }}
            placeholder={selectedDef ? `对「${selectedDef.name}」提出修改或提问…` : "描述需求,或选择已有流程后提出修改…"}
            disabled={busy}
          />
          <Button onClick={() => send(input)} disabled={busy || !input.trim()}>
            <Send className="h-4 w-4" />
          </Button>
        </div>
      </div>
    </div>
  );
}
