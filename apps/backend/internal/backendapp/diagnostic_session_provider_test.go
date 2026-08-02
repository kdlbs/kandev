package backendapp

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/task/models"
)

func TestDiagnosticSessionProviderBuildsMessageFreeAllowList(t *testing.T) {
	started := time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC)
	reader := &diagnosticSessionReaderStub{
		workspaces: []*models.Workspace{{ID: "ws-1", OwnerID: "user-1"}},
		tasks: map[string][]*models.Task{
			"ws-1": {{ID: "task-1", Title: "Repair diagnostic export", WorkspaceID: "ws-1"}},
		},
		sessions: map[string][]*models.TaskSession{
			"task-1": {{
				ID: "session-1", TaskID: "task-1", State: models.TaskSessionStateRunning,
				StartedAt: started, UpdatedAt: started.Add(time.Minute),
				AgentProfileSnapshot: map[string]interface{}{
					"agent_name": "claude-acp", "provider": "anthropic", "model": "sonnet",
					"title": "must not leak",
				},
				ExecutorSnapshot: map[string]interface{}{"type": "local_docker"},
			}},
		},
	}
	provider := newDiagnosticSessionProvider(reader)
	rows, err := provider.ListDiagnosticSessions(
		context.Background(),
		authn.Identity{UserID: "user-1", Role: authn.RoleMember},
		started.Add(-time.Hour), nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(rows))
	}
	row := rows[0]
	if row.TaskID != "task-1" || row.TaskTitle != "Repair diagnostic export" ||
		row.SessionID != "session-1" || row.Agent != "claude-acp" ||
		row.Provider != "anthropic" || row.Model != "sonnet" || row.ExecutorType != "local_docker" {
		t.Fatalf("allow-list row = %#v", row)
	}
	if row.Status != string(models.TaskSessionStateRunning) || !row.LastActivityAt.Equal(started.Add(time.Minute)) {
		t.Fatalf("runtime fields = %#v", row)
	}
	if reader.lastIdentity.UserID != "user-1" {
		t.Fatalf("provider did not preserve caller identity: %#v", reader.lastIdentity)
	}
}

type diagnosticSessionReaderStub struct {
	workspaces   []*models.Workspace
	tasks        map[string][]*models.Task
	sessions     map[string][]*models.TaskSession
	lastIdentity authn.Identity
}

func (r *diagnosticSessionReaderStub) ListWorkspaces(ctx context.Context) ([]*models.Workspace, error) {
	r.lastIdentity, _ = authn.IdentityFromContext(ctx)
	return r.workspaces, nil
}

func (r *diagnosticSessionReaderStub) ListTasksByWorkspace(
	context.Context, string, string, string, string, int, int, string, bool, bool, bool, bool,
) ([]*models.Task, int, error) {
	return r.tasks["ws-1"], len(r.tasks["ws-1"]), nil
}

func (r *diagnosticSessionReaderStub) ListTaskSessions(
	_ context.Context, taskID string,
) ([]*models.TaskSession, error) {
	return r.sessions[taskID], nil
}

func (r *diagnosticSessionReaderStub) GetTaskSession(
	_ context.Context, sessionID string,
) (*models.TaskSession, error) {
	for _, sessions := range r.sessions {
		for _, session := range sessions {
			if session.ID == sessionID {
				return session, nil
			}
		}
	}
	return nil, models.ErrTaskSessionNotFound
}

func (r *diagnosticSessionReaderStub) GetExecutorRunningBySessionID(
	context.Context, string,
) (*models.ExecutorRunning, error) {
	return nil, models.ErrExecutorRunningNotFound
}
