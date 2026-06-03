package workflow

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/nextlevelbuilder/goclaw/internal/store"
)

func TestEngineListRunsFiltersByTenantAndUser(t *testing.T) {
	registry := NewDefaultRegistry()
	engine := NewEngine(registry)

	tenantA := uuid.New()
	tenantB := uuid.New()

	engine.runs["run-a"] = Run{ID: "run-a", TenantID: tenantA.String(), UserID: "user-a", CreatedAt: time.Unix(20, 0)}
	engine.runs["run-b"] = Run{ID: "run-b", TenantID: tenantA.String(), UserID: "user-b", CreatedAt: time.Unix(30, 0)}
	engine.runs["run-c"] = Run{ID: "run-c", TenantID: tenantB.String(), UserID: "user-a", CreatedAt: time.Unix(40, 0)}

	ctx := store.WithTenantID(context.Background(), tenantA)
	ctx = store.WithUserID(ctx, "user-a")

	runs := engine.ListRuns(ctx)
	if len(runs) != 1 {
		t.Fatalf("expected 1 run, got %d", len(runs))
	}
	if runs[0].ID != "run-a" {
		t.Fatalf("expected run-a, got %s", runs[0].ID)
	}
}

func TestEngineGetRunFiltersByTenantAndUser(t *testing.T) {
	registry := NewDefaultRegistry()
	engine := NewEngine(registry)
	tenant := uuid.New()

	engine.runs["run-a"] = Run{ID: "run-a", TenantID: tenant.String(), UserID: "user-a", CreatedAt: time.Now()}

	ctx := store.WithTenantID(context.Background(), tenant)
	ctx = store.WithUserID(ctx, "user-b")

	if _, err := engine.GetRun(ctx, "run-a"); err == nil {
		t.Fatal("expected get run to be blocked by user mismatch")
	}
}

func TestEngineAppendMessageFiltersByTenantAndUser(t *testing.T) {
	registry := NewDefaultRegistry()
	engine := NewEngine(registry)
	tenant := uuid.New()
	ctx := store.WithTenantID(context.Background(), tenant)
	ctx = store.WithUserID(ctx, "user-a")

	msg := engine.AppendMessage(ctx, AppendMessageRequest{Role: MessageRoleUser, Content: "在么", Kind: MessageKindChat})
	if msg.TenantID != tenant.String() {
		t.Fatalf("expected tenant %s, got %s", tenant, msg.TenantID)
	}
	if msg.UserID != "user-a" {
		t.Fatalf("expected user-a, got %s", msg.UserID)
	}

	sameTenantOtherUser := store.WithTenantID(context.Background(), tenant)
	sameTenantOtherUser = store.WithUserID(sameTenantOtherUser, "user-b")
	if got := engine.ListMessages(sameTenantOtherUser); len(got) != 0 {
		t.Fatalf("expected no messages for same tenant other user, got %d", len(got))
	}

	otherTenant := store.WithTenantID(context.Background(), uuid.New())
	otherTenant = store.WithUserID(otherTenant, "user-a")
	if got := engine.ListMessages(otherTenant); len(got) != 0 {
		t.Fatalf("expected no messages for other tenant, got %d", len(got))
	}

	if got := engine.ListMessages(ctx); len(got) != 1 || got[0].ID != msg.ID {
		t.Fatalf("expected original user to see one message %s, got %#v", msg.ID, got)
	}
}

func TestEngineListRunsDoesNotLeakWhenUserIDMissing(t *testing.T) {
	registry := NewDefaultRegistry()
	engine := NewEngine(registry)
	tenant := uuid.New()

	engine.runs["run-a"] = Run{ID: "run-a", TenantID: tenant.String(), UserID: "user-a", CreatedAt: time.Unix(20, 0)}

	ctx := store.WithTenantID(context.Background(), tenant)
	if got := engine.ListRuns(ctx); len(got) != 0 {
		t.Fatalf("expected no scoped runs without user id, got %d", len(got))
	}
}


