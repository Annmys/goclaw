package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/nextlevelbuilder/goclaw/internal/store"
)

// pgAutoInjector implements AutoInjector backed by EpisodicStore + FTS search.
type pgAutoInjector struct {
	episodicStore store.EpisodicStore
	kgStore       store.KnowledgeGraphStore  // nil = KG retrieval disabled
	metricsStore  store.EvolutionMetricsStore // nil = metrics disabled
}

// NewAutoInjector creates an AutoInjector backed by episodic store search.
// If a KnowledgeGraphStore is provided, also retrieves relevant KG entities
// (rules, corrections, facts) for cross-session behavioral recall.
func NewAutoInjector(es store.EpisodicStore, ms store.EvolutionMetricsStore, kgs ...store.KnowledgeGraphStore) AutoInjector {
	var kg store.KnowledgeGraphStore
	if len(kgs) > 0 && kgs[0] != nil {
		kg = kgs[0]
	}
	return &pgAutoInjector{episodicStore: es, metricsStore: ms, kgStore: kg}
}

// Inject searches episodic memory for relevant L0 abstracts and formats a prompt section.
func (a *pgAutoInjector) Inject(ctx context.Context, params InjectParams) (*InjectResult, error) {
	if a.episodicStore == nil {
		return &InjectResult{}, nil
	}
	if isTrivialMessage(params.UserMessage) {
		return &InjectResult{}, nil
	}

	maxEntries := params.MaxEntries
	if maxEntries <= 0 {
		maxEntries = 5
	}
	threshold := params.Threshold
	if threshold <= 0 {
		threshold = 0.3
	}

	// Phase 9: context-aware recall. When the caller supplied RecentContext,
	// build a richer search query that captures conversational intent. Without
	// this, vector search on "what's my favorite?" misses memories about the
	// topic under discussion. With it, the query embedding captures the
	// follow-up semantics and returns materially better matches.
	searchQuery := buildRecallQuery(params.UserMessage, params.RecentContext)

	// Search with FTS bias (faster than vector for auto-inject)
	results, err := a.episodicStore.Search(ctx, searchQuery, params.AgentID, params.UserID,
		store.EpisodicSearchOptions{
			MaxResults:   maxEntries * 2, // fetch more, filter by threshold
			MinScore:     threshold,
			VectorWeight: 0.3,
			TextWeight:   0.7,
		})
	if err != nil {
		return nil, fmt.Errorf("auto-inject search: %w", err)
	}
	if len(results) == 0 {
		return &InjectResult{}, nil
	}

	// Build prompt section from L0 abstracts
	var sb strings.Builder
	sb.WriteString("## Memory Context\n\nRelevant memories from past sessions (use memory_search for details):\n")

	injected := 0
	var topScore float64
	for _, r := range results {
		if injected >= maxEntries {
			break
		}
		if r.L0Abstract == "" {
			continue
		}
		sb.WriteString("- ")
		sb.WriteString(r.L0Abstract)
		sb.WriteString("\n")
		injected++
		if r.Score > topScore {
			topScore = r.Score
		}
	}

	if injected == 0 {
		return &InjectResult{MatchCount: len(results)}, nil
	}

	// KG retrieval: search knowledge graph for relevant rules/facts.
	// These persist across sessions and carry behavioral corrections the user taught.
	if a.kgStore != nil && params.AgentID != "" {
		kgEntities, kgErr := a.kgStore.SearchEntities(ctx, params.AgentID, params.UserID, searchQuery, 3)
		if kgErr == nil && len(kgEntities) > 0 {
			sb.WriteString("\n## Knowledge Rules\n\nRelevant rules and facts from knowledge graph:\n")
			for _, e := range kgEntities {
				desc := e.Description
				if desc == "" {
					desc = e.Name
				}
				sb.WriteString("- ")
				if e.EntityType != "" {
					sb.WriteString("[")
					sb.WriteString(e.EntityType)
					sb.WriteString("] ")
				}
				sb.WriteString(desc)
				sb.WriteString("\n")
				injected++
			}
		}
	}

	result := &InjectResult{
		Section:    sb.String(),
		MatchCount: len(results),
		Injected:   injected,
		TopScore:   topScore,
	}

	// Record retrieval metric non-blocking (best-effort).
	a.recordRetrievalMetric(params, result)

	return result, nil
}

// recordRetrievalMetric records an auto-inject retrieval metric in a background goroutine.
func (a *pgAutoInjector) recordRetrievalMetric(params InjectParams, result *InjectResult) {
	if a.metricsStore == nil || params.TenantID == "" {
		return
	}
	tenantID, err := uuid.Parse(params.TenantID)
	if err != nil {
		return
	}
	agentID, err := uuid.Parse(params.AgentID)
	if err != nil {
		return
	}
	go func() {
		bgCtx, cancel := context.WithTimeout(store.WithTenantID(context.Background(), tenantID), 5*time.Second)
		defer cancel()
		value, _ := json.Marshal(map[string]any{
			"result_count":  result.MatchCount,
			"injected":      result.Injected,
			"top_score":     result.TopScore,
			"used_in_reply": result.Injected > 0,
		})
		if err := a.metricsStore.RecordMetric(bgCtx, store.EvolutionMetric{
			ID:         uuid.New(),
			TenantID:   tenantID,
			AgentID:    agentID,
			MetricType: store.MetricRetrieval,
			MetricKey:  "auto_inject",
			Value:      value,
		}); err != nil {
			slog.Debug("evolution.metric.auto_inject_failed", "error", err)
		}
	}()
}
