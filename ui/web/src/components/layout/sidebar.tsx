import { useMemo, useState } from "react";
import {
  LayoutDashboard,
  MessageSquare,
  Bot,
  History,
  Zap,
  Clock,
  Activity,
  Radio,
  Radar,
  Terminal,
  Settings,
  ShieldCheck,
  Users,
  Link,
  Package,
  Blocks,
  Plug,
  Volume2,
  Cpu,
  ClipboardList,
  HardDrive,
  Inbox,
  Brain,
  Network,
  Contact,
  KeyRound,
  Building2,
  ArrowLeftRight,
  FileArchive,
  DatabaseBackup,
  Webhook,
  Sparkles,
  FolderSync,
  Workflow,
} from "lucide-react";
import { useTranslation } from "react-i18next";
import { SidebarGroup } from "./sidebar-group";
import { SidebarItem } from "./sidebar-item";
import { ConnectionStatus } from "./connection-status";
import { ROUTES } from "@/lib/constants";
import { cn } from "@/lib/utils";
import { usePendingPairingsCount } from "@/hooks/use-pending-pairings-count";
import { useAuthStore } from "@/stores/use-auth-store";
import { useTenants } from "@/hooks/use-tenants";

interface SidebarProps {
  collapsed: boolean;
  onNavItemClick?: () => void;
}

export function Sidebar({ collapsed, onNavItemClick }: SidebarProps) {
  const { t } = useTranslation("sidebar");
  const { pendingCount } = usePendingPairingsCount();
  const role = useAuthStore((s) => s.role);
  const { isOwner } = useTenants();
  const isAdmin = role === "admin" || role === "owner";

  return (
    <aside
      className={cn(
        "flex h-full flex-col border-r bg-sidebar text-sidebar-foreground transition-all duration-200",
        collapsed ? "w-16" : "w-64",
      )}
      onClick={(e) => {
        // Close mobile drawer when clicking a nav link
        if (onNavItemClick && (e.target as HTMLElement).closest("a")) {
          onNavItemClick();
        }
      }}
    >
      {/* Logo / title */}
      <div className="flex h-14 items-center border-b px-4">
        {!collapsed && (
          <div className="flex items-center gap-2.5">
            <img src="/goclaw-icon.svg" alt="GoClaw" className="h-8 w-8" />
            <span className="text-lg font-bold tracking-tight text-sidebar-primary">
              GoClaw <span className="font-medium text-sidebar-foreground/70">-by:Annmy</span>
            </span>
          </div>
        )}
        {collapsed && (
          <img src="/goclaw-icon.svg" alt="GoClaw" className="mx-auto h-7 w-7" />
        )}
      </div>

      {/* Nav items */}
      <nav className="flex-1 space-y-4 overflow-y-auto px-2 py-4">
        <SidebarGroup label={t("groups.core")} collapsed={collapsed}>
          <SidebarItem to={ROUTES.CHAT} icon={MessageSquare} label={t("nav.chat")} collapsed={collapsed} />
          <SidebarItem to={ROUTES.AGENTS} icon={Bot} label={t("nav.agents")} collapsed={collapsed} />
          <SidebarItem to={ROUTES.TEAMS} icon={Users} label={t("nav.agentTeams")} collapsed={collapsed} />
          <SidebarItem to={ROUTES.OVERVIEW} icon={LayoutDashboard} label={t("nav.overview")} collapsed={collapsed} />
        </SidebarGroup>

        <SidebarGroup label={t("groups.workflow")} collapsed={collapsed}>
          <SidebarItem to={ROUTES.WORKFLOW_AI} icon={Sparkles} label={t("nav.workflowAI")} collapsed={collapsed} />
          <SidebarItem to={ROUTES.WORKFLOW_DEFINITIONS} icon={Workflow} label={t("nav.workflowDefinitions")} collapsed={collapsed} />
          <SidebarItem to={ROUTES.WORKFLOW_BUILDER} icon={Blocks} label={t("nav.workflowBuilder")} collapsed={collapsed} />
          {!collapsed && (
            <WorkflowHistorySidebar />
          )}
        </SidebarGroup>

        <SidebarGroup label={t("groups.conversations")} collapsed={collapsed}>
          <SidebarItem to={ROUTES.SESSIONS} icon={History} label={t("nav.sessions")} collapsed={collapsed} />
          <SidebarItem to={ROUTES.PENDING_MESSAGES} icon={Inbox} label={t("nav.pendingMessages")} collapsed={collapsed} />
          <SidebarItem to={ROUTES.CONTACTS} icon={Contact} label={t("nav.contacts")} collapsed={collapsed} />
        </SidebarGroup>

        <SidebarGroup label={t("groups.connectivity")} collapsed={collapsed}>
          <SidebarItem to={ROUTES.CHANNELS} icon={Radio} label={t("nav.channels")} collapsed={collapsed} />
          <SidebarItem to={ROUTES.NODES} icon={Link} label={t("nav.nodes")} collapsed={collapsed} badge={pendingCount} />
        </SidebarGroup>

        <SidebarGroup label={t("groups.capabilities")} collapsed={collapsed}>
          <SidebarItem to={ROUTES.SKILLS} icon={Zap} label={t("nav.skills")} collapsed={collapsed} />
          <SidebarItem to={ROUTES.BUILTIN_TOOLS} icon={Package} label={t("nav.builtinTools")} collapsed={collapsed} />
          <SidebarItem to={ROUTES.MCP} icon={Plug} label={t("nav.mcpServers")} collapsed={collapsed} />
          {isOwner && (
            <SidebarItem to={ROUTES.TTS} icon={Volume2} label={t("nav.tts")} collapsed={collapsed} />
          )}
          <SidebarItem to={ROUTES.CRON} icon={Clock} label={t("nav.cron")} collapsed={collapsed} />
          <SidebarItem to={ROUTES.HOOKS} icon={Webhook} label={t("nav.hooks")} collapsed={collapsed} />
          {isAdmin && (
            <SidebarItem to={ROUTES.EVOLUTION_CENTER} icon={Sparkles} label={t("nav.evolutionCenter")} collapsed={collapsed} />
          )}
        </SidebarGroup>

        <SidebarGroup label={t("groups.data")} collapsed={collapsed}>
          <SidebarItem to={ROUTES.MEMORY} icon={Brain} label={t("nav.memory")} collapsed={collapsed} />
          <SidebarItem to={ROUTES.VAULT} icon={FileArchive} label={t("nav.vault")} collapsed={collapsed} />
          <SidebarItem to={ROUTES.KNOWLEDGE_GRAPH} icon={Network} label={t("nav.knowledgeGraph")} collapsed={collapsed} />
          {isAdmin && (
            <SidebarItem to={ROUTES.LOCAL_KNOWLEDGE} icon={FolderSync} label={t("nav.localKnowledge")} collapsed={collapsed} />
          )}
          <SidebarItem to={ROUTES.STORAGE} icon={HardDrive} label={t("nav.storage")} collapsed={collapsed} />
        </SidebarGroup>

        <SidebarGroup label={t("groups.monitoring")} collapsed={collapsed}>
          <SidebarItem to={ROUTES.TRACES} icon={Activity} label={t("nav.traces")} collapsed={collapsed} />
          <SidebarItem to={ROUTES.EVENTS} icon={Radar} label={t("nav.realtimeEvents")} collapsed={collapsed} />
          <SidebarItem to={ROUTES.ACTIVITY} icon={ClipboardList} label={t("nav.activity")} collapsed={collapsed} />
          <SidebarItem to={ROUTES.LOGS} icon={Terminal} label={t("nav.logs")} collapsed={collapsed} />
        </SidebarGroup>

        {isAdmin && (
        <SidebarGroup label={t("groups.system")} collapsed={collapsed}>
          {isOwner && (
            <SidebarItem to={ROUTES.TENANTS} icon={Building2} label={t("nav.tenants")} collapsed={collapsed} />
          )}
          <SidebarItem to={ROUTES.PROVIDERS} icon={Cpu} label={t("nav.providers")} collapsed={collapsed} />
          <SidebarItem to={ROUTES.CLI_CREDENTIALS} icon={KeyRound} label={t("nav.cliCredentials")} collapsed={collapsed} />
          <SidebarItem to={ROUTES.API_KEYS} icon={KeyRound} label={t("nav.apiKeys")} collapsed={collapsed} />
          <SidebarItem to={ROUTES.PACKAGES} icon={Blocks} label={t("nav.packages")} collapsed={collapsed} />
          {isOwner && (
            <SidebarItem to={ROUTES.CONFIG} icon={Settings} label={t("nav.config")} collapsed={collapsed} />
          )}
          <SidebarItem to={ROUTES.APPROVALS} icon={ShieldCheck} label={t("nav.approvals")} collapsed={collapsed} />
          <SidebarItem to={ROUTES.IMPORT_EXPORT} icon={ArrowLeftRight} label={t("nav.importExport")} collapsed={collapsed} />
          {isOwner && (
            <SidebarItem to={ROUTES.BACKUP_RESTORE} icon={DatabaseBackup} label={t("nav.backupRestore")} collapsed={collapsed} />
          )}
        </SidebarGroup>
        )}
      </nav>

      {/* Footer: connection status */}
      <div className={cn("border-t py-3", collapsed ? "px-2 flex justify-center" : "px-4")}>
        <ConnectionStatus collapsed={collapsed} />
      </div>
    </aside>
  );
}

// WorkflowHistorySidebar shows the last 5 workflow chat sessions in the sidebar.
// Each session has a "..." menu with delete/pin/unpin actions.
// Users only see their own sessions (localStorage per-user).
function WorkflowHistorySidebar() {
  const userId = useAuthStore((s) => s.userId);
  const storageKey = `wf-history:${userId || "anon"}`;

  const [sessions, setSessions] = useState<{ id: string; wfId: string; title: string; pinned: boolean; createdAt: string }[]>(() => {
    try { return JSON.parse(localStorage.getItem(storageKey) || "[]"); }
    catch { return []; }
  });
  const [showAll, setShowAll] = useState(false);
  const [menuOpen, setMenuOpen] = useState<string | null>(null);

  // Sort: pinned first, then by date desc
  const sorted = useMemo(() => {
    return [...sessions].sort((a, b) => {
      if (a.pinned && !b.pinned) return -1;
      if (!a.pinned && b.pinned) return 1;
      return new Date(b.createdAt).getTime() - new Date(a.createdAt).getTime();
    });
  }, [sessions]);

  const visible = showAll ? sorted : sorted.slice(0, 5);

  const persist = (updated: typeof sessions) => {
    setSessions(updated);
    try { localStorage.setItem(storageKey, JSON.stringify(updated)); } catch {}
  };

  const togglePin = (id: string) => {
    persist(sessions.map((s) => s.id === id ? { ...s, pinned: !s.pinned } : s));
    setMenuOpen(null);
  };

  const deleteSession = (id: string) => {
    persist(sessions.filter((s) => s.id !== id));
    // Also clear the chat history for this session
    const session = sessions.find((s) => s.id === id);
    if (session) {
      try { localStorage.removeItem(`wf-chat:${userId || "anon"}:${session.wfId}`); } catch {}
    }
    setMenuOpen(null);
  };

  if (sessions.length === 0) return null;

  return (
    <div className="mt-2 border-t pt-2">
      <div className="px-2 pb-1 text-[10px] font-medium uppercase text-muted-foreground">工作流历史会话</div>
      {visible.map((s) => (
        <div key={s.id} className="group relative flex items-center gap-1 rounded px-2 py-1 text-xs hover:bg-accent">
          <a href={`/workflow/chat/${s.wfId}`} className="min-w-0 flex-1 truncate">
            {s.pinned ? "📌 " : ""}{s.title}
          </a>
          <button
            onClick={(e) => { e.stopPropagation(); setMenuOpen(menuOpen === s.id ? null : s.id); }}
            className="hidden shrink-0 rounded px-1 text-muted-foreground hover:bg-background hover:text-foreground group-hover:block"
          >
            ⋯
          </button>
          {menuOpen === s.id && (
            <div className="absolute right-0 top-full z-50 mt-1 w-28 rounded border bg-card py-1 shadow-lg">
              <button
                onClick={() => togglePin(s.id)}
                className="w-full px-3 py-1 text-left text-xs hover:bg-accent"
              >
                {s.pinned ? "取消置顶" : "置顶"}
              </button>
              <button
                onClick={() => deleteSession(s.id)}
                className="w-full px-3 py-1 text-left text-xs text-red-600 hover:bg-accent"
              >
                删除对话
              </button>
            </div>
          )}
        </div>
      ))}
      {sorted.length > 5 && !showAll && (
        <button
          onClick={() => setShowAll(true)}
          className="w-full px-2 py-1 text-left text-[10px] text-muted-foreground hover:text-foreground"
        >
          查看全部 ({sorted.length})
        </button>
      )}
    </div>
  );
}
