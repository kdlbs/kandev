package handlers

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/task/models"
	taskrepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/task/service"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	ws "github.com/kandev/kandev/pkg/websocket"
)

// The WS half of the same gaps. task.state and task.move name their task `id`,
// which is what let them past the gateway backstop; these assert the handlers
// refuse regardless, since the service guard is the actual fix and the handler
// is also what a non-gateway caller reaches.
//
// The denial assertion is deliberately a *comparison* rather than a fixed error
// code: what matters is that a foreign task is indistinguishable from one that
// does not exist, which is what these handlers already emit for any service
// error.

// authzWSRepo serves exactly one task — user-b's — and records every mutating
// write. Any other ID is genuinely absent, so "foreign" and "missing" can be
// compared against each other.
type authzWSRepo struct {
	mockRepository

	mu        sync.Mutex
	states    []string
	positions []string
}

func (r *authzWSRepo) GetTask(_ context.Context, id string) (*models.Task, error) {
	if id != "task-b" {
		return nil, taskrepo.ErrTaskNotFound
	}
	return &models.Task{
		ID: "task-b", WorkspaceID: "ws-b", Title: "Victim",
		WorkflowID: "wf-b", WorkflowStepID: "step-b-1", State: v1.TaskStateTODO,
	}, nil
}

func (r *authzWSRepo) GetWorkspace(_ context.Context, id string) (*models.Workspace, error) {
	return &models.Workspace{ID: id, Name: "B's", OwnerID: "user-b"}, nil
}

func (r *authzWSRepo) UpdateTaskState(_ context.Context, id string, state v1.TaskState) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.states = append(r.states, id+"="+string(state))
	return nil
}

func (r *authzWSRepo) UpdateTask(_ context.Context, task *models.Task) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.positions = append(r.positions, task.ID+"@"+task.WorkflowStepID)
	return nil
}

func (r *authzWSRepo) writes() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.states) + len(r.positions)
}

func (r *authzWSRepo) CountToolCallMessagesBySession(context.Context, []string) (map[string]int, error) {
	return nil, nil
}

func (r *authzWSRepo) ListChildren(context.Context, string) ([]*models.Task, error) {
	return nil, nil
}

func newAuthzWSHandlers(t *testing.T, repo *authzWSRepo) *TaskHandlers {
	t.Helper()
	log := newTestLogger(t)
	svc := service.NewService(service.Repos{
		Workspaces: repo, Tasks: repo, TaskRepos: repo,
		Workflows: repo, Messages: repo, Turns: repo,
		Sessions: repo, GitSnapshots: repo, RepoEntities: repo,
		Executors: repo, Environments: repo, TaskEnvironments: repo,
		Reviews: repo,
	}, nil, log, service.RepositoryDiscoveryConfig{})
	return &TaskHandlers{service: svc, repo: repo, logger: log}
}

func authzWSContext(userID string) context.Context {
	return authn.WithIdentity(context.Background(),
		authn.Identity{UserID: userID, Role: authn.RoleMember})
}

func authzWSMessage(t *testing.T, action string, payload map[string]any) *ws.Message {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	return &ws.Message{ID: "msg-1", Action: action, Payload: raw}
}

// authzWSCall names one entry point and how to invoke it against a task ID.
type authzWSCall struct {
	action string
	invoke func(*TaskHandlers, context.Context, string) (*ws.Message, error)
}

func authzWSCalls(t *testing.T) []authzWSCall {
	t.Helper()
	return []authzWSCall{
		{ws.ActionTaskState, func(h *TaskHandlers, ctx context.Context, id string) (*ws.Message, error) {
			return h.wsUpdateTaskState(ctx, authzWSMessage(t, ws.ActionTaskState,
				map[string]any{"id": id, "state": string(v1.TaskStateCompleted)}))
		}},
		{ws.ActionTaskMove, func(h *TaskHandlers, ctx context.Context, id string) (*ws.Message, error) {
			return h.wsMoveTask(ctx, authzWSMessage(t, ws.ActionTaskMove, map[string]any{
				"id": id, "workflow_id": "wf-b", "workflow_step_id": "step-b-2", "position": 3,
			}))
		}},
	}
}

func TestWSTaskStateAndMoveDenyForeignTask(t *testing.T) {
	for _, tc := range authzWSCalls(t) {
		t.Run(tc.action, func(t *testing.T) {
			repo := &authzWSRepo{}
			h := newAuthzWSHandlers(t, repo)

			denied, err := tc.invoke(h, authzWSContext("user-a"), "task-b")
			if err != nil {
				t.Fatalf("%s: %v", tc.action, err)
			}
			if denied.Type != ws.MessageTypeError {
				t.Fatalf("foreign %s: type = %s payload = %s, want error",
					tc.action, denied.Type, denied.Payload)
			}
			if strings.Contains(string(denied.Payload), "Victim") {
				t.Fatalf("the foreign task leaked into the response: %s", denied.Payload)
			}
			if n := repo.writes(); n != 0 {
				t.Fatalf("a denied %s reached the repository (%d writes)", tc.action, n)
			}

			// A denial must read exactly like a task that does not exist.
			missing, err := tc.invoke(h, authzWSContext("user-a"), "task-nonexistent")
			if err != nil {
				t.Fatalf("%s (unknown task): %v", tc.action, err)
			}
			if string(missing.Payload) != string(denied.Payload) {
				t.Fatalf("denial is distinguishable from a missing task:\n denied  = %s\n missing = %s",
					denied.Payload, missing.Payload)
			}
		})
	}
}

// TestWSTaskStateAllowsOwner is the witness for the denial above: without it the
// test would still pass if the guard refused everyone.
func TestWSTaskStateAllowsOwner(t *testing.T) {
	repo := &authzWSRepo{}
	h := newAuthzWSHandlers(t, repo)

	resp, err := h.wsUpdateTaskState(authzWSContext("user-b"), authzWSMessage(t, ws.ActionTaskState,
		map[string]any{"id": "task-b", "state": string(v1.TaskStateCompleted)}))
	if err != nil {
		t.Fatalf("owner task.state: %v", err)
	}
	if resp.Type == ws.MessageTypeError {
		t.Fatalf("owner task.state was refused: %s", resp.Payload)
	}
	if repo.writes() == 0 {
		t.Fatal("owner task.state did not reach the repository")
	}
}
