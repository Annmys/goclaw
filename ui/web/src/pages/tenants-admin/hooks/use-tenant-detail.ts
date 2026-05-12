import { useCallback } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import i18next from "i18next";
import { useWs } from "@/hooks/use-ws";
import { queryKeys } from "@/lib/query-keys";
import { toast } from "@/stores/use-toast-store";
import { Methods } from "@/api/protocol";
import type { TenantData, TenantUserData } from "@/types/tenant";

export interface TenantAccessAgent {
  id: string;
  agent_key: string;
  display_name?: string;
  owner_id?: string;
  status: string;
  enabled: boolean;
  is_default?: boolean;
}

export interface TenantAccessTeam {
  id: string;
  name: string;
  description?: string;
  status: string;
  lead_agent_key?: string;
  lead_display_name?: string;
  enabled: boolean;
}

const EMPTY_ACCESS: { agents: TenantAccessAgent[]; teams: TenantAccessTeam[] } = { agents: [], teams: [] };

export function useTenantDetail(tenantId: string) {
  const ws = useWs();
  const queryClient = useQueryClient();

  const { data: tenant, isLoading: tenantLoading } = useQuery({
    queryKey: queryKeys.tenants.detail(tenantId),
    queryFn: async () => {
      const res = await ws.call<TenantData>(Methods.TENANTS_GET, { id: tenantId });
      return res ?? null;
    },
    enabled: !!tenantId,
    staleTime: 60_000,
  });

  const { data: users = [], isLoading: usersLoading, isFetching: usersRefreshing } = useQuery({
    queryKey: queryKeys.tenants.users(tenantId),
    queryFn: async () => {
      const res = await ws.call<{ users: TenantUserData[] }>(Methods.TENANTS_USERS_LIST, { tenant_id: tenantId });
      return res?.users ?? [];
    },
    enabled: !!tenantId,
    staleTime: 60_000,
  });

  const { data: access, isLoading: accessLoading, isFetching: accessRefreshing } = useQuery({
    queryKey: queryKeys.tenants.access(tenantId),
    queryFn: async () => {
      const res = await ws.call<{ agents: TenantAccessAgent[]; teams: TenantAccessTeam[] }>(
        Methods.TENANTS_ACCESS_GET,
        { tenant_id: tenantId },
      );
      return { agents: res?.agents ?? [], teams: res?.teams ?? [] };
    },
    enabled: !!tenantId,
    staleTime: 60_000,
  });

  const invalidateUsers = useCallback(
    () => queryClient.invalidateQueries({ queryKey: queryKeys.tenants.users(tenantId) }),
    [queryClient, tenantId],
  );

  const invalidateTenant = useCallback(
    () => queryClient.invalidateQueries({ queryKey: queryKeys.tenants.detail(tenantId) }),
    [queryClient, tenantId],
  );

  const invalidateAccess = useCallback(
    () => queryClient.invalidateQueries({ queryKey: queryKeys.tenants.access(tenantId) }),
    [queryClient, tenantId],
  );

  const addUser = useCallback(
    async (userId: string, role: string) => {
      try {
        await ws.call(Methods.TENANTS_USERS_ADD, { tenant_id: tenantId, user_id: userId, role });
        await invalidateUsers();
        toast.success(i18next.t("tenants:addUser"), userId);
      } catch (err) {
        toast.error(i18next.t("tenants:addUser"), err instanceof Error ? err.message : "");
        throw err;
      }
    },
    [ws, tenantId, invalidateUsers],
  );

  const removeUser = useCallback(
    async (userId: string) => {
      try {
        await ws.call(Methods.TENANTS_USERS_REMOVE, { tenant_id: tenantId, user_id: userId });
        await invalidateUsers();
        toast.success(i18next.t("tenants:removeUser"), userId);
      } catch (err) {
        toast.error(i18next.t("tenants:removeUser"), err instanceof Error ? err.message : "");
        throw err;
      }
    },
    [ws, tenantId, invalidateUsers],
  );

  const updateUserRole = useCallback(
    async (userId: string, role: string) => {
      try {
        await ws.call(Methods.TENANTS_USERS_ADD, { tenant_id: tenantId, user_id: userId, role });
        await invalidateUsers();
        await queryClient.invalidateQueries({ queryKey: queryKeys.tenants.all });
        toast.success(i18next.t("tenants:updateUserRole"), userId);
      } catch (err) {
        toast.error(i18next.t("tenants:updateUserRole"), err instanceof Error ? err.message : "");
        throw err;
      }
    },
    [ws, tenantId, invalidateUsers, queryClient],
  );

  const updateTenant = useCallback(
    async (updates: { name?: string; status?: string; settings?: Record<string, unknown> }) => {
      try {
        await ws.call(Methods.TENANTS_UPDATE, { id: tenantId, ...updates });
        await invalidateTenant();
        await queryClient.invalidateQueries({ queryKey: queryKeys.tenants.all });
        toast.success(i18next.t("tenants:updateTenant"), tenant?.name ?? tenantId);
      } catch (err) {
        toast.error(i18next.t("tenants:updateTenant"), err instanceof Error ? err.message : "");
        throw err;
      }
    },
    [ws, tenantId, tenant?.name, invalidateTenant, queryClient],
  );

  const deleteTenant = useCallback(
    async () => {
      try {
        await ws.call(Methods.TENANTS_DELETE, { id: tenantId });
        await queryClient.invalidateQueries({ queryKey: queryKeys.tenants.all });
        queryClient.removeQueries({ queryKey: queryKeys.tenants.detail(tenantId) });
        queryClient.removeQueries({ queryKey: queryKeys.tenants.users(tenantId) });
        toast.success(i18next.t("tenants:deleteTenant"), tenant?.name ?? tenantId);
      } catch (err) {
        toast.error(i18next.t("tenants:deleteTenant"), err instanceof Error ? err.message : "");
        throw err;
      }
    },
    [ws, tenantId, tenant?.name, queryClient],
  );

  const updateAccess = useCallback(
    async (agentIds: string[], teamIds: string[]) => {
      try {
        await ws.call(Methods.TENANTS_ACCESS_UPDATE, { tenant_id: tenantId, agent_ids: agentIds, team_ids: teamIds });
        await invalidateAccess();
        await queryClient.invalidateQueries({ queryKey: queryKeys.agents.all });
        await queryClient.invalidateQueries({ queryKey: queryKeys.teams.all });
        toast.success(i18next.t("tenants:updateAccess"), tenant?.name ?? tenantId);
      } catch (err) {
        toast.error(i18next.t("tenants:updateAccess"), err instanceof Error ? err.message : "");
        throw err;
      }
    },
    [ws, tenantId, tenant?.name, invalidateAccess, queryClient],
  );

  return {
    tenant,
    tenantLoading,
    users,
    usersLoading,
    usersRefreshing,
    access: access ?? EMPTY_ACCESS,
    accessLoading,
    accessRefreshing,
    refreshUsers: invalidateUsers,
    refreshAccess: invalidateAccess,
    updateTenant,
    deleteTenant,
    updateAccess,
    addUser,
    updateUserRole,
    removeUser,
  };
}
