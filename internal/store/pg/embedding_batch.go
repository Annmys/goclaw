package pg

import (
	"strings"

	"github.com/nextlevelbuilder/goclaw/internal/store"
)

const defaultEmbeddingBackfillBatchSize = 50
const dashScopeEmbeddingBackfillBatchSize = 10

func embeddingBackfillBatchSize(provider store.EmbeddingProvider) int {
	if provider == nil {
		return defaultEmbeddingBackfillBatchSize
	}
	name := strings.ToLower(provider.Name())
	model := strings.ToLower(provider.Model())
	if strings.Contains(name, "dashscope") || strings.Contains(name, "bailian") || strings.Contains(model, "text-embedding-v4") || strings.Contains(model, "text-embedding-v3") {
		return dashScopeEmbeddingBackfillBatchSize
	}
	return defaultEmbeddingBackfillBatchSize
}
