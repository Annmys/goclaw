import { useCallback, useEffect, useRef, useState } from "react";
import { useParams, useNavigate } from "react-router";
import { Send, ArrowLeft, Paperclip, X, Download } from "lucide-react";
import { Button } from "@/components/ui/button";
import { PageHeader } from "@/components/shared/page-header";
import { ROUTES } from "@/lib/routes";
import { useGraphDefinitions } from "./hooks/use-graph-definitions";
import { useHttp } from "@/hooks/use-ws";
import { useAuthStore } from "@/stores/use-auth-store";
import type { GraphDefinition } from "@/types/workflow-graph";
import type { WorkflowRun, WorkflowRunEvent } from "@/types/workflow";

// Safe UUID generator (crypto.randomUUID unavailable on HTTP)
let _idSeq = 0;
function uid(): string {
  _idSeq++;
  return `${Date.now()}-${_idSeq}-${Math.random().toString(36).slice(2, 8)}`;
}

interface ChatTurn {
  role: "user" | "assistant";
  text: string;
  status?: string;
  events?: WorkflowRunEvent[]; // node-level execution events for progress display
}

// renderOutput turns a workflow run's output into a human-friendly display.
// For EPL results: extracts key info from JSON, formats nicely.
// For agent markdown: preserves as-is (already formatted).
function renderOutput(output: Record<string, unknown> | undefined): string {
  if (!output || Object.keys(output).length === 0) return "(无输出)";
  for (const key of ["content", "message", "response", "text", "result"]) {
    const v = output[key];
    if (typeof v === "string" && v) return formatResultText(v);
  }
  return JSON.stringify(output, null, 2);
}

// formatResultText: parse EPL script JSON from the combined result text,
// replace raw JSON with a clean summary, keep agent markdown as-is.
function formatResultText(text: string): string {
  // Split into sections: "脚本结果:{json...}" and "数据补全:..."
  const scriptMatch = text.match(/脚本结果:\s*(\{[\s\S]*?\})\s*(?:\n|数据补全|文件:)/);
  if (!scriptMatch) return text; // not an EPL result, return as-is

  try {
    const jsonStr = scriptMatch[1] || "";
    const data = JSON.parse(jsonStr);
    const filled = data.filled || {};
    const lines: string[] = [];

    lines.push("✅ EPL 预估箱单已生成\n");
    lines.push(`📋 订单: ${(data.orders || []).join(", ") || "未知"}`);
    if (filled.c_n) lines.push(`📦 C/N: ${filled.c_n} (${data.boxes || "?"}箱)`);
    if (filled.dimension) lines.push(`📐 尺寸: ${filled.dimension.replace(/\\n/g, " | ")}`);
    if (data.gw_total) lines.push(`⚖️ G.W.: ${data.gw_total} kg`);
    if (filled.gw?.length) {
      for (const g of filled.gw) {
        lines.push(`   └ ${g.series} ${g.qty}M × ${(g.net_kg / g.qty).toFixed(3)}kg/m = ${g.net_kg}kg`);
      }
    }
    if (data.pkg_weight) lines.push(`   └ 外包装: ${data.pkg_weight}kg`);

    // Replace the raw JSON section with formatted summary
    let result = text.replace(/脚本结果:\s*\{[\s\S]*?\}\s*(?=\n|数据补全|文件:)/, lines.join("\n") + "\n");

    // Clean up "EPL预估箱单已生成并补全数据。\n" prefix (redundant with our ✅ line)
    result = result.replace(/^EPL预估箱单已生成并补全数据。\s*\n?/, "");

    // Clean up "文件:/app/workspace/epl/output.xlsx" (download button handles this)
    result = result.replace(/\n?文件:\/app\/workspace\/epl\/output\.xlsx\s*$/, "");

    return result.trim();
  } catch {
    return text; // JSON parse failed, return original
  }
}

// extractFilePath detects a downloadable file path from the output text.
// Looks for /app/workspace/... paths, or falls back to the known EPL output path.
function extractFilePath(text: string): string | null {
  const m = text.match(/\/app\/workspace\/[^\s"',}\\]+\.xlsx/);
  if (m) return m[0];
  // Fallback: if text mentions "output" and "xlsx" or "ok.*true", assume EPL output exists
  if (text.includes("/app/workspace/") || (text.includes('"ok"') && text.includes("xlsx"))) {
    return "/app/workspace/epl/output.xlsx";
  }
  return null;
}

// fileDownloadUrl builds the download URL for a workspace file.
function fileDownloadUrl(filePath: string): string {
  // /v1/files/{path...} serves files by absolute path (minus leading /)
  const relative = filePath.replace(/^\//, "");
  return `/v1/files/${relative}`;
}

// NodeProgressTable renders execution events as a structured table showing
// each node's name, type, status, duration, and output summary.
function NodeProgressTable({ events }: { events: WorkflowRunEvent[] }) {
  // Build per-node state from event stream
  const nodes: Record<string, {
    id: string;
    type: string;
    status: "pending" | "running" | "completed" | "error";
    startTime?: string;
    endTime?: string;
    output?: string;
  }> = {};

  for (const ev of events) {
    const nid = ev.node_id || "";
    if (!nid) continue;
    // Extract node type from message pattern "node xxx (type) started/completed"
    const typeMatch = ev.message.match(/\((\w[\w-]*)\)/);
    const nodeType = typeMatch ? typeMatch[1] : "";

    if (ev.type === "node_start") {
      nodes[nid] = { id: nid, type: nodeType || (nodes[nid]?.type ?? ""), status: "running", startTime: ev.created_at };
    } else if (ev.type === "node_complete") {
      const existing = nodes[nid] || { id: nid, type: nodeType || "" };
      const outputStr = ev.payload ? JSON.stringify(ev.payload).slice(0, 80) : "";
      nodes[nid] = { ...existing, type: existing.type || nodeType || "", status: "completed", endTime: ev.created_at, output: outputStr };
    } else if (ev.type === "node_error") {
      const existing = nodes[nid] || { id: nid, type: nodeType || "" };
      nodes[nid] = { ...existing, type: existing.type || nodeType || "", status: "error", endTime: ev.created_at, output: ev.message };
    }
  }

  const list = Object.values(nodes);
  if (list.length === 0) return null;

  const statusLabel = (s: string) => {
    switch (s) {
      case "completed": return <span className="text-green-600 font-medium">✅ 完成</span>;
      case "error": return <span className="text-red-600 font-medium">❌ 失败</span>;
      case "running": return <span className="text-amber-600 font-medium">⏳ 执行中</span>;
      default: return <span className="text-muted-foreground">⬜ 待执行</span>;
    }
  };

  const duration = (start?: string, end?: string) => {
    if (!start || !end) return "-";
    const ms = new Date(end).getTime() - new Date(start).getTime();
    return ms < 1000 ? `${ms}ms` : `${(ms / 1000).toFixed(1)}s`;
  };

  return (
    <div className="mt-3 overflow-x-auto rounded border">
      <table className="w-full text-xs">
        <thead>
          <tr className="border-b bg-muted/50 text-left">
            <th className="px-2 py-1.5 font-medium">节点</th>
            <th className="px-2 py-1.5 font-medium">类型</th>
            <th className="px-2 py-1.5 font-medium">状态</th>
            <th className="px-2 py-1.5 font-medium">耗时</th>
            <th className="px-2 py-1.5 font-medium">输出摘要</th>
          </tr>
        </thead>
        <tbody>
          {list.map((n) => (
            <tr
              key={n.id}
              className={
                n.status === "error" ? "bg-red-50 dark:bg-red-950/20" :
                n.status === "running" ? "bg-amber-50 dark:bg-amber-950/20" : ""
              }
            >
              <td className="px-2 py-1.5 font-medium">{n.id}</td>
              <td className="px-2 py-1.5 text-muted-foreground">{n.type}</td>
              <td className="px-2 py-1.5">{statusLabel(n.status)}</td>
              <td className="px-2 py-1.5 text-muted-foreground">{duration(n.startTime, n.endTime)}</td>
              <td className="max-w-[200px] truncate px-2 py-1.5 text-muted-foreground">{n.output || "-"}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

// WorkflowChatRunPage runs a saved workflow conversationally: each user message
// triggers a run (message passed as workflow input) and the run's output is
// shown as an assistant reply. This is goclaw's equivalent of sim's
// "deploy as chat" consumption surface.
export function WorkflowChatRunPage() {
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const { getDefinition, runDefinition, uploadFile } = useGraphDefinitions();
  const http = useHttp();
  const userId = useAuthStore((s) => s.userId);
  const [def, setDef] = useState<GraphDefinition | null>(null);
  const [input, setInput] = useState("");
  const [pendingFile, setPendingFile] = useState<File | null>(null);
  const scrollRef = useRef<HTMLDivElement>(null);
  const fileInputRef = useRef<HTMLInputElement>(null);

  // Detect if this is a fresh run from 流程库 (URL has ?new=1)
  const isNewRun = typeof window !== "undefined" && new URLSearchParams(window.location.search).get("new") === "1";

  // Persist busy state so switching away and back shows "still running" or result
  const busyKey = `wf-busy:${userId || "anon"}:${id || "none"}`;
  const [busy, setBusy] = useState(() => {
    try { return localStorage.getItem(busyKey) === "1"; }
    catch { return false; }
  });
  // Sync busy to localStorage
  useEffect(() => {
    try { localStorage.setItem(busyKey, busy ? "1" : "0"); } catch {}
  }, [busy, busyKey]);

  // Single persistent chat history per workflow + user
  const storageKey = `wf-chat:${userId || "anon"}:${id || "none"}`;
  const [turns, setTurns] = useState<ChatTurn[]>(() => {
    if (isNewRun) return []; // fresh start for new runs
    try { const r = localStorage.getItem(storageKey); return r ? JSON.parse(r) : []; }
    catch { return []; }
  });
  useEffect(() => {
    try { localStorage.setItem(storageKey, JSON.stringify(turns)); } catch {}
  }, [turns, storageKey]);

  // Register a NEW workflow session into sidebar history ONLY when coming from 流程库 (?new=1).
  // Clicking an existing history session should NOT create a new one.
  const sessionRegistered = useRef(false);
  useEffect(() => {
    if (!id || !def || sessionRegistered.current || !isNewRun) return;
    sessionRegistered.current = true;
    const historyKey = `wf-history:${userId || "anon"}`;
    try {
      const history: { id: string; wfId: string; title: string; pinned: boolean; createdAt: string }[] =
        JSON.parse(localStorage.getItem(historyKey) || "[]");
      history.unshift({ id: uid(), wfId: id, title: def.name || "未命名流程", pinned: false, createdAt: new Date().toISOString() });
      localStorage.setItem(historyKey, JSON.stringify(history.slice(0, 20)));
    } catch {}
    // Clear previous chat so it starts fresh
    try { localStorage.removeItem(storageKey); } catch {}
    setTurns([]);
    // Remove ?new=1 from URL to prevent re-triggering on refresh
    window.history.replaceState({}, "", window.location.pathname);
  }, [id, def, userId, storageKey, isNewRun]);

  useEffect(() => {
    if (id) getDefinition(id).then(setDef).catch(() => setDef(null));
  }, [id, getDefinition]);

  useEffect(() => {
    scrollRef.current?.scrollTo({ top: scrollRef.current.scrollHeight, behavior: "smooth" });
  }, [turns, busy]);

  // Recovery: if we come back and busy=true (left while running), check if the
  // last turn is a "progress" message — if so, the SSE stream was lost when we
  // navigated away. Show a notice and reset busy so the user can retry.
  useEffect(() => {
    if (!busy) return;
    const lastTurn = turns[turns.length - 1];
    if (lastTurn?.status === "progress") {
      // The stream was interrupted — update the progress message to indicate this
      setTurns((t) => {
        const filtered = t.filter((turn) => turn.status !== "progress");
        return [...filtered, { role: "assistant", text: "⚠️ 上次执行被中断(切走了页面)。请重新发送消息或上传文件重试。", status: "interrupted" }];
      });
      setBusy(false);
    } else if (!lastTurn || lastTurn.status === "completed" || lastTurn.status === "failed") {
      // Already finished — just clear busy
      setBusy(false);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const send = useCallback(
    async (text: string) => {
      const message = text.trim();
      const file = pendingFile;
      if ((!message && !file) || busy || !id) return;
      setInput("");
      setPendingFile(null);
      setTurns((t) => [...t, { role: "user", text: file ? `${message || "(已上传文件)"} 📎 ${file.name}` : message }]);
      setBusy(true);

      // Live events accumulator for the progress table
      const liveEvents: WorkflowRunEvent[] = [];
      const updateProgress = () => {
        setTurns((t) => {
          const last = t[t.length - 1];
          if (last?.role === "assistant" && last.status === "progress") {
            return [...t.slice(0, -1), { role: "assistant", text: "⚙️ 执行中…", status: "progress", events: [...liveEvents] }];
          }
          return [...t, { role: "assistant", text: "⚙️ 执行中…", status: "progress", events: [...liveEvents] }];
        });
      };

      try {
        const runInput: Record<string, unknown> = { message };
        if (file) {
          setTurns((t) => [...t, { role: "assistant", text: "⬆️ 上传文件中…", status: "progress" }]);
          const up = await uploadFile(file);
          runInput.file_path = up.path;
          runInput.file_name = up.filename;
          runInput.file_type = up.mime_type;
        }
        updateProgress();

        // Use SSE streaming endpoint for real-time node updates
        const token = useAuthStore.getState().token;
        const resp = await fetch(`/v1/workflow-definitions/${id}/run/stream`, {
          method: "POST",
          headers: { "Content-Type": "application/json", "Authorization": `Bearer ${token}` },
          body: JSON.stringify({ input: runInput }),
        });

        if (!resp.ok || !resp.body) {
          // Fallback to non-streaming
          const run: WorkflowRun = await runDefinition(id, runInput);
          setTurns((t) => {
            const filtered = t.filter((turn) => turn.status !== "progress");
            return [...filtered, { role: "assistant", text: renderOutput(run.output as Record<string, unknown>), status: run.status, events: run.events }];
          });
          return;
        }

        // Read SSE stream
        const reader = resp.body.getReader();
        const decoder = new TextDecoder();
        let buffer = "";
        let finalRun: WorkflowRun | null = null;

        while (true) {
          const { done, value } = await reader.read();
          if (done) break;
          buffer += decoder.decode(value, { stream: true });

          // Parse SSE events from buffer
          const lines = buffer.split("\n");
          buffer = lines.pop() || "";
          let eventType = "";
          for (const line of lines) {
            if (line.startsWith("event: ")) {
              eventType = line.slice(7).trim();
            } else if (line.startsWith("data: ")) {
              const data = line.slice(6);
              try {
                const parsed = JSON.parse(data);
                if (eventType === "node_start") {
                  liveEvents.push({ id: uid(), type: "node_start", node_id: parsed.node_id, message: `node ${parsed.node_id} (${parsed.node_type}) started`, created_at: new Date().toISOString() });
                  updateProgress();
                } else if (eventType === "node_complete") {
                  liveEvents.push({ id: uid(), type: "node_complete", node_id: parsed.node_id, message: `node ${parsed.node_id} (${parsed.node_type}) completed`, payload: { output: parsed.output }, created_at: new Date().toISOString() });
                  updateProgress();
                } else if (eventType === "node_error") {
                  liveEvents.push({ id: uid(), type: "node_error", node_id: parsed.node_id, message: `node ${parsed.node_id} (${parsed.node_type}) error: ${parsed.error}`, created_at: new Date().toISOString() });
                  updateProgress();
                } else if (eventType === "done") {
                  finalRun = parsed.run;
                } else if (eventType === "error") {
                  liveEvents.push({ id: uid(), type: "node_error", node_id: "system", message: parsed.error || "unknown error", created_at: new Date().toISOString() });
                  updateProgress();
                }
              } catch { /* skip unparseable */ }
              eventType = "";
            }
          }
        }

        // Final result
        setTurns((t) => {
          const filtered = t.filter((turn) => turn.status !== "progress");
          const output = finalRun?.output ? renderOutput(finalRun.output as Record<string, unknown>) : "执行完成";
          return [...filtered, { role: "assistant", text: output, status: finalRun?.status || "completed", events: [...liveEvents] }];
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
                  {turn.events && turn.events.length > 0 ? <NodeProgressTable events={turn.events} /> : null}
                  {turn.role === "assistant" && turn.text && extractFilePath(turn.text) ? (
                    <div className="mt-2">
                      <button
                        onClick={async () => {
                          const fp = extractFilePath(turn.text)!;
                          try {
                            const res = await http.post<{ url: string }>("/v1/files/sign", { path: fp });
                            window.open(res.url, "_blank");
                          } catch {
                            window.open(fileDownloadUrl(fp), "_blank");
                          }
                        }}
                        className="inline-flex items-center gap-1.5 rounded border bg-background px-2.5 py-1.5 text-xs font-medium hover:bg-accent"
                      >
                        <Download className="h-3.5 w-3.5" />
                        下载结果文件
                      </button>
                    </div>
                  ) : null}
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
