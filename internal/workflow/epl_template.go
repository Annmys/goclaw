package workflow

import (
	"context"

	"github.com/nextlevelbuilder/goclaw/internal/workflow/graph"
)

// EPLTemplateName is the display name of the seeded EPL workflow.
const EPLTemplateName = "预估箱单制作(EPL)"

// buildEPLGraph constructs a ready-to-run EPL packing-list workflow:
//
//	trigger(收到含 CI 的 xlsx)
//	  → agent(船务专员,加载 epl-core-workflow 技能,执行 6 步)
//	  → response(返回生成的 EPL 文件/说明)
//
// The agent node carries the EPL business logic by loading the existing
// epl-core-workflow skill (复制CI→动态定位→查流转单算重量→合并居中→改Total),
// rather than re-encoding hundreds of lines of openpyxl as canvas nodes.
func buildEPLGraph(agentKey string) graph.Graph {
	return graph.Graph{
		Version: graph.Version,
		Blocks: []graph.Block{
			{
				ID:       "trigger",
				Position: graph.Position{X: 40, Y: 120},
				Metadata: &graph.BlockMetadata{ID: graph.TypeTrigger, Name: "上传船务清单"},
				Enabled:  true,
			},
			{
				ID:       "epl",
				Position: graph.Position{X: 300, Y: 120},
				Metadata: &graph.BlockMetadata{ID: graph.TypeAgent, Name: "EPL 制作(船务专员)"},
				Config: graph.BlockConfig{
					Params: map[string]any{
						"agent":  agentKey,
						"skills": "epl-core-workflow",
						"prompt": "请根据上传的船务清单(含 CI sheet)制作预估箱单(EPL)。" +
							"严格按 epl-core-workflow 技能的 6 步执行:复制 CI sheet→动态定位结构→" +
							"调整表头→查询流转单与重量→填入数据并合并居中→修正 Total 行。" +
							"用户输入:<trigger.message>",
					},
				},
				Enabled: true,
			},
			{
				ID:       "result",
				Position: graph.Position{X: 560, Y: 120},
				Metadata: &graph.BlockMetadata{ID: graph.TypeResponse, Name: "返回 EPL 结果"},
				Config: graph.BlockConfig{
					Params: map[string]any{"content": "<epl.content>"},
				},
				Enabled: true,
			},
		},
		Connections: []graph.Connection{
			{Source: "trigger", Target: "epl", SourceHandle: graph.HandleSource},
			{Source: "epl", Target: "result", SourceHandle: graph.HandleSource},
		},
	}
}

// SeedEPLTemplate creates (or returns the existing) EPL workflow definition for
// the caller's tenant/user, using a shipping-capable agent. Returns the
// definition id. Idempotent by name within the scope.
func (e *Engine) SeedEPLTemplate(ctx context.Context, agentKey string) (*GraphDefinition, error) {
	// reuse an existing one with the same name if present
	existing, err := e.ListDefinitions(ctx)
	if err == nil {
		for i := range existing {
			if existing[i].Name == EPLTemplateName {
				return &existing[i], nil
			}
		}
	}
	if agentKey == "" {
		agentKey = e.resolveShippingAgent(ctx)
	}
	def := &GraphDefinition{
		Name:        EPLTemplateName,
		Description: "上传含 CI sheet 的 xlsx,自动按 epl-core-workflow 技能制作预估箱单。",
		Graph:       buildEPLGraph(agentKey),
		Active:      true,
	}
	if err := e.SaveDefinition(ctx, def); err != nil {
		return nil, err
	}
	return def, nil
}

// resolveShippingAgent picks an agent suited to EPL: prefers a shipping agent
// (hr-shipping), else any available agent.
func (e *Engine) resolveShippingAgent(ctx context.Context) string {
	const preferred = "hr-shipping"
	if e.runner == nil || e.runner.ListAgents == nil {
		return preferred
	}
	avail := e.runner.ListAgents(ctx)
	for _, id := range avail {
		if id == preferred {
			return id
		}
	}
	if len(avail) > 0 {
		return avail[0]
	}
	return preferred
}
