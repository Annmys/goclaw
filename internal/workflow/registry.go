package workflow

import (
	"slices"
	"sort"
	"strings"
)

type Registry struct {
	definitions []Definition
}

func NewRegistry(definitions []Definition) *Registry {
	cp := make([]Definition, 0, len(definitions))
	for _, def := range definitions {
		if def.Active {
			cp = append(cp, def)
		}
	}
	return &Registry{definitions: cp}
}

func NewDefaultRegistry() *Registry {
	return NewRegistry([]Definition{
		{
			ID:          "packing_list_v1",
			Version:     1,
			Name:        "包装清单流程",
			Description: "根据用户清单生成包装清单，缺字段时先联动补齐，再输出流程专属结果。",
			Domain:      "packing",
			Active:      true,
			MatchRules: []MatchRule{
				{Key: "file_type", Label: "Excel 文件", FileTypes: []string{"xlsx", "xls", "csv"}},
				{Key: "keyword", Label: "包装清单关键词", Keywords: []string{"包装清单", "packing list", "箱单", "装箱单", "预估箱单"}},
			},
			Nodes: []NodeDefinition{
				{ID: "trigger-1", Type: NodeTrigger, TypeLabel: "触发节点", InstanceNo: 1, Name: "接收用户文件"},
				{ID: "detect-1", Type: NodeDetect, TypeLabel: "识别节点", InstanceNo: 1, Name: "识别业务类型"},
				{ID: "parse-1", Type: NodeParse, TypeLabel: "解析节点", InstanceNo: 1, Name: "解析清单原始数据"},
				{ID: "choice-1", Type: NodeChoice, TypeLabel: "选择节点", InstanceNo: 1, Name: "检查缺失字段", DependsOn: []string{"parse-1"}, MissingFields: packingListMissingFields()},
				{ID: "fill-1", Type: NodeFill, TypeLabel: "填充节点", InstanceNo: 1, Name: "填入包装清单字段", MissingFields: packingListMissingFields()},
				{ID: "validate-1", Type: NodeValidate, TypeLabel: "校验节点", InstanceNo: 1, Name: "校验必填字段", DependsOn: []string{"fill-1"}},
				{ID: "action-1", Type: NodeAction, TypeLabel: "执行节点", InstanceNo: 1, Name: "生成包装清单结果", DependsOn: []string{"validate-1"}},
				{ID: "transform-1", Type: NodeTransform, TypeLabel: "转换节点", InstanceNo: 1, Name: "整理输出结构", DependsOn: []string{"action-1"}},
				{ID: "output-1", Type: NodeOutput, TypeLabel: "输出节点", InstanceNo: 1, Name: "生成包装清单结果", DependsOn: []string{"transform-1"}},
				{ID: "persist-1", Type: NodePersist, TypeLabel: "持久化节点", InstanceNo: 1, Name: "保存执行记录", DependsOn: []string{"output-1"}},
				{ID: "feedback-1", Type: NodeFeedback, TypeLabel: "反馈节点", InstanceNo: 1, Name: "接收用户反馈", DependsOn: []string{"persist-1"}},
			},
			Output: OutputDefinition{Adapter: "packing_list", OutputSchema: map[string]any{
				"result_file": "string",
				"rows":        "array",
				"warnings":    "array",
			}},
		},
		{
			ID:          "label_generation_v1",
			Version:     1,
			Name:        "标签生成流程",
			Description: "根据产品和包装数据生成标签输出，支持标签预览和打印数据输出。",
			Domain:      "label",
			Active:      true,
			MatchRules: []MatchRule{
				{Key: "file_type", Label: "Excel 文件", FileTypes: []string{"xlsx", "xls", "csv"}},
				{Key: "keyword", Label: "标签关键词", Keywords: []string{"标签", "label", "打印标签", "条码"}},
			},
			Nodes: []NodeDefinition{
				{ID: "trigger-1", Type: NodeTrigger, TypeLabel: "触发节点", InstanceNo: 1, Name: "接收用户文件"},
				{ID: "detect-1", Type: NodeDetect, TypeLabel: "识别节点", InstanceNo: 1, Name: "识别标签任务"},
				{ID: "parse-1", Type: NodeParse, TypeLabel: "解析节点", InstanceNo: 1, Name: "解析标签数据源"},
				{ID: "choice-1", Type: NodeChoice, TypeLabel: "选择节点", InstanceNo: 1, Name: "检查标签模板", DependsOn: []string{"parse-1"}, MissingFields: labelGenerationMissingFields()},
				{ID: "fill-1", Type: NodeFill, TypeLabel: "填充节点", InstanceNo: 1, Name: "填入标签字段", MissingFields: labelGenerationMissingFields()},
				{ID: "validate-1", Type: NodeValidate, TypeLabel: "校验节点", InstanceNo: 1, Name: "校验标签字段", DependsOn: []string{"fill-1"}},
				{ID: "transform-1", Type: NodeTransform, TypeLabel: "转换节点", InstanceNo: 1, Name: "转换为标签数据", DependsOn: []string{"validate-1"}},
				{ID: "output-1", Type: NodeOutput, TypeLabel: "输出节点", InstanceNo: 1, Name: "生成标签结果", DependsOn: []string{"transform-1"}},
				{ID: "persist-1", Type: NodePersist, TypeLabel: "持久化节点", InstanceNo: 1, Name: "保存执行记录", DependsOn: []string{"output-1"}},
				{ID: "feedback-1", Type: NodeFeedback, TypeLabel: "反馈节点", InstanceNo: 1, Name: "接收用户反馈", DependsOn: []string{"persist-1"}},
			},
			Output: OutputDefinition{Adapter: "label_generation", OutputSchema: map[string]any{
				"preview":       "array",
				"print_payload": "object",
				"result_file":   "string",
			}},
		},
		workflowCreatorDefinition(),
		workflowDataProcessorDefinition(),
		workflowMaintainerDefinition(),
	})
}

func packingListMissingFields() []MissingField {
	return []MissingField{
		{
			Key:      "carton_rule",
			Label:    "装箱规则",
			Kind:     "select",
			Required: true,
			Options:  []string{"按产品", "按订单", "按客户规则"},
			Details: []OptionDetail{
				{Value: "按产品", Title: "按产品", Description: "按产品维度优先计算箱规", Highlights: []string{"适合型号稳定", "优先保持同产品分组"}},
				{Value: "按订单", Title: "按订单", Description: "按整单维度优先出结果", Highlights: []string{"适合订单粒度清晰", "减少拆分"}},
				{Value: "按客户规则", Title: "按客户规则", Description: "按客户特殊规则执行", Highlights: []string{"适合客户有固定装箱习惯", "优先遵循客户约定"}},
			},
		},
		{
			Key:      "weight_source",
			Label:    "重量来源",
			Kind:     "select",
			Required: true,
			Options:  []string{"用户清单", "重量表", "人工补充"},
			Details: []OptionDetail{
				{Value: "用户清单", Title: "用户清单", Description: "直接使用用户文件里已有重量", Highlights: []string{"最快", "但依赖原文件完整"}},
				{Value: "重量表", Title: "重量表", Description: "从重量表取标准重量", Highlights: []string{"准确性更高", "适合有维护表"}},
				{Value: "人工补充", Title: "人工补充", Description: "由用户手动补齐缺失重量", Highlights: []string{"用于缺字段补齐", "需要用户确认"}},
			},
		},
	}
}

func labelGenerationMissingFields() []MissingField {
	return []MissingField{
		{
			Key:      "label_template",
			Label:    "标签模板",
			Kind:     "select",
			Required: true,
			Options:  []string{"产品标签", "箱唛标签", "条码标签"},
			Details: []OptionDetail{
				{Value: "产品标签", Title: "产品标签", Description: "用于产品信息展示", Highlights: []string{"关注型号", "适合单件信息"}},
				{Value: "箱唛标签", Title: "箱唛标签", Description: "用于外箱标识", Highlights: []string{"关注箱数", "适合外箱打印"}},
				{Value: "条码标签", Title: "条码标签", Description: "用于扫码流转", Highlights: []string{"关注条码字段", "适合仓储环节"}},
			},
		},
		{
			Key:      "print_mode",
			Label:    "打印模式",
			Kind:     "select",
			Required: true,
			Options:  []string{"预览", "直接打印", "导出文件"},
			Details: []OptionDetail{
				{Value: "预览", Title: "预览", Description: "先查看效果再打印", Highlights: []string{"最稳妥", "适合确认样式"}},
				{Value: "直接打印", Title: "直接打印", Description: "直接交给打印设备", Highlights: []string{"效率最高", "需确认模板准确"}},
				{Value: "导出文件", Title: "导出文件", Description: "输出成可下载文件", Highlights: []string{"便于复核", "适合后续处理"}},
			},
		},
	}
}

func workflowCreatorDefinition() Definition {
	return Definition{
		ID:          "workflow_chuanjian",
		Version:     1,
		Name:        "workflow 创建官",
		Description: "对话中接收完整创建信息，负责申请和创建正式 workflow。",
		Domain:      "workflow-admin",
		Active:      true,
		MatchRules: []MatchRule{
			{Key: "keyword", Label: "创建关键词", Keywords: []string{"创建workflow", "申请流程", "新建流程", "workflow创建"}},
		},
		Nodes: []NodeDefinition{
			{ID: "trigger-1", Type: NodeTrigger, TypeLabel: "触发节点", InstanceNo: 1, Name: "接收创建申请"},
			{ID: "parse-1", Type: NodeParse, TypeLabel: "解析节点", InstanceNo: 1, Name: "整理创建信息"},
			{ID: "choice-1", Type: NodeChoice, TypeLabel: "选择节点", InstanceNo: 1, Name: "确认流程边界", MissingFields: []MissingField{
				{Key: "workflow_name", Label: "流程名称", Kind: "text", Required: true},
				{Key: "workflow_domain", Label: "流程域", Kind: "select", Required: true, Options: []string{"packing", "label", "workflow-admin"}},
			}},
			{ID: "action-1", Type: NodeAction, TypeLabel: "执行节点", InstanceNo: 1, Name: "提交流程创建"},
			{ID: "output-1", Type: NodeOutput, TypeLabel: "输出节点", InstanceNo: 1, Name: "返回创建结果"},
		},
		Output:   OutputDefinition{Adapter: "workflow_admin", OutputSchema: map[string]any{"workflow_name": "string", "workflow_domain": "string", "status": "string"}},
		Metadata: map[string]string{"workflow_role": "workflow_chuanjian"},
	}
}

func workflowDataProcessorDefinition() Definition {
	return Definition{
		ID:          "workflow_shujuchuli",
		Version:     1,
		Name:        "workflow 数据处理官",
		Description: "只负责执行中数据识别、归一、映射和字段填充，不改流程结构。",
		Domain:      "workflow-exec",
		Active:      true,
		MatchRules: []MatchRule{
			{Key: "keyword", Label: "数据处理关键词", Keywords: []string{"workflow数据处理", "数据填入", "字段映射", "结构化填充"}},
		},
		Nodes: []NodeDefinition{
			{ID: "trigger-1", Type: NodeTrigger, TypeLabel: "触发节点", InstanceNo: 1, Name: "接收当前步骤数据"},
			{ID: "parse-1", Type: NodeParse, TypeLabel: "解析节点", InstanceNo: 1, Name: "识别输入数据"},
			{ID: "fill-1", Type: NodeFill, TypeLabel: "填充节点", InstanceNo: 1, Name: "填入当前步骤字段"},
			{ID: "validate-1", Type: NodeValidate, TypeLabel: "校验节点", InstanceNo: 1, Name: "校验字段完整性"},
			{ID: "output-1", Type: NodeOutput, TypeLabel: "输出节点", InstanceNo: 1, Name: "返回结构化数据"},
		},
		Output:   OutputDefinition{Adapter: "workflow_data_fill", OutputSchema: map[string]any{"filled_fields": "object", "missing_fields": "array", "warnings": "array"}},
		Metadata: map[string]string{"workflow_role": "workflow_shujuchuli"},
	}
}

func workflowMaintainerDefinition() Definition {
	return Definition{
		ID:          "workflow_liuchengweihu",
		Version:     1,
		Name:        "workflow 流程维护官",
		Description: "负责用户反馈后的流程修缮、版本升级建议和问题归类。",
		Domain:      "workflow-maintenance",
		Active:      true,
		MatchRules: []MatchRule{
			{Key: "keyword", Label: "维护关键词", Keywords: []string{"流程维护", "修缮流程", "反馈问题", "workflow维护"}},
		},
		Nodes: []NodeDefinition{
			{ID: "trigger-1", Type: NodeTrigger, TypeLabel: "触发节点", InstanceNo: 1, Name: "接收用户反馈"},
			{ID: "parse-1", Type: NodeParse, TypeLabel: "解析节点", InstanceNo: 1, Name: "整理问题描述"},
			{ID: "choice-1", Type: NodeChoice, TypeLabel: "选择节点", InstanceNo: 1, Name: "判断修缮方向", MissingFields: []MissingField{
				{Key: "issue_type", Label: "问题类型", Kind: "select", Required: true, Options: []string{"数据错误", "格式错误", "结果错误", "流程缺陷"}},
			}},
			{ID: "action-1", Type: NodeAction, TypeLabel: "执行节点", InstanceNo: 1, Name: "生成修缮建议"},
			{ID: "output-1", Type: NodeOutput, TypeLabel: "输出节点", InstanceNo: 1, Name: "返回维护结论"},
		},
		Output:   OutputDefinition{Adapter: "workflow_maintenance", OutputSchema: map[string]any{"issue_type": "string", "repair_scope": "string", "status": "string"}},
		Metadata: map[string]string{"workflow_role": "workflow_liuchengweihu"},
	}
}

func (r *Registry) List() []Definition {
	out := make([]Definition, len(r.definitions))
	copy(out, r.definitions)
	return out
}

func (r *Registry) Get(id string) (Definition, bool) {
	for _, def := range r.definitions {
		if def.ID == id {
			return def, true
		}
	}
	return Definition{}, false
}

func (r *Registry) Match(req MatchRequest) MatchResult {
	if strings.TrimSpace(req.ExplicitID) != "" {
		if def, ok := r.Get(req.ExplicitID); ok {
			return MatchResult{
				Matched: true,
				Candidates: []MatchCandidate{{
					WorkflowID:      def.ID,
					WorkflowVersion: def.Version,
					Name:            def.Name,
					Score:           100,
					Reasons:         []string{"用户显式选择 workflow"},
				}},
			}
		}
		return MatchResult{Matched: false, Message: "explicit workflow not found"}
	}

	intent := strings.ToLower(req.Intent)
	fileType := strings.ToLower(strings.TrimPrefix(req.FileType, "."))
	fileName := strings.ToLower(req.FileName)
	candidates := make([]MatchCandidate, 0)

	for _, def := range r.definitions {
		score := 0
		reasons := make([]string, 0)
		for _, rule := range def.MatchRules {
			if fileType != "" && slices.Contains(rule.FileTypes, fileType) {
				score += 20
				reasons = append(reasons, "文件类型命中: "+fileType)
			}
			for _, keyword := range rule.Keywords {
				k := strings.ToLower(keyword)
				if k != "" && (strings.Contains(intent, k) || strings.Contains(fileName, k)) {
					score += 40
					reasons = append(reasons, "关键词命中: "+keyword)
				}
			}
		}
		if score > 0 {
			candidates = append(candidates, MatchCandidate{
				WorkflowID:      def.ID,
				WorkflowVersion: def.Version,
				Name:            def.Name,
				Score:           score,
				Reasons:         reasons,
			})
		}
	}

	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].Score > candidates[j].Score
	})

	if len(candidates) == 0 {
		return MatchResult{Matched: false, Message: "没有匹配到可执行 workflow"}
	}
	return MatchResult{
		Matched:     true,
		NeedsChoice: len(candidates) > 1 && candidates[0].Score == candidates[1].Score,
		Candidates:  candidates,
	}
}
