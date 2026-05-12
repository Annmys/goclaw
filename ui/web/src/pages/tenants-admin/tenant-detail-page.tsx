import { useEffect, useState, useMemo } from "react";
import { useParams, useNavigate } from "react-router";
import { useTranslation } from "react-i18next";
import { ArrowLeft, Plus, RefreshCw, Users, Trash2, Calendar, Hash, Shield, Save, Bot, Network, Pencil } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import { Badge } from "@/components/ui/badge";
import { Switch } from "@/components/ui/switch";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  DialogFooter,
} from "@/components/ui/dialog";
import { ConfirmDialog } from "@/components/shared/confirm-dialog";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { PageHeader } from "@/components/shared/page-header";
import { EmptyState } from "@/components/shared/empty-state";
import { TableSkeleton } from "@/components/shared/loading-skeleton";
import { UserPickerCombobox } from "@/components/shared/user-picker-combobox";
import { useContactResolver } from "@/hooks/use-contact-resolver";
import { formatUserLabel } from "@/lib/format-user-label";
import { useDeferredLoading } from "@/hooks/use-deferred-loading";
import { useMinLoading } from "@/hooks/use-min-loading";
import { useTenantDetail } from "./hooks/use-tenant-detail";
import { ROUTES } from "@/lib/constants";

const TENANT_ROLES = ["owner", "admin", "operator", "member", "viewer"] as const;
const TENANT_STATUSES = ["active", "suspended", "archived"] as const;
const MASTER_TENANT_ID = "0193a5b0-7000-7000-8000-000000000001";

const ROLE_KEYS: Record<string, string> = {
  owner: "roleOwner", admin: "roleAdmin", operator: "roleOperator",
  member: "roleMember", viewer: "roleViewer",
};

const ROLE_COLORS: Record<string, string> = {
  owner: "bg-amber-100 text-amber-800 dark:bg-amber-900/30 dark:text-amber-300",
  admin: "bg-orange-100 text-orange-800 dark:bg-orange-900/30 dark:text-orange-300",
  operator: "bg-blue-100 text-blue-800 dark:bg-blue-900/30 dark:text-blue-300",
  member: "bg-muted text-muted-foreground",
  viewer: "bg-muted text-muted-foreground",
};

export function TenantDetailPage() {
  const { id = "" } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const { t } = useTranslation("tenants");
  const { t: tc } = useTranslation("common");

  const { tenant, tenantLoading, users, usersLoading, usersRefreshing, access, accessLoading, accessRefreshing, refreshUsers, refreshAccess, updateTenant, deleteTenant, updateAccess, addUser, updateUserRole, removeUser } =
    useTenantDetail(id);

  const spinning = useMinLoading(usersRefreshing);
  const showSkeleton = useDeferredLoading(usersLoading && users.length === 0);

  // Resolve user IDs to display names via contacts
  const userIds = useMemo(() => users.map((u) => u.user_id), [users]);
  const { resolve } = useContactResolver(userIds);

  const [addOpen, setAddOpen] = useState(false);
  const [userId, setUserId] = useState("");
  const [role, setRole] = useState("member");
  const [adding, setAdding] = useState(false);
  const [editRoleUser, setEditRoleUser] = useState<{ userId: string; displayName: string; role: string } | null>(null);
  const [editRole, setEditRole] = useState("member");
  const [roleSaving, setRoleSaving] = useState(false);
  const [removeTarget, setRemoveTarget] = useState<string | null>(null);
  const [removing, setRemoving] = useState(false);
  const [statusSaving, setStatusSaving] = useState(false);
  const [selectedStatus, setSelectedStatus] = useState("");
  const [deleteOpen, setDeleteOpen] = useState(false);
  const [deleting, setDeleting] = useState(false);
  const [selectedAgentIds, setSelectedAgentIds] = useState<Set<string>>(new Set());
  const [selectedTeamIds, setSelectedTeamIds] = useState<Set<string>>(new Set());
  const [accessSaving, setAccessSaving] = useState(false);

  const effectiveStatus = selectedStatus || tenant?.status || "active";
  const isMasterTenant = tenant?.id === MASTER_TENANT_ID;
  const accessSpinning = useMinLoading(accessRefreshing);
  const savedAgentIds = useMemo(() => new Set(access.agents.filter((a) => a.enabled).map((a) => a.id)), [access.agents]);
  const savedTeamIds = useMemo(() => new Set(access.teams.filter((team) => team.enabled).map((team) => team.id)), [access.teams]);
  const accessDirty = !sameSet(selectedAgentIds, savedAgentIds) || !sameSet(selectedTeamIds, savedTeamIds);

  useEffect(() => {
    setSelectedAgentIds(savedAgentIds);
    setSelectedTeamIds(savedTeamIds);
  }, [savedAgentIds, savedTeamIds]);

  const handleAdd = async () => {
    if (!userId.trim()) return;
    setAdding(true);
    try {
      await addUser(userId.trim(), role);
      setAddOpen(false);
      setUserId("");
      setRole("member");
    } finally {
      setAdding(false);
    }
  };

  const handleRemove = async () => {
    if (!removeTarget) return;
    setRemoving(true);
    try {
      await removeUser(removeTarget);
      setRemoveTarget(null);
    } finally {
      setRemoving(false);
    }
  };

  const openRoleEditor = (user: { user_id: string; display_name?: string | null; role: string }) => {
    const displayName = user.display_name || formatUserLabel(user.user_id, resolve);
    setEditRoleUser({ userId: user.user_id, displayName, role: user.role });
    setEditRole(user.role || "member");
  };

  const handleRoleSave = async () => {
    if (!editRoleUser || editRole === editRoleUser.role) return;
    setRoleSaving(true);
    try {
      await updateUserRole(editRoleUser.userId, editRole);
      setEditRoleUser(null);
    } finally {
      setRoleSaving(false);
    }
  };

  const handleStatusSave = async () => {
    if (!tenant || effectiveStatus === tenant.status) return;
    setStatusSaving(true);
    try {
      await updateTenant({ status: effectiveStatus });
      setSelectedStatus("");
    } finally {
      setStatusSaving(false);
    }
  };

  const handleDeleteTenant = async () => {
    if (!tenant) return;
    setDeleting(true);
    try {
      await deleteTenant();
      setDeleteOpen(false);
      navigate(ROUTES.TENANTS);
    } finally {
      setDeleting(false);
    }
  };

  const handleAccessSave = async () => {
    setAccessSaving(true);
    try {
      await updateAccess(Array.from(selectedAgentIds), Array.from(selectedTeamIds));
    } finally {
      setAccessSaving(false);
    }
  };

  if (tenantLoading) {
    return <div className="p-4 sm:p-6 pb-10"><TableSkeleton rows={3} /></div>;
  }

  return (
    <div className="p-4 sm:p-6 space-y-6">
      <PageHeader
        title={tenant?.name ?? t("detail")}
        description=""
        actions={
          <Button variant="outline" size="sm" onClick={() => navigate(ROUTES.TENANTS)} className="gap-1">
            <ArrowLeft className="h-3.5 w-3.5" /> {t("back")}
          </Button>
        }
      />

      {/* Tenant Info Card */}
      {tenant && (
        <div className="grid grid-cols-1 sm:grid-cols-3 gap-3">
          <InfoCard icon={Hash} label={t("slug")} value={tenant.slug} mono />
          <InfoCard icon={Shield} label={t("status")}>
            <Badge variant={tenant.status === "active" ? "default" : tenant.status === "suspended" ? "destructive" : "secondary"}>
              {t(tenant.status) || tenant.status}
            </Badge>
          </InfoCard>
          <InfoCard icon={Calendar} label={t("created")} value={new Date(tenant.created_at).toLocaleDateString()} />
        </div>
      )}

      {tenant && (
        <div className="rounded-lg border p-4">
          <div className="flex flex-col gap-4 lg:flex-row lg:items-end lg:justify-between">
            <div className="space-y-1">
              <h2 className="text-base font-semibold">{t("tenantControls")}</h2>
              <p className="text-sm text-muted-foreground">{t("tenantControlsHelp")}</p>
            </div>
            <div className="flex flex-col gap-2 sm:flex-row sm:items-end">
              <div className="space-y-1.5">
                <Label>{t("status")}</Label>
                <Select value={effectiveStatus} onValueChange={setSelectedStatus}>
                  <SelectTrigger className="w-full text-base md:text-sm sm:w-[180px]"><SelectValue /></SelectTrigger>
                  <SelectContent>
                    {TENANT_STATUSES.map((status) => (
                      <SelectItem key={status} value={status}>{t(status)}</SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
              <Button
                size="sm"
                onClick={handleStatusSave}
                disabled={statusSaving || effectiveStatus === tenant.status}
                className="gap-1"
              >
                <Save className="h-3.5 w-3.5" /> {t("saveStatus")}
              </Button>
              <Button
                variant="destructive"
                size="sm"
                onClick={() => setDeleteOpen(true)}
                disabled={isMasterTenant}
                className="gap-1"
                title={isMasterTenant ? t("masterTenantDeleteBlocked") : t("deleteTenant")}
              >
                <Trash2 className="h-3.5 w-3.5" /> {t("deleteTenant")}
              </Button>
            </div>
          </div>
        </div>
      )}

      {tenant && (
        <div className="rounded-lg border p-4 space-y-4">
          <div className="flex items-center justify-between gap-3">
            <div className="space-y-1">
              <h2 className="text-base font-semibold">{t("tenantAccessTitle")}</h2>
              <p className="text-sm text-muted-foreground">{t("tenantAccessHelp")}</p>
            </div>
            <div className="flex gap-2">
              <Button variant="outline" size="sm" onClick={refreshAccess} disabled={accessSpinning} className="gap-1">
                <RefreshCw className={accessSpinning ? "animate-spin h-3.5 w-3.5" : "h-3.5 w-3.5"} />
              </Button>
              <Button size="sm" onClick={handleAccessSave} disabled={accessSaving || !accessDirty} className="gap-1">
                <Save className="h-3.5 w-3.5" /> {t("saveAccess")}
              </Button>
            </div>
          </div>

          {accessLoading ? (
            <TableSkeleton rows={4} />
          ) : (
            <div className="grid gap-4 lg:grid-cols-2">
              <AccessList
                icon={Bot}
                title={t("availableAgents")}
                empty={t("noAgents")}
                items={access.agents.map((agent) => ({
                  id: agent.id,
                  title: agent.display_name || agent.agent_key,
                  subtitle: [agent.agent_key, agent.owner_id ? `${t("owner")}: ${agent.owner_id}` : ""].filter(Boolean).join(" · "),
                  badge: agent.is_default ? t("defaultVisible") : agent.status,
                  disabled: !!agent.is_default,
                  enabled: selectedAgentIds.has(agent.id),
                }))}
                onToggle={(itemId, enabled) => setSelectedAgentIds((prev) => toggleSet(prev, itemId, enabled))}
              />
              <AccessList
                icon={Network}
                title={t("availableTeams")}
                empty={t("noTeams")}
                items={access.teams.map((team) => ({
                  id: team.id,
                  title: team.name,
                  subtitle: [team.lead_display_name || team.lead_agent_key, team.description].filter(Boolean).join(" · "),
                  badge: team.status,
                  enabled: selectedTeamIds.has(team.id),
                }))}
                onToggle={(itemId, enabled) => setSelectedTeamIds((prev) => toggleSet(prev, itemId, enabled))}
              />
            </div>
          )}
        </div>
      )}

      {/* User Management */}
      <div>
        <div className="flex items-center justify-between mb-3">
          <h2 className="text-base font-semibold flex items-center gap-2">
            <Users className="h-4 w-4 text-muted-foreground" />
            {t("userManagement")}
            {users.length > 0 && (
              <span className="text-xs font-normal text-muted-foreground">({users.length})</span>
            )}
          </h2>
          <div className="flex gap-2">
            <Button size="sm" onClick={() => setAddOpen(true)} className="gap-1">
              <Plus className="h-3.5 w-3.5" /> {t("addUser")}
            </Button>
            <Button variant="outline" size="sm" onClick={refreshUsers} disabled={spinning} className="gap-1">
              <RefreshCw className={spinning ? "animate-spin h-3.5 w-3.5" : "h-3.5 w-3.5"} />
            </Button>
          </div>
        </div>

        {showSkeleton ? (
          <TableSkeleton rows={4} />
        ) : users.length === 0 ? (
          <EmptyState icon={Users} title={t("noUsers")} description="" />
        ) : (
          <div className="grid gap-2">
            {users.map((u) => (
              <div key={u.user_id} className="flex items-center justify-between rounded-lg border px-4 py-3 hover:bg-muted/30 transition-colors">
                <div className="flex items-center gap-3 min-w-0">
                  <div className="flex h-8 w-8 shrink-0 items-center justify-center rounded-full bg-muted text-xs font-medium uppercase">
                    {(u.display_name || formatUserLabel(u.user_id, resolve)).charAt(0)}
                  </div>
                  <div className="min-w-0">
                    <p className="text-sm font-medium truncate">{u.display_name || formatUserLabel(u.user_id, resolve)}</p>
                    <p className="text-xs text-muted-foreground">{new Date(u.created_at).toLocaleDateString()}</p>
                  </div>
                </div>
                <div className="flex items-center gap-2 shrink-0">
                  <span className={`inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium ${ROLE_COLORS[u.role] || ROLE_COLORS.member}`}>
                    {t(ROLE_KEYS[u.role] ?? u.role)}
                  </span>
                  <Button
                    variant="ghost"
                    size="sm"
                    className="h-8 w-8 p-0 text-muted-foreground hover:text-foreground"
                    onClick={() => openRoleEditor(u)}
                    title={t("editRole")}
                  >
                    <Pencil className="h-3.5 w-3.5" />
                  </Button>
                  <Button
                    variant="ghost"
                    size="sm"
                    className="h-8 w-8 p-0 text-muted-foreground hover:text-destructive"
                    onClick={() => setRemoveTarget(u.user_id)}
                    title={t("removeUser")}
                  >
                    <Trash2 className="h-3.5 w-3.5" />
                  </Button>
                </div>
              </div>
            ))}
          </div>
        )}
      </div>

      {/* Add User Dialog */}
      <Dialog open={addOpen} onOpenChange={setAddOpen}>
        <DialogContent className="max-sm:inset-0 max-sm:translate-x-0 max-sm:translate-y-0 sm:max-w-md">
          <DialogHeader>
            <DialogTitle>{t("addUserTitle")}</DialogTitle>
            <DialogDescription>{t("description")}</DialogDescription>
          </DialogHeader>
          <div className="space-y-4 py-2">
            <div className="space-y-1.5">
              <Label>{t("userId")}</Label>
              <UserPickerCombobox
                value={userId}
                onChange={setUserId}
                placeholder="user-id"
                source="tenant_user"
                allowCustom={true}
              />
            </div>
            <div className="space-y-1.5">
              <Label>{t("selectRole")}</Label>
              <Select value={role} onValueChange={setRole}>
                <SelectTrigger className="text-base md:text-sm"><SelectValue /></SelectTrigger>
                <SelectContent>
                  {TENANT_ROLES.map((r) => (
                    <SelectItem key={r} value={r}>{t(ROLE_KEYS[r] ?? r)}</SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setAddOpen(false)} disabled={adding}>{tc("cancel")}</Button>
            <Button onClick={handleAdd} disabled={adding || !userId.trim()}>{t("addUser")}</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={!!editRoleUser} onOpenChange={(open) => { if (!open) setEditRoleUser(null); }}>
        <DialogContent className="max-sm:inset-0 max-sm:translate-x-0 max-sm:translate-y-0 sm:max-w-md">
          <DialogHeader>
            <DialogTitle>{t("editRoleTitle")}</DialogTitle>
            <DialogDescription>{editRoleUser?.displayName}</DialogDescription>
          </DialogHeader>
          <div className="space-y-4 py-2">
            <div className="space-y-1.5">
              <Label>{t("selectRole")}</Label>
              <Select value={editRole} onValueChange={setEditRole}>
                <SelectTrigger className="text-base md:text-sm"><SelectValue /></SelectTrigger>
                <SelectContent>
                  {TENANT_ROLES.map((r) => (
                    <SelectItem key={r} value={r}>{t(ROLE_KEYS[r] ?? r)}</SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setEditRoleUser(null)} disabled={roleSaving}>{tc("cancel")}</Button>
            <Button onClick={handleRoleSave} disabled={roleSaving || !editRoleUser || editRole === editRoleUser.role}>
              {t("saveRole")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <ConfirmDialog
        open={!!removeTarget}
        onOpenChange={(o) => { if (!o) setRemoveTarget(null); }}
        title={t("removeUser")}
        description={t("confirmRemoveUser")}
        confirmLabel={t("removeUser")}
        variant="destructive"
        onConfirm={handleRemove}
        loading={removing}
      />

      <ConfirmDialog
        open={deleteOpen}
        onOpenChange={setDeleteOpen}
        title={t("deleteTenant")}
        description={t("confirmDeleteTenant")}
        confirmLabel={t("deleteTenant")}
        variant="destructive"
        onConfirm={handleDeleteTenant}
        loading={deleting}
      />
    </div>
  );
}

function InfoCard({ icon: Icon, label, value, mono, children }: {
  icon: React.ComponentType<{ className?: string }>;
  label: string;
  value?: string;
  mono?: boolean;
  children?: React.ReactNode;
}) {
  return (
    <div className="rounded-lg border p-3 flex items-start gap-3">
      <div className="flex h-8 w-8 shrink-0 items-center justify-center rounded-md bg-muted">
        <Icon className="h-4 w-4 text-muted-foreground" />
      </div>
      <div className="min-w-0">
        <p className="text-xs text-muted-foreground">{label}</p>
        {children ?? (
          <p className={`text-sm font-medium truncate ${mono ? "font-mono" : ""}`}>{value}</p>
        )}
      </div>
    </div>
  );
}

function AccessList({ icon: Icon, title, empty, items, onToggle }: {
  icon: React.ComponentType<{ className?: string }>;
  title: string;
  empty: string;
  items: Array<{
    id: string;
    title: string;
    subtitle?: string;
    badge?: string;
    enabled: boolean;
    disabled?: boolean;
  }>;
  onToggle: (id: string, enabled: boolean) => void;
}) {
  return (
    <div className="rounded-lg border">
      <div className="flex items-center gap-2 border-b px-4 py-3">
        <Icon className="h-4 w-4 text-muted-foreground" />
        <h3 className="text-sm font-semibold">{title}</h3>
        <span className="text-xs text-muted-foreground">({items.length})</span>
      </div>
      {items.length === 0 ? (
        <div className="p-4 text-sm text-muted-foreground">{empty}</div>
      ) : (
        <div className="divide-y">
          {items.map((item) => (
            <div key={item.id} className="flex items-center justify-between gap-3 px-4 py-3">
              <div className="min-w-0">
                <div className="flex items-center gap-2">
                  <p className="truncate text-sm font-medium">{item.title}</p>
                  {item.badge && <Badge variant="secondary" className="shrink-0">{item.badge}</Badge>}
                </div>
                {item.subtitle && <p className="truncate text-xs text-muted-foreground">{item.subtitle}</p>}
              </div>
              <Switch
                checked={item.enabled}
                disabled={item.disabled}
                onCheckedChange={(enabled) => onToggle(item.id, enabled)}
              />
            </div>
          ))}
        </div>
      )}
    </div>
  );
}

function toggleSet(prev: Set<string>, id: string, enabled: boolean) {
  const next = new Set(prev);
  if (enabled) {
    next.add(id);
  } else {
    next.delete(id);
  }
  return next;
}

function sameSet(a: Set<string>, b: Set<string>) {
  if (a.size !== b.size) return false;
  for (const value of a) {
    if (!b.has(value)) return false;
  }
  return true;
}
