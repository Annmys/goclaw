package store

import "encoding/json"

// DefaultLocalKnowledgeSourceSeeds are deployment-aware local sources used by
// the first registry stage. They do not scan or import data by themselves.
func DefaultLocalKnowledgeSourceSeeds() []LocalKnowledgeSourceSeed {
	meta, _ := json.Marshal(map[string]string{
		"seed":  "default",
		"owner": "goclaw",
	})
	return []LocalKnowledgeSourceSeed{
		{
			SourceKey:     "flow-orders",
			Name:          "包装流转单",
			Description:   "包装流转单源文件和转换后的订单索引数据。",
			PathWindows:   `D:\数据\包装流转单`,
			PathContainer: "/mnt/target/flow-orders",
			TenantScope:   "system",
			SyncMode:      "scheduled",
			IndexTarget:   "tool_cache",
			Enabled:       true,
			Metadata:      meta,
		},
		{
			SourceKey:     "package-weights",
			Name:          "产品包装重量表",
			Description:   "产品包装重量表 Excel 与 SQLite 查询缓存。",
			PathWindows:   `D:\数据\产品包装重量表`,
			PathContainer: "/mnt/source/product-package-weights",
			TenantScope:   "system",
			SyncMode:      "scheduled",
			IndexTarget:   "tool_cache",
			Enabled:       true,
			Metadata:      meta,
		},
		{
			SourceKey:     "packaging-data",
			Name:          "包装资料",
			Description:   "包装计算使用的新包装资料数据源。",
			PathWindows:   `D:\数据\包装资料`,
			PathContainer: "/mnt/package-materials",
			TenantScope:   "system",
			SyncMode:      "scheduled",
			IndexTarget:   "tool_cache",
			Enabled:       true,
			Metadata:      meta,
		},
		{
			SourceKey:     "label-templates",
			Name:          "标签模板",
			Description:   "BarTender 标签模板目录。",
			PathWindows:   `D:\数据\标签模板`,
			PathContainer: "/mnt/label-templates",
			TenantScope:   "system",
			SyncMode:      "manual",
			IndexTarget:   "registry",
			Enabled:       true,
			Metadata:      meta,
		},
		{
			SourceKey:     "operation-log",
			Name:          "GoClaw 操作记录",
			Description:   "GoClaw 项目续接记录、规则和本地备份资料。",
			PathWindows:   `D:\goclaw操作记录`,
			PathContainer: "/mnt/operation-log",
			TenantScope:   "system",
			SyncMode:      "manual",
			IndexTarget:   "registry",
			Enabled:       true,
			Metadata:      meta,
		},
	}
}
