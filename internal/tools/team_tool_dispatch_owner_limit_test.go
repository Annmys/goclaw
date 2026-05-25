package tools

import (
	"testing"
	"time"

	"github.com/nextlevelbuilder/goclaw/internal/store"
)

func TestDispatchUnblockedTasksSkipsBusyOwner(t *testing.T) {
	mb, _, _, memberID, ctx := newTestTeamSetup()
	team := mb.team
	if team == nil {
		t.Fatal("expected team fixture")
	}
	manager := NewTeamToolManager(mb.taskStore, nil, nil, "")

	busyTask := &store.TeamTaskData{
		TeamID:        team.ID,
		TenantID:      testTenantID,
		Subject:       "busy task",
		Description:   "already in progress",
		Status:        store.TeamTaskStatusInProgress,
		OwnerAgentID:  &memberID,
		Priority:      10,
		Channel:       ChannelDashboard,
		ChatID:        team.ID.String(),
		LockedAt:      ptrTime(time.Now()),
		LockExpiresAt: ptrTime(time.Now().Add(10 * time.Minute)),
	}
	if err := mb.taskStore.CreateTask(ctx, busyTask); err != nil {
		t.Fatalf("create busy task: %v", err)
	}

	pendingTask := &store.TeamTaskData{
		TeamID:       team.ID,
		TenantID:     testTenantID,
		Subject:      "pending task",
		Description:  "should stay pending because owner is busy",
		Status:       store.TeamTaskStatusPending,
		OwnerAgentID: &memberID,
		Priority:     9,
		Channel:      ChannelDashboard,
		ChatID:       team.ID.String(),
	}
	if err := mb.taskStore.CreateTask(ctx, pendingTask); err != nil {
		t.Fatalf("create pending task: %v", err)
	}

	mb.taskStore.tasks[pendingTask.ID].OwnerAgentID = &memberID
	mb.taskStore.tasks[pendingTask.ID].Status = store.TeamTaskStatusPending
	mb.taskStore.tasks[busyTask.ID].OwnerAgentID = &memberID
	mb.taskStore.tasks[busyTask.ID].Status = store.TeamTaskStatusInProgress

	manager.DispatchUnblockedTasks(ctx, team.ID)

	got, err := mb.taskStore.GetTask(ctx, pendingTask.ID)
	if err != nil {
		t.Fatalf("get pending task: %v", err)
	}
	if got.Status != store.TeamTaskStatusPending {
		t.Fatalf("pending task got dispatched despite busy owner, status=%s", got.Status)
	}
}

func ptrTime(t time.Time) *time.Time { return &t }
