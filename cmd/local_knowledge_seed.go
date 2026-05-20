package cmd

import (
	"context"
	"log/slog"

	"github.com/nextlevelbuilder/goclaw/internal/store"
)

func seedLocalKnowledgeSources(ctx context.Context, s store.LocalKnowledgeSourceStore) {
	ctx = store.WithTenantID(ctx, store.MasterTenantID)
	for _, seed := range store.DefaultLocalKnowledgeSourceSeeds() {
		if _, err := s.UpsertSource(ctx, seed); err != nil {
			slog.Warn("local_knowledge.seed_failed", "source_key", seed.SourceKey, "error", err)
		}
	}
}
