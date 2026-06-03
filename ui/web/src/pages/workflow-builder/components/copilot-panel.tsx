import { useState } from "react";
import { Sparkles, Send, X } from "lucide-react";
import { Button } from "@/components/ui/button";
import type { WorkflowGraph } from "@/types/workflow-graph";

interface CopilotPanelProps {
  // current canvas graph, sent for incremental edits
  current: WorkflowGraph;
  // generate calls the backend AI; resolves to the new graph + explanation
  generate: (prompt: string, current?: WorkflowGraph) => Promise<{ graph: WorkflowGraph; explanation: string }>;
  // onApply replaces the canvas with the generated graph
  onApply: (graph: WorkflowGraph) => void;
  onClose: () => void;
}

interface ChatTurn {
  role: "user" | "assistant";
  text: string;
}

// CopilotPanel is goclaw's self-hosted equivalent of sim's AI workflow copilot.
// The user types a natural-language request; the backend agent returns a graph
// which is applied to the canvas. Backed by goclaw's own agent runtime — no
// external service.
export function CopilotPanel({ current, generate, onApply, onClose }: CopilotPanelProps) {
  const [input, setInput] = useState("");
  const [turns, setTurns] = useState<ChatTurn[]>([]);
  const [busy, setBusy] = useState(false);

  const submit = async () => {
    const prompt = input.trim();
    if (!prompt || busy) return;
    setInput("");
    setTurns((t) => [...t, { role: "user", text: prompt }]);
    setBusy(true);
    try {
      // pass current graph for incremental edits once the canvas is non-empty
      const hasGraph = current.blocks.length > 0;
      const res = await generate(prompt, hasGraph ? current : undefined);
      onApply(res.graph);
      setTurns((t) => [
        ...t,
        { role: "assistant", text: res.explanation || `已生成 ${res.graph.blocks.length} 个节点。` },
      ]);
    } catch (err) {
      setTurns((t) => [...t, { role: "assistant", text: `生成失败:${(err as Error).message}` }]);
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="absolute bottom-2 left-2 flex max-h-[70%] w-80 flex-col rounded-md border bg-card shadow-lg">
      <div className="flex items-center justify-between border-b px-3 py-2">
        <div className="flex items-center gap-1.5 text-sm font-semibold">
          <Sparkles className="h-4 w-4 text-violet-500" />
          AI 助手
        </div>
        <button onClick={onClose} className="text-muted-foreground hover:text-foreground">
          <X className="h-4 w-4" />
        </button>
      </div>

      <div className="min-h-0 flex-1 space-y-2 overflow-y-auto p-3">
        {turns.length === 0 ? (
          <p className="text-xs text-muted-foreground">
            用一句话描述你想要的工作流,例如:“每天抓取未读邮件,用 AI 总结后存入知识库”。AI 会自动生成画布节点。
          </p>
        ) : (
          turns.map((turn, i) => (
            <div
              key={i}
              className={
                turn.role === "user"
                  ? "ml-6 rounded bg-primary/10 px-2 py-1.5 text-sm"
                  : "mr-6 rounded bg-muted px-2 py-1.5 text-sm"
              }
            >
              {turn.text}
            </div>
          ))
        )}
        {busy ? <div className="mr-6 rounded bg-muted px-2 py-1.5 text-sm text-muted-foreground">生成中…</div> : null}
      </div>

      <div className="flex items-center gap-1 border-t p-2">
        <input
          className="min-w-0 flex-1 rounded border bg-background px-2 py-1 text-sm"
          value={input}
          onChange={(e) => setInput(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "Enter" && !e.shiftKey) {
              e.preventDefault();
              submit();
            }
          }}
          placeholder="描述你想要的流程…"
          disabled={busy}
        />
        <Button size="sm" onClick={submit} disabled={busy || !input.trim()}>
          <Send className="h-4 w-4" />
        </Button>
      </div>
    </div>
  );
}
