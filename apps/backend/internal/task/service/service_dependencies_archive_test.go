package service

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

func TestArchivedPredecessorOutcomeThroughGateAndView(t *testing.T) {
	for _, testCase := range []struct {
		name        string
		state       v1.TaskState
		wantStatus  string
		wantReason  string
		wantBlocked bool
	}{
		{
			name:        "completed predecessor resolves",
			state:       v1.TaskStateCompleted,
			wantStatus:  DependencyResolved,
			wantBlocked: false,
		},
		{
			name:        "failed predecessor fails",
			state:       v1.TaskStateFailed,
			wantStatus:  DependencyFailed,
			wantReason:  BlockedReasonFailed,
			wantBlocked: true,
		},
		{
			name:        "cancelled predecessor fails",
			state:       v1.TaskStateCancelled,
			wantStatus:  DependencyFailed,
			wantReason:  BlockedReasonFailed,
			wantBlocked: true,
		},
		{
			name:        "unfinished predecessor stays pending",
			state:       v1.TaskStateInProgress,
			wantStatus:  DependencyPending,
			wantReason:  BlockedReasonPending,
			wantBlocked: true,
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			ctx := context.Background()
			svc, repo := setupOfficeTest(t)
			svc.SetBlockerRepository(&mockBlockerRepo{})
			predecessor := mustSeedTask(t, svc, "Predecessor")
			dependent := mustSeedTask(t, svc, "Dependent")
			if err := svc.AddDependency(ctx, dependent.ID, predecessor.ID); err != nil {
				t.Fatalf("add dependency: %v", err)
			}
			setDependencyState(t, svc, predecessor.ID, testCase.state)
			archivedAt := time.Now().UTC()
			if _, err := repo.DB().ExecContext(ctx,
				`UPDATE tasks SET archived_at = ? WHERE id = ?`, archivedAt, predecessor.ID); err != nil {
				t.Fatalf("archive predecessor: %v", err)
			}

			blocked, reason, err := svc.DependencyGate(ctx, dependent.ID)
			if err != nil || blocked != testCase.wantBlocked || reason != testCase.wantReason {
				t.Errorf("DependencyGate = blocked:%t reason:%q err:%v; want blocked:%t reason:%q",
					blocked, reason, err, testCase.wantBlocked, testCase.wantReason)
			}

			views := svc.BuildDependencyViews(ctx, []*models.Task{dependent})
			view, ok := views[dependent.ID]
			if !ok {
				t.Fatal("BuildDependencyViews omitted the dependent")
			}
			if len(view.DependsOn) != 1 {
				t.Fatalf("depends_on length = %d, want 1", len(view.DependsOn))
			}
			if view.DependsOn[0].Status != testCase.wantStatus {
				t.Errorf("archived predecessor status = %q, want %q",
					view.DependsOn[0].Status, testCase.wantStatus)
			}
			if view.Blocked != testCase.wantBlocked || view.BlockedReason != testCase.wantReason {
				t.Errorf("view blocked:%t reason:%q; want blocked:%t reason:%q",
					view.Blocked, view.BlockedReason, testCase.wantBlocked, testCase.wantReason)
			}
		})
	}
}
