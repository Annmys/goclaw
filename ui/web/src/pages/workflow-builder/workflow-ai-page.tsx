import { useCallback, useRef, useState, useEffect } from "react";
import { useNavigate } from "react-router";
import { Sparkles, Send, ArrowRight, RotateCcw } from "lucide-react";
import { Button } from "@/components/ui/button";
import { PageHeader } from "@/components/shared/page-header";
import { ROUTES } from "@/lib/routes";
import { useGraphDefinitions } from "./hooks/use-graph-definitions";
import type { WorkflowGraph } from "@/types/workflow-graph";

interface ChatTurn {
  role: "user" | "assistant";
  text: string;
  graph?: WorkflowGraph; // attached when the assistant produced a workflow
}

// example prompts shown on the empty state (like sim's template-prompts).
const EXAMPLES = [
  "接收用户消息,用智能体判断是否投诉,是投诉转人工,否则自动回复",
  "每天定时抓取一批订单号,逐个查询流转单信息并汇总",
  "用户提问 → 知识库检索 → 智能体根据检索结果作答",
];

// WorkflowAIPage is goclaw's standalone "build a workflow by chatting" page,
// equivalent in shape to sim's mothership-chat. Each user message is sent to the
// backend AI generator; the produced graph is shown inline and can be opened in
// the visual builder or refined with a follow-up message.
export function WorkflowAIPage() {
  const navigate = useNavigate();
  const { generateGraph, saveDefinition } = useGraphDefinitions();
  const [turns, setTurns] = useState<ChatTurn[]>([]);
  const [input, setInput] = useState("");
  const [busy, setBusy] = useState(false);
  const scrollRef = useRef<HTMLDivElement>(null);

  // track the latest produced graph so follow-up messages refine it
  const latestGraph = [...turns].reverse().find((t) => t.graph)?.graph;

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
        const res = await generateGraph(prompt, latestGraph);
        setTurns((t) => [
          ...t,
          {
            role: "assistant",
            text: res.explanation || `已生成包含 ${res.graph.blocks.length} 个节点的工作流。`,
            graph: res.graph,
          },
        ]);
      } catch (err) {
        setTurns((t) => [...t, { role: "assistant", text: `生成失败:${(err as Error).message}` }]);
      } finally {
        setBusy(false);
      }
    },
    [busy, generateGraph, latestGraph],
  );

  // openInBuilder saves the graph then navigates to the visual editor.
  const openInBuilder = useCallback(
    async (graph: WorkflowGraph, name: string) => {
      const saved = await saveDefinition({ name: name || "AI 生成的工作流", graph });
      navigate(ROUTES.WORKFLOW_BUILDER_EDIT.replace(":id", saved.id));
    },
    [saveDefinition, navigate],
  );

  return (
    <div className="flex h-full flex-col">
      <PageHeader
        title="AI 建流程"
        actions={
          turns.length > 0 ? (
            <Button size="sm" variant="outline" onClick={() => setTurns([])}>
              <RotateCcw className="mr-1 h-4 w-4" />
              新会话
            </Button>
          ) : null
        }
      />

      <div ref={scrollRef} className="min-h-0 flex-1 overflow-y-auto px-4 py-4">
        {turns.length === 0 ? (
          <div className="mx-auto max-w-2xl pt-10 text-center">
            <Sparkles className="mx-auto mb-3 h-10 w-10 text-violet-500" />
            <h2 className="mb-1 text-lg font-semibold">用一句话描述你想要的工作流</h2>
            <p className="mb-6 text-sm text-muted-foreground">
              AI 会自动生成节点和连线,你可以继续追加要求来修改,满意后一键在画布中打开。
            </p>
            <div className="space-y-2 text-left">
              {EXAMPLES.map((ex) => (
                <button
                  key={ex}
                  onClick={() => send(ex)}
                  className="flex w-full items-center gap-2 rounded-md border px-3 py-2 text-sm hover:bg-accent"
                >
                  <Sparkles className="h-3.5 w-3.5 shrink-0 text-violet-500" />
                  {ex}
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
                        工作流预览:{turn.graph.blocks.length} 节点 · {turn.graph.connections.length} 连线
                      </div>
                      <div className="mb-2 flex flex-wrap gap-1">
                        {turn.graph.blocks.slice(0, 12).map((b) => (
                          <span key={b.id} className="rounded bg-muted px-1.5 py-0.5 text-[10px]">
                            {b.metadata?.name || b.metadata?.id}
                          </span>
                        ))}
                      </div>
                      <Button size="sm" onClick={() => openInBuilder(turn.graph!, turn.text.slice(0, 30))}>
                        在画布中打开
                        <ArrowRight className="ml-1 h-3.5 w-3.5" />
                      </Button>
                    </div>
                  ) : null}
                </div>
              </div>
            ))}
            {busy ? (
              <div className="flex justify-start">
                <div className="rounded-lg bg-muted px-3 py-2 text-sm text-muted-foreground">生成中…</div>
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
            placeholder={latestGraph ? "继续描述要修改的地方…" : "描述你想要的工作流…"}
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
