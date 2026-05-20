package cmd

import (
	"context"
	"log/slog"
	"time"

	httpapi "github.com/nextlevelbuilder/goclaw/internal/http"
	"github.com/nextlevelbuilder/goclaw/internal/store"
)

const defaultLocalKnowledgeSyncInterval = 6 * time.Hour

// localKnowledgeSyncWorker periodically refreshes registry status for enabled
// scheduled local knowledge sources. It only updates status metadata.
type localKnowledgeSyncWorker struct {
	syncer   *httpapi.LocalKnowledgeSyncer
	interval time.Duration
	ctx      context.Context
	cancel   context.CancelFunc
	stopCh   chan struct{}
	doneCh   chan struct{}
}

func newLocalKnowledgeSyncWorker(syncer *httpapi.LocalKnowledgeSyncer, interval time.Duration) *localKnowledgeSyncWorker {
	if interval <= 0 {
		interval = defaultLocalKnowledgeSyncInterval
	}
	return &localKnowledgeSyncWorker{
		syncer:   syncer,
		interval: interval,
		stopCh:   make(chan struct{}),
		doneCh:   make(chan struct{}),
	}
}

func (w *localKnowledgeSyncWorker) Start() {
	if w == nil || w.syncer == nil {
		return
	}
	w.ctx, w.cancel = context.WithCancel(context.Background())
	go w.loop()
	slog.Info("local knowledge sync worker started", "interval", w.interval)
}

func (w *localKnowledgeSyncWorker) Stop() {
	if w == nil {
		return
	}
	if w.cancel != nil {
		w.cancel()
	}
	close(w.stopCh)
	<-w.doneCh
	slog.Info("local knowledge sync worker stopped")
}

func (w *localKnowledgeSyncWorker) loop() {
	defer close(w.doneCh)
	w.syncOnce()

	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	for {
		select {
		case <-w.stopCh:
			return
		case <-ticker.C:
			w.syncOnce()
		}
	}
}

func (w *localKnowledgeSyncWorker) syncOnce() {
	ctx := store.WithTenantID(context.Background(), store.MasterTenantID)
	if w.ctx != nil {
		ctx = store.WithTenantID(w.ctx, store.MasterTenantID)
	}
	ctx, cancel := context.WithTimeout(ctx, minDuration(w.interval/2, 30*time.Minute))
	defer cancel()

	results, err := w.syncer.SyncScheduled(ctx)
	if err != nil {
		slog.Warn("local_knowledge.sync_worker.list_failed", "error", err)
		return
	}
	var okCount, failCount int
	for _, result := range results {
		if result.Error != "" {
			failCount++
			slog.Warn("local_knowledge.sync_worker.source_failed",
				"source", result.SourceKey,
				"name", result.Name,
				"error", result.Error,
			)
			continue
		}
		okCount++
	}
	slog.Info("local_knowledge.sync_worker.complete", "success", okCount, "failed", failCount)
}

func minDuration(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}
