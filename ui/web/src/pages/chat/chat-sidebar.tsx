import { memo } from "react";
import { useTranslation } from "react-i18next";
import { PanelLeftClose, PanelLeftOpen, Plus } from "lucide-react";
import { Button } from "@/components/ui/button";
import { AgentSelector } from "@/components/chat/agent-selector";
import { SessionSwitcher } from "@/components/chat/session-switcher";
import { cn } from "@/lib/utils";
import type { SessionInfo } from "@/types/session";

interface ChatSidebarProps {
  agentId: string;
  onAgentChange: (agentId: string) => void;
  sessions: SessionInfo[];
  sessionsLoading: boolean;
  activeSessionKey: string;
  onSessionSelect: (key: string) => void;
  onDeleteSession?: (key: string) => void;
  onNewChat: () => void;
  sessionsCollapsed?: boolean;
  onSessionsCollapsedChange?: (collapsed: boolean) => void;
}

export const ChatSidebar = memo(function ChatSidebar({
  agentId,
  onAgentChange,
  sessions,
  sessionsLoading,
  activeSessionKey,
  onSessionSelect,
  onDeleteSession,
  onNewChat,
  sessionsCollapsed = false,
  onSessionsCollapsedChange,
}: ChatSidebarProps) {
  const { t } = useTranslation("chat");
  const ToggleIcon = sessionsCollapsed ? PanelLeftOpen : PanelLeftClose;

  return (
    <div className="flex h-full w-72 max-w-[85vw] flex-col border-r bg-background transition-[width] duration-200">
      {/* Agent selector */}
      <div className="border-b p-3">
        <div className="flex items-center gap-2">
          <div className="min-w-0 flex-1">
            <AgentSelector value={agentId} onChange={onAgentChange} />
          </div>
          <button
            type="button"
            onClick={() => onSessionsCollapsedChange?.(!sessionsCollapsed)}
            className="cursor-pointer rounded-md p-2 text-muted-foreground hover:bg-accent hover:text-accent-foreground"
            title={sessionsCollapsed ? t("expandSessions") : t("collapseSessions")}
            aria-label={sessionsCollapsed ? t("expandSessions") : t("collapseSessions")}
            aria-expanded={!sessionsCollapsed}
          >
            <ToggleIcon className="h-4 w-4" />
          </button>
        </div>
      </div>

      {/* New chat button */}
      <div className={cn("p-3", sessionsCollapsed && "hidden")}>
        <Button
          variant="outline"
          className="w-full justify-start gap-2"
          onClick={onNewChat}
        >
          <Plus className="h-4 w-4" />
          {t("newChat")}
        </Button>
      </div>

      {/* Session list */}
      <div className={cn("flex-1 overflow-y-auto", sessionsCollapsed && "hidden")}>
        <SessionSwitcher
          sessions={sessions}
          activeKey={activeSessionKey}
          onSelect={onSessionSelect}
          onDelete={onDeleteSession}
          loading={sessionsLoading}
        />
      </div>
    </div>
  );
});
