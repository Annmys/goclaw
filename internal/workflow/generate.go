package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"regexp"
	"strings"

	"github.com/nextlevelbuilder/goclaw/internal/agent"
	"github.com/nextlevelbuilder/goclaw/internal/workflow/graph"
)

// GenerateRequest is a natural-language request to author or edit a workflow.
type GenerateRequest struct {
	// Prompt is the user's natural-language description.
	Prompt string
	// AgentID is the agent used as the "copilot brain". When empty, the engine
	// falls back to the first configured workflow-authoring agent.
	AgentID string
	// Current is the existing graph (for incremental edits); nil for new flows.
	Current *graph.Graph
}

// GenerateResult carries the produced graph plus the agent's explanation.
type GenerateResult struct {
	Graph       graph.Graph `json:"graph"`
	Explanation string      `json:"explanation"`
}

// defaultCopilotAgent is the agent key used for workflow authoring. Matches the
// "Workflow 创建" agent seeded by cmd/workflow_agents_seed.go.
const defaultCopilotAgent = "workflow-chuanjian"

// resolveCopilotAgent picks the agent used for AI generation: the preferred
// authoring agent if present, otherwise the first available agent. Returns ""
// when none can be resolved.
func (e *Engine) resolveCopilotAgent(ctx context.Context) string {
	if e.runner == nil || e.runner.ListAgents == nil {
		return defaultCopilotAgent // best effort; AgentRun will surface a clear error if absent
	}
	available := e.runner.ListAgents(ctx)
	for _, id := range available {
		if id == defaultCopilotAgent {
			return id
		}
	}
	if len(available) > 0 {
		return available[0]
	}
	return ""
}

// GenerateGraph is goclaw's self-hosted equivalent of sim's Copilot: it asks an
// agent (via the injected NodeRunner) to translate a natural-language request
// into a serialized workflow Graph. Unlike sim — whose generation logic lives
// in a closed remote "mothership" — this runs entirely on goclaw's own agent
// runtime and model providers.
//
// The agent is given the closed graph schema + the catalog of node types and
// must return a JSON graph. The result is validated before returning.
func (e *Engine) GenerateGraph(ctx context.Context, req GenerateRequest) (*GenerateResult, error) {
	if e.runner == nil || !e.runner.HasAgent() {
		return nil, fmt.Errorf("workflow generate: agent runner not configured")
	}
	agentID := req.AgentID
	if agentID == "" {
		agentID = e.resolveCopilotAgent(ctx)
	}
	if agentID == "" {
		return nil, fmt.Errorf("workflow generate: no agent available")
	}

	sys := buildGeneratorSystemPrompt(req.Current)
	msg := strings.TrimSpace(req.Prompt)
	if msg == "" {
		return nil, fmt.Errorf("workflow generate: empty prompt")
	}

	res, err := e.runner.AgentRun(ctx, agentID, agent.RunRequest{
		Message:           msg,
		ExtraSystemPrompt: sys,
		RunKind:           "delegation",
	})
	if err != nil {
		return nil, fmt.Errorf("workflow generate: agent run failed: %w", err)
	}

	log.Printf("[workflow-generate] agent response len=%d content_preview=%.500s", len(res.Content), res.Content)

	g, explanation, err := parseGeneratedGraph(res.Content)
	if err != nil {
		return nil, fmt.Errorf("workflow generate: %w (agent_content_len=%d)", err, len(res.Content))
	}
	if g.Version == "" {
		g.Version = graph.Version
	}
	if err := g.Validate(); err != nil {
		return nil, fmt.Errorf("workflow generate: produced invalid graph: %w", err)
	}
	return &GenerateResult{Graph: *g, Explanation: explanation}, nil
}

// GenerateGraphJSON is the HTTP-facing adapter: it accepts the current graph as
// a loosely-typed map (so callers need not import the graph package), runs
// generation, and returns the result. A nil/empty current means a new flow.
func (e *Engine) GenerateGraphJSON(ctx context.Context, prompt, agentID string, current map[string]any) (*GenerateResult, error) {
	req := GenerateRequest{Prompt: prompt, AgentID: agentID}
	if len(current) > 0 {
		b, err := json.Marshal(current)
		if err != nil {
			return nil, fmt.Errorf("workflow generate: bad current graph: %w", err)
		}
		var g graph.Graph
		if err := json.Unmarshal(b, &g); err != nil {
			return nil, fmt.Errorf("workflow generate: bad current graph: %w", err)
		}
		req.Current = &g
	}
	return e.GenerateGraph(ctx, req)
}

// jsonBlockRe extracts a fenced ```json ... ``` block when present.
var jsonBlockRe = regexp.MustCompile("(?s)```(?:json)?\\s*(\\{.*\\})\\s*```")

// parseGeneratedGraph extracts a Graph from the agent's free-form response. It
// accepts either a fenced JSON block or a bare JSON object, and treats any text
// before the JSON as the explanation.
func parseGeneratedGraph(content string) (*graph.Graph, string, error) {
	raw := ""
	explanation := strings.TrimSpace(content)

	if m := jsonBlockRe.FindStringSubmatch(content); len(m) == 2 {
		raw = m[1]
		explanation = strings.TrimSpace(content[:strings.Index(content, m[0])])
	} else if i := strings.Index(content, "{"); i >= 0 {
		// fall back to the first balanced-looking object to end of string
		raw = content[i:]
		explanation = strings.TrimSpace(content[:i])
	}
	if raw == "" {
		return nil, "", fmt.Errorf("no JSON graph found in agent response")
	}

	var g graph.Graph
	if err := json.Unmarshal([]byte(raw), &g); err != nil {
		return nil, "", fmt.Errorf("graph JSON parse failed: %w", err)
	}
	return &g, explanation, nil
}

// buildGeneratorSystemPrompt builds the instruction describing the graph schema
// and node catalog. When current is non-nil, it is injected so the agent can do
// incremental edits.
func buildGeneratorSystemPrompt(current *graph.Graph) string {
	var b strings.Builder
	b.WriteString(`你是 goclaw 的工作流架构师。把用户的自然语言需求转换成一个工作流图(DAG),用 JSON 输出。

输出规则:
1. 先用一两句话简要说明你的设计,然后输出一个 ` + "```json" + ` 代码块,块内是完整的工作流图 JSON。
2. 图的结构(字段名必须完全一致):
{
  "version": "1.0",
  "blocks": [
    {
      "id": "唯一英文短id",
      "position": {"x": 数字, "y": 数字},
      "metadata": {"id": "节点类型", "name": "中文显示名"},
      "config": {"tool": "工具名(仅tool节点)", "params": {参数对象}},
      "enabled": true
    }
  ],
  "connections": [
    {"source": "源id", "target": "目标id", "sourceHandle": "source"}
  ],
  "loops": {},
  "parallels": {}
}
3. 可用节点类型(metadata.id 取值):
   - trigger    流程入口
   - agent      调用 AI 智能体;params: {"agent":"<agentId>", "prompt":"<给智能体的话>"}
   - tool       调用工具;config.tool 填工具名, params 填工具参数
   - condition  条件分支;params: {"conditions":[{"id":"c1","title":"if","value":"<JS表达式>"}]};出边 sourceHandle 用 "condition-c1"
   - router     路由分支;出边 sourceHandle 用 "router-<id>"
   - function   运行JS;params: {"code":"return {...}"}
   - knowledge  知识库检索;params: {"query":"<检索词>"}
   - response   流程最终输出;params 即输出内容
   - human-in-the-loop 人工介入暂停;params: {"prompt":"...","fields":["..."]}
4. 节点输入可引用上游输出: "<节点id.字段>"。例如 "<trigger.message>"。
5. 连线默认 sourceHandle 用 "source";错误分支用 "error"。
6. 布局: position.x 按执行先后从左到右递增(每级 +220),position.y 错开避免重叠。
7. 只输出设计说明 + 一个 JSON 块,不要输出多余内容。
`)
	if current != nil && len(current.Blocks) > 0 {
		cur, _ := json.Marshal(current)
		b.WriteString("\n当前画布上已有的工作流(请在此基础上按用户要求修改,保留未提及的部分):\n```json\n")
		b.Write(cur)
		b.WriteString("\n```\n")
	}
	return b.String()
}
