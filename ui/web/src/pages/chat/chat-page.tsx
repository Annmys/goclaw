import { useState, useCallback, useEffect, useRef, useMemo } from "react";
import { useTranslation } from "react-i18next";
import { useParams, useNavigate } from "react-router";
import { Eye, PanelLeftOpen } from "lucide-react";
import { useAuthStore } from "@/stores/use-auth-store";
import { useIsMobile } from "@/hooks/use-media-query";
import { cn } from "@/lib/utils";
import { ChatSidebar } from "./chat-sidebar";
import { ChatThread } from "./chat-thread";
import { ChatInput, type AttachedFile } from "@/components/chat/chat-input";
import { ChatTopBar } from "@/components/chat/chat-top-bar";
import { DropZone } from "@/components/chat/drop-zone";
import { AgentPickerPrompt } from "@/components/chat/agent-picker-prompt";
import { useChatSessions } from "./hooks/use-chat-sessions";
import { useChatMessages } from "./hooks/use-chat-messages";
import { useChatSend } from "./hooks/use-chat-send";
import { isOwnSession, parseSessionKey } from "@/lib/session-key";
import { useVirtualKeyboard } from "@/hooks/use-virtual-keyboard";
import { TaskPanel } from "@/components/chat/task-panel";
import { LOCAL_STORAGE_KEYS } from "@/lib/constants";

type StoredChatSelection = {
  agentId: string;
  sessionKey: string;
  updatedAt: number;
};

type StoredChatSelectionMap = Record<string, StoredChatSelection>;

function buildChatSelectionScope(tenantId: string, tenantSlug: string, userId: string) {
  const tenantScope = tenantId || tenantSlug || "default";
  return `${tenantScope}:${userId || "anonymous"}`;
}

function readStoredSelections(): StoredChatSelectionMap {
  if (typeof window === "undefined") return {};
  try {
    const raw = window.localStorage.getItem(LOCAL_STORAGE_KEYS.LAST_CHAT_SELECTION);
    if (!raw) return {};
    const parsed = JSON.parse(raw);
    return parsed && typeof parsed === "object" ? parsed as StoredChatSelectionMap : {};
  } catch {
    return {};
  }
}

function writeStoredSelections(map: StoredChatSelectionMap) {
  if (typeof window === "undefined") return;
  window.localStorage.setItem(LOCAL_STORAGE_KEYS.LAST_CHAT_SELECTION, JSON.stringify(map));
}

export function ChatPage() {
  const { t } = useTranslation("chat");
  const { sessionKey: urlSessionKey } = useParams<{ sessionKey: string }>();
  const navigate = useNavigate();
  const connected = useAuthStore((s) => s.connected);
  const userId = useAuthStore((s) => s.userId);
  const tenantId = useAuthStore((s) => s.tenantId);
  const tenantSlug = useAuthStore((s) => s.tenantSlug);

  const [scrollTrigger, setScrollTrigger] = useState(0);
  const [files, setFiles] = useState<AttachedFile[]>([]);
  const suppressRestoreRef = useRef(false);
  const restoreAttemptedRef = useRef(false);

  // sessionKey derived from URL — single source of truth, no separate state
  const sessionKey = urlSessionKey ?? "";

  // Fallback agent ID used only when URL has no session key
  const [agentIdFallback, setAgentIdFallback] = useState("");

  // Agent is confirmed when URL has a session (agentId parsed) or user explicitly picked one
  const agentConfirmed = !!urlSessionKey || !!agentIdFallback;

  // Derive agentId from URL (source of truth), fallback to state when no session
  const agentId = useMemo(() => {
    if (urlSessionKey) {
      const { agentId: parsed } = parseSessionKey(urlSessionKey);
      if (parsed) return parsed;
    }
    return agentIdFallback;
  }, [urlSessionKey, agentIdFallback]);

  const selectionScope = useMemo(
    () => buildChatSelectionScope(tenantId, tenantSlug, userId),
    [tenantId, tenantSlug, userId],
  );

  useEffect(() => {
    restoreAttemptedRef.current = false;
  }, [selectionScope]);

  useEffect(() => {
    if (!connected) return;
    if (urlSessionKey) return;
    if (restoreAttemptedRef.current) return;
    restoreAttemptedRef.current = true;

    const stored = readStoredSelections()[selectionScope];
    if (!stored) return;

    if (suppressRestoreRef.current) {
      suppressRestoreRef.current = false;
      if (stored.agentId && stored.agentId !== agentIdFallback) {
        setAgentIdFallback(stored.agentId);
      }
      return;
    }

    if (stored.agentId && stored.agentId !== agentIdFallback) {
      setAgentIdFallback(stored.agentId);
    }
    if (stored.sessionKey) {
      navigate(`/chat/${encodeURIComponent(stored.sessionKey)}`, { replace: true });
    }
  }, [connected, urlSessionKey, selectionScope, navigate, agentIdFallback]);

  useEffect(() => {
    if (!connected) return;
    if (!agentId) return;

    const all = readStoredSelections();
    const previous = all[selectionScope];
    const nextSessionKey = sessionKey || previous?.sessionKey || "";
    all[selectionScope] = {
      agentId,
      sessionKey: nextSessionKey,
      updatedAt: Date.now(),
    };
    writeStoredSelections(all);
  }, [connected, selectionScope, agentId, sessionKey]);

  const {
    sessions,
    loading: sessionsLoading,
    refresh: refreshSessions,
    buildNewSessionKey,
    deleteSession,
  } = useChatSessions(agentId);

  const {
    messages,
    streamText,
    thinkingText,
    toolStream,
    isRunning,
    isBusy,
    loading: messagesLoading,
    activity,
    blockReplies,
    teamTasks,
    expectRun,
    addLocalMessage,
  } = useChatMessages(sessionKey, agentId);

  // Refresh sessions when all work completes (main agent + team tasks)
  const prevIsBusyRef = useRef(false);
  useEffect(() => {
    if (prevIsBusyRef.current && !isBusy) {
      refreshSessions();
    }
    prevIsBusyRef.current = isBusy;
  }, [isBusy, refreshSessions]);

  const isOwn = !sessionKey || isOwnSession(sessionKey, userId);

  const handleMessageAdded = useCallback(
    (msg: { role: "user" | "assistant" | "tool"; content: string; timestamp?: number }, key?: string) => {
      addLocalMessage(msg, key);
    },
    [addLocalMessage],
  );

  const { send, abort, error: sendError } = useChatSend({
    agentId,
    onMessageAdded: handleMessageAdded,
    onExpectRun: expectRun,
  });

  const handleNewChat = useCallback(() => {
    navigate(`/chat/${encodeURIComponent(buildNewSessionKey())}`);
  }, [buildNewSessionKey, navigate]);

  const handleSessionSelect = useCallback(
    (key: string) => {
      const { agentId: parsed } = parseSessionKey(key);
      if (parsed) setAgentIdFallback(parsed);
      navigate(`/chat/${encodeURIComponent(key)}`);
    },
    [navigate],
  );

  const handleDeleteSession = useCallback(async (key: string) => {
    await deleteSession(key);
    if (key === sessionKey) {
      const next = sessions.find((s) => s.key !== key);
      if (next) {
        handleSessionSelect(next.key);
      } else {
        handleNewChat();
      }
    }
  }, [deleteSession, sessionKey, sessions, handleSessionSelect, handleNewChat]);

  const handleAgentChange = useCallback(
    (newAgentId: string) => {
      suppressRestoreRef.current = true;
      setAgentIdFallback(newAgentId);

      const all = readStoredSelections();
      all[selectionScope] = {
        agentId: newAgentId,
        sessionKey: "",
        updatedAt: Date.now(),
      };
      writeStoredSelections(all);

      if (sessionKey) {
        navigate("/chat");
      }
    },
    [navigate, selectionScope, sessionKey],
  );

  const handleSend = useCallback(
    (message: string, sendFiles?: AttachedFile[]) => {
      let key = sessionKey;
      if (!key) {
        key = buildNewSessionKey();
        navigate(`/chat/${encodeURIComponent(key)}`, { replace: true });
      }
      send(message, key, sendFiles);
      setScrollTrigger((n) => n + 1);
    },
    [sessionKey, send, buildNewSessionKey, navigate],
  );

  const handleDropFiles = useCallback((dropped: File[]) => {
    setFiles((prev) => [...prev, ...dropped.map((f) => ({ file: f }))]);
  }, []);

  const handleAbort = useCallback(() => {
    abort(sessionKey);
  }, [abort, sessionKey]);

  const isMobile = useIsMobile();
  useVirtualKeyboard();
  const [chatSidebarOpen, setChatSidebarOpen] = useState(false);
  const [taskPanelOpen, setTaskPanelOpen] = useState(false);

  // Auto-open task panel when first task appears, auto-close when all done.
  const prevTaskCountRef = useRef(0);
  useEffect(() => {
    const prev = prevTaskCountRef.current;
    const curr = teamTasks.length;
    if (prev === 0 && curr > 0) setTaskPanelOpen(true);
    if (curr === 0 && prev > 0) setTaskPanelOpen(false);
    prevTaskCountRef.current = curr;
  }, [teamTasks.length]);

  const handleSessionSelectMobile = useCallback(
    (key: string) => {
      handleSessionSelect(key);
      setChatSidebarOpen(false);
    },
    [handleSessionSelect],
  );

  const handleNewChatMobile = useCallback(() => {
    handleNewChat();
    setChatSidebarOpen(false);
  }, [handleNewChat]);

  return (
    <div className="relative flex h-full overflow-hidden">
      {/* Chat Sidebar */}
      {isMobile ? (
        <>
          {chatSidebarOpen && (
            <div
              className="fixed inset-0 z-40 bg-black/50"
              onClick={() => setChatSidebarOpen(false)}
            />
          )}
          <div
            className={cn(
              "fixed inset-y-0 left-0 z-50 transition-transform duration-200 ease-in-out",
              chatSidebarOpen ? "translate-x-0" : "-translate-x-full",
            )}
          >
            <ChatSidebar
              agentId={agentId}
              onAgentChange={handleAgentChange}
              sessions={sessions}
              sessionsLoading={sessionsLoading}
              activeSessionKey={sessionKey}
              onSessionSelect={handleSessionSelectMobile}
              onDeleteSession={handleDeleteSession}
              onNewChat={handleNewChatMobile}
            />
          </div>
        </>
      ) : (
        <ChatSidebar
          agentId={agentId}
          onAgentChange={handleAgentChange}
          sessions={sessions}
          sessionsLoading={sessionsLoading}
          activeSessionKey={sessionKey}
          onSessionSelect={handleSessionSelect}
          onDeleteSession={handleDeleteSession}
          onNewChat={handleNewChat}
        />
      )}

      {/* Main chat area */}
      <div className="flex min-w-0 flex-1 min-h-0 flex-col">
        {isMobile && (
          <div className="flex shrink-0 items-center border-b px-3 py-2 landscape-compact">
            <button
              onClick={() => setChatSidebarOpen(true)}
              className="rounded-md p-1.5 text-muted-foreground hover:bg-accent hover:text-accent-foreground"
              title={t("openSessions")}
            >
              <PanelLeftOpen className="h-4 w-4" />
            </button>
          </div>
        )}

        <div className="shrink-0">
          <ChatTopBar
            agentId={agentId}
            isRunning={isRunning}
            isBusy={isBusy}
            activity={activity}
            teamTasks={teamTasks}
            onToggleTaskPanel={() => setTaskPanelOpen((v) => !v)}
            taskPanelOpen={taskPanelOpen}
            session={sessions.find((s) => s.key === sessionKey) ?? null}
          />
        </div>

        {sendError && (
          <div className="shrink-0 border-b bg-destructive/10 px-4 py-2 text-sm text-destructive">
            {sendError}
          </div>
        )}

        <DropZone onDrop={handleDropFiles}>
          <ChatThread
            messages={messages}
            streamText={streamText}
            thinkingText={thinkingText}
            toolStream={toolStream}
            blockReplies={blockReplies}
            activity={activity}
            teamTasks={teamTasks}
            isRunning={isRunning}
            isBusy={isBusy}
            loading={messagesLoading}
            scrollTrigger={scrollTrigger}
            onToggleTaskPanel={() => setTaskPanelOpen((v) => !v)}
          />

          {!isOwn ? (
            <div className="mx-3 mb-3 flex items-center gap-2 rounded-xl border bg-muted/50 px-4 py-3 text-sm text-muted-foreground shadow-sm">
              <Eye className="h-4 w-4" />
              {t("readOnly")}
            </div>
          ) : !agentConfirmed ? (
            <AgentPickerPrompt onSelect={handleAgentChange} />
          ) : (
            <ChatInput
              onSend={handleSend}
              onAbort={handleAbort}
              isBusy={isBusy}
              disabled={!connected}
              files={files}
              onFilesChange={setFiles}
            />
          )}
        </DropZone>
      </div>

      {/* Mobile overlay backdrop — must render before TaskPanel so panel sits above */}
      {isMobile && taskPanelOpen && (
        <div className="fixed inset-0 z-40 bg-black/50" onClick={() => setTaskPanelOpen(false)} />
      )}

      {/* Task panel — toggleable sidebar on the right */}
      <TaskPanel tasks={teamTasks} open={taskPanelOpen} onClose={() => setTaskPanelOpen(false)} />
    </div>
  );
}
