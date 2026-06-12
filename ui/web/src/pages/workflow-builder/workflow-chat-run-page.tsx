import { useCallback, useEffect, useRef, useState } from "react";
import { useParams, useNavigate } from "react-router";
import { Send, ArrowLeft, Paperclip, X } from "lucide-react";
import { Button } from "@/components/ui/button";
import { PageHeader } from "@/components/shared/page-header";
import { ROUTES } from "@/lib/routes";
import { useGraphDefinitions } from "./hooks/use-graph-definitions";
import { useAuthStore } from "@/stores/use-auth-store";
import type { GraphDefinition } from "@/types/workflow-graph";
import type { WorkflowRun } from "@/types/workflow";

interface ChatTurn {
  role: "user" | "assistant";
  text: string;
  status?: string;
}

// renderOutput turns a workflow run's output map into a readable reply. It
// prefers common reply fields (content/message/response/text), else pretty JSON.
function renderOutput(output: Record<string, unknown> | undefined): string {
  if (!output || Object.keys(output).length === 0) return "(无输出)";
  for (const key of ["content", "message", "response", "text", "result"]) {
    const v = output[key];
    if (typeof v === "string" && v) return v;
  }
  return JSON.stringify(output, null, 2);
}

// WorkflowChatRunPage runs a saved workflow conversationally: each user message
// triggers a run (message passed as workflow input) and the run's output is
// shown as an assistant reply. This is goclaw's equivalent of sim's
// "deploy as chat" consumption surface.
export function WorkflowChatRunPage() {
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const { getDefinition, runDefinition, uploadFile } = useGraphDefinitions();
  const userId = useAuthStore((s) => s.userId);
  const [def, setDef] = useState<GraphDefinition | null>(null);
  const [input, setInput] = useState("");
  const [busy, setBusy] = useState(false);
  const [pendingFile, setPendingFile] = useState<File | null>(null);
  const scrollRef = useRef<HTMLDivElement>(null);
  const fileInputRef = useRef<HTMLInputElement>(null);

  // Persistent chat history per workflow + user (survives navigation)
  const storageKey = `wf-chat:${userId || "anon"}:${id || "none"}`;
  const [turns, setTurns] = useState<ChatTurn[]>(() => {
    try {
      const raw = localStorage.getItem(storageKey);
      return raw ? JSON.parse(raw) : [];
    } catch { return []; }
  });
  useEffect(() => {
    try { localStorage.setItem(storageKey, JSON.stringify(turns)); } catch {}
  }, [turns, storageKey]);

  useEffect(() => {
    if (id) getDefinition(id).then(setDef).catch(() => setDef(null));
  }, [id, getDefinition]);

  useEffect(() => {
    scrollRef.current?.scrollTo({ top: scrollRef.current.scrollHeight, behavior: "smooth" });
  }, [turns, busy]);

  const send = useCallback(
    async (text: string) => {
      const message = text.trim();
      const file = pendingFile;
      if ((!message && !file) || busy || !id) return;
      setInput("");
      setPendingFile(null);
      setTurns((t) => [...t, { role: "user", text: file ? `${message || "(已上传文件)"} 📎 ${file.name}` : message }]);
      setBusy(true);

      // Progress helper: show intermediate status as a temporary assistant message
      const progress = (msg: string) => setTurns((t) => {
        const last = t[t.length - 1];
        if (last?.role === "assistant" && last.status === "progress") {
          return [...t.slice(0, -1), { role: "assistant", text: msg, status: "progress" }];
        }
        return [...t, { role: "assistant", text: msg, status: "progress" }];
      });

      try {
        const runInput: Record<string, unknown> = { message };
        if (file) {
          progress("⬆️ 上传文件中…");
          const up = await uploadFile(file);
          runInput.file_path = up.path;
          runInput.file_name = up.filename;
          runInput.file_type = up.mime_type;
          progress("⚙️ 执行流程中…");
        } else {
          progress("⚙️ 执行流程中…");
        }
        const run: WorkflowRun = await runDefinition(id, runInput);
        // Replace progress message with final result
        setTurns((t) => {
          const filtered = t.filter((turn) => turn.status !== "progress");
          return [...filtered, { role: "assistant", text: renderOutput(run.output as Record<string, unknown>), status: run.status }];
        });
      } catch (err) {
        setTurns((t) => {
          const filtered = t.filter((turn) => turn.status !== "progress");
          return [...filtered, { role: "assistant", text: `运行失败:${(err as Error).message}`, status: "failed" }];
        });
      } finally {
        setBusy(false);
      }
    },
    [busy, id, runDefinition, uploadFile, pendingFile],
  );

  return (
    <div className="flex h-full flex-col">
      <PageHeader
        title={def ? `运行:${def.name}` : "运行工作流"}
        actions={
          <Button size="sm" variant="outline" onClick={() => navigate(ROUTES.WORKFLOW_DEFINITIONS)}>
            <ArrowLeft className="mr-1 h-4 w-4" />
            返回流程库
          </Button>
        }
      />

      <div ref={scrollRef} className="min-h-0 flex-1 overflow-y-auto px-4 py-4">
        <div className="mx-auto max-w-2xl space-y-3">
          {turns.length === 0 ? (
            <p className="pt-8 text-center text-sm text-muted-foreground">
              输入消息以运行该工作流。你发送的内容会作为流程输入(可在节点中用 &lt;trigger.message&gt; 引用)。
            </p>
          ) : (
            turns.map((turn, i) => (
              <div key={i} className={turn.role === "user" ? "flex justify-end" : "flex justify-start"}>
                <div
                  className={
                    turn.role === "user"
                      ? "max-w-[80%] rounded-lg bg-primary px-3 py-2 text-sm text-primary-foreground"
                      : "max-w-[85%] rounded-lg bg-muted px-3 py-2 text-sm"
                  }
                >
                  {turn.status && turn.status !== "completed" ? (
                    <div className="mb-1 text-[10px] uppercase text-muted-foreground">{turn.status}</div>
                  ) : null}
                  <div className="whitespace-pre-wrap">{turn.text}</div>
                </div>
              </div>
            ))
          )}
          {busy ? (
            <div className="flex justify-start">
              <div className="rounded-lg bg-muted px-3 py-2 text-sm text-muted-foreground">运行中…</div>
            </div>
          ) : null}
        </div>
      </div>

      <div className="border-t p-3">
        <div className="mx-auto max-w-2xl">
          {pendingFile ? (
            <div className="mb-2 flex items-center gap-2 rounded border bg-muted px-2 py-1 text-xs">
              <Paperclip className="h-3.5 w-3.5" />
              <span className="truncate">{pendingFile.name}</span>
              <button className="ml-auto text-muted-foreground hover:text-foreground" onClick={() => setPendingFile(null)}>
                <X className="h-3.5 w-3.5" />
              </button>
            </div>
          ) : null}
          <div className="flex items-center gap-2">
            <input
              ref={fileInputRef}
              type="file"
              className="hidden"
              accept=".xlsx,.xls,.csv"
              onChange={(e) => {
                const f = e.target.files?.[0];
                if (f) setPendingFile(f);
                e.target.value = "";
              }}
            />
            <Button size="icon" variant="outline" onClick={() => fileInputRef.current?.click()} disabled={busy} title="上传文件">
              <Paperclip className="h-4 w-4" />
            </Button>
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
              placeholder={pendingFile ? "可补充说明,或直接发送…" : "输入消息运行工作流…"}
              disabled={busy}
            />
            <Button onClick={() => send(input)} disabled={busy || (!input.trim() && !pendingFile)}>
              <Send className="h-4 w-4" />
            </Button>
          </div>
        </div>
      </div>
    </div>
  );
}
