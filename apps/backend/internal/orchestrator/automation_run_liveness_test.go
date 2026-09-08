package orchestrator

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"testing"
	"time"

	runtimeapi "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
)

type automationRunLivenessTurnService struct {
	TurnService
	err error
}

func (s automationRunLivenessTurnService) GetActiveTurn(context.Context, string) (*models.Turn, error) {
	return nil, s.err
}

// @covers AC-OFFICE-AUTOMATION-CONTINUITY-003.4
func TestAutomationRunLiveNormalizesGoneExecutionErrors(t *testing.T) {
	transientErr := errors.New("database temporarily unavailable")
	tests := []struct {
		name    string
		err     error
		wantErr error
	}{
		{name: "runtime not found", err: fmt.Errorf("inspect turn: %w", runtimeapi.ErrNotFound)},
		{name: "executor execution not found", err: fmt.Errorf("inspect turn: %w", executor.ErrExecutionNotFound)},
		{name: "lifecycle execution not found", err: fmt.Errorf("inspect turn: %w", lifecycle.ErrExecutionNotFound)},
		{name: "SQL row not found", err: fmt.Errorf("inspect turn: %w", sql.ErrNoRows)},
		{name: "transient error", err: transientErr, wantErr: transientErr},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := setupTestRepo(t)
			ctx := context.Background()
			now := time.Now().UTC()
			if err := repo.CreateTask(ctx, &models.Task{
				ID: "task", WorkspaceID: "workspace", Origin: models.TaskOriginAutomationRun,
				CreatedAt: now, UpdatedAt: now,
			}); err != nil {
				t.Fatal(err)
			}
			if err := repo.CreateTaskSession(ctx, &models.TaskSession{
				ID: "session", TaskID: "task", State: models.TaskSessionStateWaitingForInput,
				StartedAt: now, UpdatedAt: now,
			}); err != nil {
				t.Fatal(err)
			}
			svc := &Service{
				repo:        repo,
				turnService: automationRunLivenessTurnService{err: tt.err},
			}

			live, err := svc.AutomationRunLive(ctx, "task", "session", "turn")

			if live {
				t.Fatal("AutomationRunLive returned live for a failed liveness inspection")
			}
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("AutomationRunLive error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

// @covers AC-OFFICE-AUTOMATION-CONTINUITY-003.4
func TestAutomationRunLiveNormalizesMissingTaskAndSession(t *testing.T) {
	tests := []struct {
		name          string
		deleteTask    bool
		deleteSession bool
	}{
		{name: "missing task", deleteTask: true},
		{name: "deleted session", deleteSession: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := setupTestRepo(t)
			ctx := context.Background()
			now := time.Now().UTC()
			task := &models.Task{
				ID: "task", WorkspaceID: "workspace", Origin: models.TaskOriginAutomationRun,
				CreatedAt: now, UpdatedAt: now,
			}
			if err := repo.CreateTask(ctx, task); err != nil {
				t.Fatal(err)
			}
			session := &models.TaskSession{
				ID: "session", TaskID: task.ID, State: models.TaskSessionStateWaitingForInput,
				StartedAt: now, UpdatedAt: now,
			}
			if err := repo.CreateTaskSession(ctx, session); err != nil {
				t.Fatal(err)
			}
			if tt.deleteTask {
				if err := repo.DeleteTask(ctx, task.ID); err != nil {
					t.Fatal(err)
				}
			}
			if tt.deleteSession {
				if err := repo.DeleteTaskSession(ctx, session.ID); err != nil {
					t.Fatal(err)
				}
			}

			svc := &Service{repo: repo}
			live, err := svc.AutomationRunLive(ctx, task.ID, session.ID, "turn")

			if live {
				t.Fatal("AutomationRunLive returned live for a missing task or session")
			}
			if err != nil {
				t.Fatalf("AutomationRunLive error = %v, want nil", err)
			}
		})
	}
}
