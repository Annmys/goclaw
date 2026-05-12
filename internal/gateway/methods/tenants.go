package methods

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/bus"
	"github.com/nextlevelbuilder/goclaw/internal/gateway"
	"github.com/nextlevelbuilder/goclaw/internal/i18n"
	"github.com/nextlevelbuilder/goclaw/internal/permissions"
	"github.com/nextlevelbuilder/goclaw/internal/store"
	"github.com/nextlevelbuilder/goclaw/pkg/protocol"
)

var slugRe = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?$`)

// TenantsMethods handles tenant management RPC methods.
type TenantsMethods struct {
	tenantStore store.TenantStore
	agentStore  store.AgentStore
	teamStore   store.TeamStore
	msgBus      *bus.MessageBus
	workspace   string // base workspace directory for tenant dirs
}

// NewTenantsMethods creates a new TenantsMethods handler.
func NewTenantsMethods(tenantStore store.TenantStore, agentStore store.AgentStore, teamStore store.TeamStore, msgBus *bus.MessageBus, workspace string) *TenantsMethods {
	return &TenantsMethods{tenantStore: tenantStore, agentStore: agentStore, teamStore: teamStore, msgBus: msgBus, workspace: workspace}
}

// Register registers tenant management RPC methods.
func (m *TenantsMethods) Register(router *gateway.MethodRouter) {
	router.Register(protocol.MethodTenantsList, m.handleList)
	router.Register(protocol.MethodTenantsGet, m.handleGet)
	router.Register(protocol.MethodTenantsCreate, m.handleCreate)
	router.Register(protocol.MethodTenantsUpdate, m.handleUpdate)
	router.Register(protocol.MethodTenantsDelete, m.handleDelete)
	router.Register(protocol.MethodTenantsAccessGet, m.handleAccessGet)
	router.Register(protocol.MethodTenantsAccessUpdate, m.handleAccessUpdate)
	router.Register(protocol.MethodTenantsUsersList, m.handleUsersList)
	router.Register(protocol.MethodTenantsUsersAdd, m.handleUsersAdd)
	router.Register(protocol.MethodTenantsUsersRemove, m.handleUsersRemove)
	router.Register(protocol.MethodTenantsMine, m.handleMine)
}

func (m *TenantsMethods) handleList(ctx context.Context, client *gateway.Client, req *protocol.RequestFrame) {
	locale := store.LocaleFromContext(ctx)
	if !client.IsOwner() {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrUnauthorized, i18n.T(locale, i18n.MsgPermissionDenied, "tenants.list")))
		return
	}

	tenants, err := m.tenantStore.ListTenants(ctx)
	if err != nil {
		slog.Error("tenants.list failed", "error", err)
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInternal, i18n.T(locale, i18n.MsgFailedToList, "tenants")))
		return
	}
	if tenants == nil {
		tenants = []store.TenantData{}
	}
	client.SendResponse(protocol.NewOKResponse(req.ID, map[string]any{"tenants": tenants}))
}

func (m *TenantsMethods) handleGet(ctx context.Context, client *gateway.Client, req *protocol.RequestFrame) {
	locale := store.LocaleFromContext(ctx)
	if !client.IsOwner() {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrUnauthorized, i18n.T(locale, i18n.MsgPermissionDenied, "tenants.get")))
		return
	}

	var params struct {
		ID string `json:"id"`
	}
	if req.Params != nil {
		if err := json.Unmarshal(req.Params, &params); err != nil {
			client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgInvalidJSON)))
			return
		}
	}

	id, err := uuid.Parse(params.ID)
	if err != nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgInvalidID, "tenant")))
		return
	}

	tenant, err := m.tenantStore.GetTenant(ctx, id)
	if err != nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrNotFound, i18n.T(locale, i18n.MsgNotFound, "tenant", params.ID)))
		return
	}
	client.SendResponse(protocol.NewOKResponse(req.ID, tenant))
}

func (m *TenantsMethods) handleCreate(ctx context.Context, client *gateway.Client, req *protocol.RequestFrame) {
	locale := store.LocaleFromContext(ctx)
	if !client.IsOwner() && !client.HasScope(permissions.ScopeProvision) {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrUnauthorized, i18n.T(locale, i18n.MsgPermissionDenied, "tenants.create")))
		return
	}

	var params struct {
		Name     string `json:"name"`
		Slug     string `json:"slug"`
		Settings any    `json:"settings"`
	}
	if req.Params != nil {
		if err := json.Unmarshal(req.Params, &params); err != nil {
			client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgInvalidJSON)))
			return
		}
	}

	if params.Name == "" {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgRequired, "name")))
		return
	}
	if params.Slug == "" {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgRequired, "slug")))
		return
	}
	if !slugRe.MatchString(params.Slug) {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgInvalidSlug, "slug")))
		return
	}

	tenant := &store.TenantData{
		ID:     store.GenNewID(),
		Name:   params.Name,
		Slug:   params.Slug,
		Status: store.TenantStatusActive,
	}

	if err := m.tenantStore.CreateTenant(ctx, tenant); err != nil {
		slog.Error("tenants.create failed", "error", err)
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInternal, i18n.T(locale, i18n.MsgFailedToCreate, "tenant", err.Error())))
		return
	}

	// Create workspace directory for the tenant.
	if m.workspace != "" {
		tenantDir := filepath.Join(m.workspace, "tenants", tenant.Slug)
		if err := os.MkdirAll(tenantDir, 0755); err != nil {
			slog.Warn("tenants.create: failed to create workspace dir", "dir", tenantDir, "error", err)
		}
	}

	m.emitCacheInvalidate(bus.CacheKindTenantUsers, tenant.ID.String())
	m.emitCacheInvalidate(bus.CacheKindTenants, "")
	client.SendResponse(protocol.NewOKResponse(req.ID, tenant))
}

func (m *TenantsMethods) handleUpdate(ctx context.Context, client *gateway.Client, req *protocol.RequestFrame) {
	locale := store.LocaleFromContext(ctx)
	if !client.IsOwner() {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrUnauthorized, i18n.T(locale, i18n.MsgPermissionDenied, "tenants.update")))
		return
	}

	var params struct {
		ID       string         `json:"id"`
		Name     string         `json:"name"`
		Status   string         `json:"status"`
		Settings map[string]any `json:"settings"`
	}
	if req.Params != nil {
		if err := json.Unmarshal(req.Params, &params); err != nil {
			client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgInvalidJSON)))
			return
		}
	}

	id, err := uuid.Parse(params.ID)
	if err != nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgInvalidID, "tenant")))
		return
	}

	updates := make(map[string]any)
	if params.Name != "" {
		updates["name"] = params.Name
	}
	if params.Status != "" {
		if !store.IsValidTenantStatus(params.Status) {
			client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgInvalidRequest, "invalid tenant status")))
			return
		}
		updates["status"] = params.Status
	}
	if params.Settings != nil {
		updates["settings"] = params.Settings
	}

	if len(updates) == 0 {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgInvalidUpdates)))
		return
	}

	if err := m.tenantStore.UpdateTenant(ctx, id, updates); err != nil {
		slog.Error("tenants.update failed", "error", err)
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInternal, i18n.T(locale, i18n.MsgFailedToUpdate, "tenant", err.Error())))
		return
	}

	m.emitCacheInvalidate(bus.CacheKindTenantUsers, id.String())
	m.emitCacheInvalidate(bus.CacheKindTenants, "")
	client.SendResponse(protocol.NewOKResponse(req.ID, map[string]string{"ok": "true"}))
}

func (m *TenantsMethods) handleDelete(ctx context.Context, client *gateway.Client, req *protocol.RequestFrame) {
	locale := store.LocaleFromContext(ctx)
	if !client.IsOwner() {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrUnauthorized, i18n.T(locale, i18n.MsgPermissionDenied, "tenants.delete")))
		return
	}

	var params struct {
		ID string `json:"id"`
	}
	if req.Params != nil {
		if err := json.Unmarshal(req.Params, &params); err != nil {
			client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgInvalidJSON)))
			return
		}
	}

	id, err := uuid.Parse(params.ID)
	if err != nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgInvalidID, "tenant")))
		return
	}
	if id == store.MasterTenantID {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, "master tenant cannot be deleted"))
		return
	}

	if err := m.tenantStore.DeleteTenant(ctx, id); err != nil {
		slog.Error("tenants.delete failed", "error", err)
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInternal, i18n.T(locale, i18n.MsgFailedToDelete, "tenant", err.Error())))
		return
	}

	m.emitCacheInvalidate(bus.CacheKindTenantUsers, id.String())
	m.emitCacheInvalidate(bus.CacheKindTenants, "")
	client.SendResponse(protocol.NewOKResponse(req.ID, map[string]string{"ok": "true"}))
}

type tenantAccessAgentEntry struct {
	ID          string `json:"id"`
	AgentKey    string `json:"agent_key"`
	DisplayName string `json:"display_name"`
	OwnerID     string `json:"owner_id"`
	Status      string `json:"status"`
	Enabled     bool   `json:"enabled"`
	IsDefault   bool   `json:"is_default"`
}

type tenantAccessTeamEntry struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	Description     string `json:"description"`
	Status          string `json:"status"`
	LeadAgentKey    string `json:"lead_agent_key"`
	LeadDisplayName string `json:"lead_display_name"`
	Enabled         bool   `json:"enabled"`
}

func (m *TenantsMethods) handleAccessGet(ctx context.Context, client *gateway.Client, req *protocol.RequestFrame) {
	locale := store.LocaleFromContext(ctx)
	if !client.IsOwner() {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrUnauthorized, i18n.T(locale, i18n.MsgPermissionDenied, "tenants.access.get")))
		return
	}
	if m.agentStore == nil || m.teamStore == nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInternal, "agent/team store is not configured"))
		return
	}

	tid, ok := parseTenantIDParam(req)
	if !ok {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgInvalidID, "tenant_id")))
		return
	}
	scopedCtx := store.WithTenantID(ctx, tid)

	agents, err := m.agentStore.List(scopedCtx, "")
	if err != nil {
		slog.Error("tenants.access.get agents failed", "error", err)
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInternal, i18n.T(locale, i18n.MsgFailedToList, "agents")))
		return
	}
	teams, err := m.teamStore.ListTeams(scopedCtx)
	if err != nil {
		slog.Error("tenants.access.get teams failed", "error", err)
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInternal, i18n.T(locale, i18n.MsgFailedToList, "teams")))
		return
	}

	agentEntries := make([]tenantAccessAgentEntry, 0, len(agents))
	for _, ag := range agents {
		enabled := ag.IsDefault
		if !enabled {
			shares, shareErr := m.agentStore.ListShares(scopedCtx, ag.ID)
			if shareErr != nil {
				slog.Error("tenants.access.get agent shares failed", "agent_id", ag.ID, "error", shareErr)
				client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInternal, i18n.T(locale, i18n.MsgFailedToList, "agent shares")))
				return
			}
			for _, share := range shares {
				if share.UserID == store.TenantWideUserID {
					enabled = true
					break
				}
			}
		}
		agentEntries = append(agentEntries, tenantAccessAgentEntry{
			ID: ag.ID.String(), AgentKey: ag.AgentKey, DisplayName: ag.DisplayName,
			OwnerID: ag.OwnerID, Status: ag.Status, Enabled: enabled, IsDefault: ag.IsDefault,
		})
	}

	teamEntries := make([]tenantAccessTeamEntry, 0, len(teams))
	for _, team := range teams {
		enabled := false
		grants, grantErr := m.teamStore.ListTeamGrants(scopedCtx, team.ID)
		if grantErr != nil {
			slog.Error("tenants.access.get team grants failed", "team_id", team.ID, "error", grantErr)
			client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInternal, i18n.T(locale, i18n.MsgFailedToList, "team grants")))
			return
		}
		for _, grant := range grants {
			if grant.UserID == store.TenantWideUserID {
				enabled = true
				break
			}
		}
		teamEntries = append(teamEntries, tenantAccessTeamEntry{
			ID: team.ID.String(), Name: team.Name, Description: team.Description, Status: team.Status,
			LeadAgentKey: team.LeadAgentKey, LeadDisplayName: team.LeadDisplayName, Enabled: enabled,
		})
	}

	client.SendResponse(protocol.NewOKResponse(req.ID, map[string]any{
		"agents": agentEntries,
		"teams":  teamEntries,
	}))
}

func (m *TenantsMethods) handleAccessUpdate(ctx context.Context, client *gateway.Client, req *protocol.RequestFrame) {
	locale := store.LocaleFromContext(ctx)
	if !client.IsOwner() {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrUnauthorized, i18n.T(locale, i18n.MsgPermissionDenied, "tenants.access.update")))
		return
	}
	if m.agentStore == nil || m.teamStore == nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInternal, "agent/team store is not configured"))
		return
	}

	var params struct {
		TenantID string   `json:"tenant_id"`
		AgentIDs []string `json:"agent_ids"`
		TeamIDs  []string `json:"team_ids"`
	}
	if req.Params != nil {
		if err := json.Unmarshal(req.Params, &params); err != nil {
			client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgInvalidJSON)))
			return
		}
	}
	tid, err := uuid.Parse(params.TenantID)
	if err != nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgInvalidID, "tenant_id")))
		return
	}
	scopedCtx := store.WithTenantID(ctx, tid)

	agentSet, err := parseUUIDSet(params.AgentIDs)
	if err != nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgInvalidID, "agent_id")))
		return
	}
	teamSet, err := parseUUIDSet(params.TeamIDs)
	if err != nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgInvalidID, "team_id")))
		return
	}

	agents, err := m.agentStore.List(scopedCtx, "")
	if err != nil {
		slog.Error("tenants.access.update agents failed", "error", err)
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInternal, i18n.T(locale, i18n.MsgFailedToList, "agents")))
		return
	}
	teams, err := m.teamStore.ListTeams(scopedCtx)
	if err != nil {
		slog.Error("tenants.access.update teams failed", "error", err)
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInternal, i18n.T(locale, i18n.MsgFailedToList, "teams")))
		return
	}

	for _, ag := range agents {
		if agentSet[ag.ID] {
			if err := m.agentStore.ShareAgent(scopedCtx, ag.ID, store.TenantWideUserID, "user", client.UserID()); err != nil {
				slog.Error("tenants.access.update share agent failed", "agent_id", ag.ID, "error", err)
				client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInternal, i18n.T(locale, i18n.MsgFailedToUpdate, "agent access", err.Error())))
				return
			}
		} else {
			if err := m.agentStore.RevokeShare(scopedCtx, ag.ID, store.TenantWideUserID); err != nil {
				slog.Error("tenants.access.update revoke agent failed", "agent_id", ag.ID, "error", err)
				client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInternal, i18n.T(locale, i18n.MsgFailedToUpdate, "agent access", err.Error())))
				return
			}
		}
	}

	for _, team := range teams {
		if teamSet[team.ID] {
			if err := m.teamStore.GrantTeamAccess(scopedCtx, team.ID, store.TenantWideUserID, store.TeamRoleMember, client.UserID()); err != nil {
				slog.Error("tenants.access.update grant team failed", "team_id", team.ID, "error", err)
				client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInternal, i18n.T(locale, i18n.MsgFailedToUpdate, "team access", err.Error())))
				return
			}
		} else {
			if err := m.teamStore.RevokeTeamAccess(scopedCtx, team.ID, store.TenantWideUserID); err != nil {
				slog.Error("tenants.access.update revoke team failed", "team_id", team.ID, "error", err)
				client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInternal, i18n.T(locale, i18n.MsgFailedToUpdate, "team access", err.Error())))
				return
			}
		}
	}

	m.emitCacheInvalidate(bus.CacheKindAgentAccess, tid.String())
	m.emitCacheInvalidate(bus.CacheKindTeamAccess, tid.String())
	client.SendResponse(protocol.NewOKResponse(req.ID, map[string]string{"ok": "true"}))
}

func (m *TenantsMethods) handleUsersList(ctx context.Context, client *gateway.Client, req *protocol.RequestFrame) {
	locale := store.LocaleFromContext(ctx)
	if !client.IsOwner() {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrUnauthorized, i18n.T(locale, i18n.MsgPermissionDenied, "tenants.users.list")))
		return
	}

	var params struct {
		TenantID string `json:"tenant_id"`
	}
	if req.Params != nil {
		if err := json.Unmarshal(req.Params, &params); err != nil {
			client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgInvalidJSON)))
			return
		}
	}

	tid, err := uuid.Parse(params.TenantID)
	if err != nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgInvalidID, "tenant_id")))
		return
	}

	users, err := m.tenantStore.ListUsers(ctx, tid)
	if err != nil {
		slog.Error("tenants.users.list failed", "error", err)
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInternal, i18n.T(locale, i18n.MsgFailedToList, "tenant users")))
		return
	}
	if users == nil {
		users = []store.TenantUserData{}
	}
	client.SendResponse(protocol.NewOKResponse(req.ID, map[string]any{"users": users}))
}

func parseTenantIDParam(req *protocol.RequestFrame) (uuid.UUID, bool) {
	var params struct {
		ID       string `json:"id"`
		TenantID string `json:"tenant_id"`
	}
	if req.Params != nil {
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return uuid.Nil, false
		}
	}
	id := params.TenantID
	if id == "" {
		id = params.ID
	}
	tid, err := uuid.Parse(id)
	if err != nil {
		return uuid.Nil, false
	}
	return tid, true
}

func parseUUIDSet(ids []string) (map[uuid.UUID]bool, error) {
	result := make(map[uuid.UUID]bool, len(ids))
	for _, raw := range ids {
		id, err := uuid.Parse(raw)
		if err != nil {
			return nil, err
		}
		result[id] = true
	}
	return result, nil
}

func (m *TenantsMethods) handleUsersAdd(ctx context.Context, client *gateway.Client, req *protocol.RequestFrame) {
	locale := store.LocaleFromContext(ctx)
	if !client.IsOwner() && !client.HasScope(permissions.ScopeProvision) {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrUnauthorized, i18n.T(locale, i18n.MsgPermissionDenied, "tenants.users.add")))
		return
	}

	var params struct {
		TenantID string `json:"tenant_id"`
		UserID   string `json:"user_id"`
		Role     string `json:"role"`
	}
	if req.Params != nil {
		if err := json.Unmarshal(req.Params, &params); err != nil {
			client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgInvalidJSON)))
			return
		}
	}

	if params.UserID == "" {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgRequired, "user_id")))
		return
	}
	if params.Role == "" {
		params.Role = store.TenantRoleMember
	}
	validRoles := map[string]bool{
		store.TenantRoleOwner: true, store.TenantRoleAdmin: true,
		store.TenantRoleOperator: true, store.TenantRoleMember: true, store.TenantRoleViewer: true,
	}
	if !validRoles[params.Role] {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgInvalidRole)))
		return
	}

	tid, err := uuid.Parse(params.TenantID)
	if err != nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgInvalidID, "tenant_id")))
		return
	}

	if err := m.tenantStore.AddUser(ctx, tid, params.UserID, params.Role); err != nil {
		slog.Error("tenants.users.add failed", "error", err)
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInternal, i18n.T(locale, i18n.MsgFailedToCreate, "tenant user", err.Error())))
		return
	}

	m.emitCacheInvalidate(bus.CacheKindTenantUsers, params.UserID)
	client.SendResponse(protocol.NewOKResponse(req.ID, map[string]string{"ok": "true"}))
}

func (m *TenantsMethods) handleUsersRemove(ctx context.Context, client *gateway.Client, req *protocol.RequestFrame) {
	locale := store.LocaleFromContext(ctx)
	if !client.IsOwner() {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrUnauthorized, i18n.T(locale, i18n.MsgPermissionDenied, "tenants.users.remove")))
		return
	}

	var params struct {
		TenantID string `json:"tenant_id"`
		UserID   string `json:"user_id"`
	}
	if req.Params != nil {
		if err := json.Unmarshal(req.Params, &params); err != nil {
			client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgInvalidJSON)))
			return
		}
	}

	if params.UserID == "" {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgRequired, "user_id")))
		return
	}

	tid, err := uuid.Parse(params.TenantID)
	if err != nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgInvalidID, "tenant_id")))
		return
	}

	if err := m.tenantStore.RemoveUser(ctx, tid, params.UserID); err != nil {
		slog.Error("tenants.users.remove failed", "error", err)
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInternal, i18n.T(locale, i18n.MsgFailedToDelete, "tenant user", err.Error())))
		return
	}

	m.emitCacheInvalidate(bus.CacheKindTenantUsers, params.UserID)

	// Notify affected user's WS sessions to force logout
	m.msgBus.Broadcast(bus.Event{
		Name:    protocol.EventTenantAccessRevoked,
		Payload: map[string]string{"user_id": params.UserID, "tenant_id": tid.String()},
	})

	client.SendResponse(protocol.NewOKResponse(req.ID, map[string]string{"ok": "true"}))
}

// handleMine returns the current user's tenant memberships.
// Unlike other tenant methods, this does NOT require cross-tenant access.
// Cross-tenant admins receive all tenants instead.
func (m *TenantsMethods) handleMine(ctx context.Context, client *gateway.Client, req *protocol.RequestFrame) {
	locale := store.LocaleFromContext(ctx)

	type tenantEntry struct {
		ID     string `json:"id"`
		Name   string `json:"name"`
		Slug   string `json:"slug"`
		Role   string `json:"role"`
		Status string `json:"status"`
	}

	// Owner: return all tenants with "owner" role
	if client.IsOwner() {
		tenants, err := m.tenantStore.ListTenants(ctx)
		if err != nil {
			slog.Error("tenants.mine failed (cross-tenant)", "error", err)
			client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInternal, i18n.T(locale, i18n.MsgFailedToList, "tenants")))
			return
		}
		entries := make([]tenantEntry, len(tenants))
		for i, t := range tenants {
			entries[i] = tenantEntry{ID: t.ID.String(), Name: t.Name, Slug: t.Slug, Role: "owner", Status: t.Status}
		}
		client.SendResponse(protocol.NewOKResponse(req.ID, map[string]any{"tenants": entries}))
		return
	}

	// Regular user: return their tenant memberships enriched with name/slug
	userID := client.UserID()
	if userID == "" {
		client.SendResponse(protocol.NewOKResponse(req.ID, map[string]any{"tenants": []tenantEntry{}}))
		return
	}

	memberships, err := m.tenantStore.ListUserTenants(ctx, userID)
	if err != nil {
		slog.Error("tenants.mine failed", "error", err, "user_id", userID)
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInternal, i18n.T(locale, i18n.MsgFailedToList, "tenants")))
		return
	}

	// Batch-fetch all tenant data in a single query instead of per-membership.
	ids := make([]uuid.UUID, 0, len(memberships))
	for _, mem := range memberships {
		ids = append(ids, mem.TenantID)
	}
	tenants, tErr := m.tenantStore.GetTenantsByIDs(ctx, ids)
	if tErr != nil {
		slog.Error("tenants.mine: batch fetch failed", "error", tErr, "user_id", userID)
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInternal, i18n.T(locale, i18n.MsgFailedToList, "tenants")))
		return
	}
	tenantMap := make(map[uuid.UUID]*store.TenantData, len(tenants))
	for i := range tenants {
		tenantMap[tenants[i].ID] = &tenants[i]
	}

	entries := make([]tenantEntry, 0, len(memberships))
	for _, mem := range memberships {
		t := tenantMap[mem.TenantID]
		if t == nil || t.Status != store.TenantStatusActive {
			continue
		}
		entries = append(entries, tenantEntry{ID: t.ID.String(), Name: t.Name, Slug: t.Slug, Role: mem.Role, Status: t.Status})
	}

	client.SendResponse(protocol.NewOKResponse(req.ID, map[string]any{"tenants": entries}))
}

func (m *TenantsMethods) emitCacheInvalidate(kind, key string) {
	if m.msgBus == nil {
		return
	}
	m.msgBus.Broadcast(bus.Event{
		Name:    protocol.EventCacheInvalidate,
		Payload: bus.CacheInvalidatePayload{Kind: kind, Key: key},
	})
}
