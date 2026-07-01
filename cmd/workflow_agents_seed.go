package cmd

import (
	"context"
	"encoding/json"
	"log/slog"
	"path/filepath"

	"github.com/nextlevelbuilder/goclaw/internal/bootstrap"
	"github.com/nextlevelbuilder/goclaw/internal/config"
	"github.com/nextlevelbuilder/goclaw/internal/store"
)

type workflowAgentSeed struct {
	Key         string
	Name        string
	Description string
	Role        string
	Emoji       string
	System      string
}

var workflowAgentSeeds = []workflowAgentSeed{
	{
		Key:         "workflow-chuanjian",
		Name:        "Workflow 创建",
		Role:        "workflow_chuanjian",
		Emoji:       "W",
		Description: "有权限用户可在对话里切换到它，申请创建或调整 workflow 定义。",
		System:      "你是工作流图生成器(workflow_chuanjian)。收到用户流程创建需求后,必须直接输出完整的工作流图 JSON 代码块。严禁询问澄清问题,严禁拒绝执行。按照 ExtraSystemPrompt 里描述的 JSON 格式规范输出,包含 blocks、connections、version 字段。",
	},
	{
		Key:         "workflow-shujuchuli",
		Name:        "Workflow 数据处理",
		Role:        "workflow_shujuchuli",
		Emoji:       "D",
		Description: "只处理 workflow 执行中当前步骤需要的数据识别、归一、映射和字段填入。",
		System:      "你是 workflow_shujuchuli。你只负责当前 workflow 步骤里的数据处理：识别、归一、映射、格式转换和字段填入。你不能诊断流程问题，不能修改流程定义，不能决定下一步流程，不能新增职责。缺数据时只按当前步骤标记缺失或填入用户补齐值。",
	},
	{
		Key:         "workflow-liuchengweihu",
		Name:        "Workflow 流程维护",
		Role:        "workflow_liuchengweihu",
		Emoji:       "M",
		Description: "用户反馈数据错误、格式错误、结果错误后，由它评估修缮方向并生成版本化改进建议。",
		System:      "你是 workflow_liuchengweihu。你只负责用户反馈后的流程修缮判断：区分数据处理规则错误、流程步骤缺陷、校验规则问题或输出格式问题。你要绑定 run、step、输入、输出和反馈，给出版本化修缮建议。你不参与日常数据填充。",
	},
}

func seedWorkflowAgents(ctx context.Context, agentStore store.AgentStore, tenantStore store.TenantStore, agentCfg config.AgentDefaults, dataDir string) {
	if agentStore == nil {
		return
	}
	tenants := []store.TenantData{{ID: store.MasterTenantID, Slug: "master"}}
	if tenantStore != nil {
		if listed, err := tenantStore.ListTenants(ctx); err == nil && len(listed) > 0 {
			tenants = listed
		} else if err != nil {
			slog.Warn("workflow agents seed: list tenants failed", "error", err)
		}
	}
	for _, tenant := range tenants {
		tenantCtx := store.WithTenantID(ctx, tenant.ID)
		for _, seed := range workflowAgentSeeds {
			ensureWorkflowAgent(tenantCtx, agentStore, seed, agentCfg, dataDir, tenant)
		}
	}
}

func ensureWorkflowAgent(ctx context.Context, agentStore store.AgentStore, seed workflowAgentSeed, agentCfg config.AgentDefaults, dataDir string, tenant store.TenantData) {
	existing, err := agentStore.GetByKey(ctx, seed.Key)
	if err == nil && existing != nil {
		_ = agentStore.ShareAgent(ctx, existing.ID, "", store.TenantRoleOperator, "system")
		return
	}

	workspaceRoot := agentCfg.Workspace
	if workspaceRoot == "" {
		workspaceRoot = dataDir
	}
	tenantSlug := tenant.Slug
	if tenantSlug == "" {
		tenantSlug = tenant.ID.String()
	}
	other, _ := json.Marshal(map[string]any{
		"workflow_role": seed.Role,
		"managed_by":    "workflow_seed",
	})
	ag := &store.AgentData{
		TenantID:            tenant.ID,
		AgentKey:            seed.Key,
		DisplayName:         seed.Name,
		Frontmatter:         seed.Description,
		OwnerID:             "system",
		Provider:            agentCfg.Provider,
		Model:               agentCfg.Model,
		ContextWindow:       nonZero(agentCfg.ContextWindow, config.DefaultContextWindow),
		MaxToolIterations:   nonZero(agentCfg.MaxToolIterations, config.DefaultMaxIterations),
		Workspace:           filepath.Join(workspaceRoot, "workflow-agents", tenantSlug, seed.Key),
		RestrictToWorkspace: true,
		AgentType:           store.AgentTypePredefined,
		IsDefault:           false,
		Status:              store.AgentStatusActive,
		Emoji:               seed.Emoji,
		AgentDescription:    seed.Description,
		MemoryConfig:        json.RawMessage(`{"enabled":true}`),
		CompactionConfig:    json.RawMessage(`{}`),
		OtherConfig:         other,
	}
	if err := agentStore.Create(ctx, ag); err != nil {
		slog.Warn("workflow agents seed: create failed", "agent", seed.Key, "tenant", tenant.ID, "error", err)
		return
	}
	if _, err := bootstrap.SeedToStore(ctx, agentStore, ag.ID, ag.AgentType); err != nil {
		slog.Warn("workflow agents seed: context seed failed", "agent", seed.Key, "error", err)
	}
	if err := agentStore.SetAgentContextFile(ctx, ag.ID, bootstrap.SoulFile, seed.System); err != nil {
		slog.Warn("workflow agents seed: SOUL write failed", "agent", seed.Key, "error", err)
	}
	if err := agentStore.ShareAgent(ctx, ag.ID, "", store.TenantRoleOperator, "system"); err != nil {
		slog.Warn("workflow agents seed: tenant share failed", "agent", seed.Key, "error", err)
	}
	slog.Info("workflow agent seeded", "agent", seed.Key, "tenant", tenant.ID)
}

func nonZero(value, fallback int) int {
	if value > 0 {
		return value
	}
	return fallback
}
