package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	"github.com/kandev/kandev/internal/task/service"
	ws "github.com/kandev/kandev/pkg/websocket"
)

// runnerSwitchWSRepo is a minimal, fully-controlled fake for the task.runner
// WS handler: one task, one executor profile/executor pair, and an
// in-memory metadata map that SwitchTaskRunner mutates on a successful
// no-conflict switch, so the handler's response DTO reflects a real change.
type runnerSwitchWSRepo struct {
	mockRepository

	task              *models.Task
	executor          *models.Executor
	executorProfile   *models.ExecutorProfile
	mutabilityBlocked bool
}

func (r *runnerSwitchWSRepo) GetTask(_ context.Context, id string) (*models.Task, error) {
	if r.task == nil || id != r.task.ID {
		return nil, repoerrors.ErrTaskNotFound
	}
	return r.task, nil
}

func (r *runnerSwitchWSRepo) GetExecutorProfile(_ context.Context, id string) (*models.ExecutorProfile, error) {
	if r.executorProfile == nil || id != r.executorProfile.ID {
		return nil, repoerrors.ErrExecutorProfileNotFound
	}
	return r.executorProfile, nil
}

func (r *runnerSwitchWSRepo) GetExecutor(_ context.Context, id string) (*models.Executor, error) {
	if r.executor == nil || id != r.executor.ID {
		return nil, models.ErrExecutorNotFound
	}
	return r.executor, nil
}

func (r *runnerSwitchWSRepo) SwitchTaskRunner(_ context.Context, req models.RunnerSwitchRequest) (*models.RunnerSwitchResult, error) {
	if r.mutabilityBlocked {
		return nil, &repoerrors.ErrRunnerMutabilityConflict{Reason: models.RunnerReasonSessionExists}
	}
	stored, _ := r.task.Metadata[models.MetaKeyExecutorProfileID].(string)
	if stored == req.ExecutorProfileID {
		return &models.RunnerSwitchResult{Task: r.task, Changed: false}, nil
	}
	if r.task.Metadata == nil {
		r.task.Metadata = map[string]interface{}{}
	}
	r.task.Metadata[models.MetaKeyExecutorProfileID] = req.ExecutorProfileID
	r.task.UpdatedAt = time.Now().UTC()
	return &models.RunnerSwitchResult{Task: r.task, Changed: true}, nil
}

func newRunnerSwitchWSHandlers(t *testing.T, repo *runnerSwitchWSRepo) *TaskHandlers {
	t.Helper()
	log := newTestLogger(t)
	svc := service.NewService(service.Repos{
		Workspaces: repo, Tasks: repo, TaskRepos: repo,
		Workflows: repo, Messages: repo, Turns: repo,
		Sessions: repo, GitSnapshots: repo, RepoEntities: repo,
		Executors: repo, Environments: repo, TaskEnvironments: repo,
		Reviews: repo,
	}, nil, log, service.RepositoryDiscoveryConfig{})
	return &TaskHandlers{service: svc, logger: log}
}

func runnerSwitchWSMessage(t *testing.T, id, executorProfileID string) *ws.Message {
	t.Helper()
	raw, err := json.Marshal(map[string]any{"id": id, "executor_profile_id": executorProfileID})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	return &ws.Message{ID: "msg-1", Action: ws.ActionTaskRunner, Payload: raw}
}

func TestWSUpdateTaskRunnerSuccessReturnsEnrichedDTO(t *testing.T) {
	repo := &runnerSwitchWSRepo{
		task:            &models.Task{ID: "task-1", WorkspaceID: "ws-1", Title: "T"},
		executor:        &models.Executor{ID: "exec-1", Status: models.ExecutorStatusActive},
		executorProfile: &models.ExecutorProfile{ID: "profile-1", ExecutorID: "exec-1"},
	}
	h := newRunnerSwitchWSHandlers(t, repo)

	resp, err := h.wsUpdateTaskRunner(context.Background(), runnerSwitchWSMessage(t, "task-1", "profile-1"))
	if err != nil {
		t.Fatalf("wsUpdateTaskRunner: %v", err)
	}
	if resp.Type != ws.MessageTypeResponse {
		t.Fatalf("response type = %v, want response (payload: %s)", resp.Type, resp.Payload)
	}
	var body map[string]any
	if err := json.Unmarshal(resp.Payload, &body); err != nil {
		t.Fatalf("unmarshal response payload: %v", err)
	}
	if body["id"] != "task-1" {
		t.Fatalf("response id = %v, want task-1", body["id"])
	}
	if _, ok := body["runner_editable"]; !ok {
		t.Fatal("response missing runner_editable")
	}
	if _, ok := body["runner_ineligible_reason"]; !ok {
		t.Fatal("response missing runner_ineligible_reason")
	}
	if got, _ := repo.task.Metadata[models.MetaKeyExecutorProfileID].(string); got != "profile-1" {
		t.Fatalf("persisted executor_profile_id = %q, want profile-1", got)
	}
}

func TestWSUpdateTaskRunnerInvalidPayload(t *testing.T) {
	h := newRunnerSwitchWSHandlers(t, &runnerSwitchWSRepo{})
	resp, err := h.wsUpdateTaskRunner(context.Background(), &ws.Message{ID: "msg-1", Action: ws.ActionTaskRunner, Payload: json.RawMessage(`not-json`)})
	if err != nil {
		t.Fatalf("wsUpdateTaskRunner: %v", err)
	}
	assertWSErrorCode(t, resp, ws.ErrorCodeBadRequest)
}

func TestWSUpdateTaskRunnerErrorMapping(t *testing.T) {
	tests := []struct {
		name     string
		repo     *runnerSwitchWSRepo
		id       string
		profile  string
		wantCode string
	}{
		{
			name:     "malformed",
			repo:     &runnerSwitchWSRepo{},
			id:       "",
			profile:  "profile-1",
			wantCode: ws.ErrorCodeValidation,
		},
		{
			name:     "task not found",
			repo:     &runnerSwitchWSRepo{},
			id:       "task-missing",
			profile:  "profile-1",
			wantCode: ws.ErrorCodeNotFound,
		},
		{
			name: "target invalid",
			repo: &runnerSwitchWSRepo{
				task: &models.Task{ID: "task-1", WorkspaceID: "ws-1", Title: "T"},
			},
			id:       "task-1",
			profile:  "profile-missing",
			wantCode: ws.ErrorCodeValidation,
		},
		{
			name: "mutability conflict",
			repo: &runnerSwitchWSRepo{
				task:              &models.Task{ID: "task-1", WorkspaceID: "ws-1", Title: "T"},
				executor:          &models.Executor{ID: "exec-1", Status: models.ExecutorStatusActive},
				executorProfile:   &models.ExecutorProfile{ID: "profile-1", ExecutorID: "exec-1"},
				mutabilityBlocked: true,
			},
			id:       "task-1",
			profile:  "profile-1",
			wantCode: ws.ErrorCodeConflict,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := newRunnerSwitchWSHandlers(t, tc.repo)
			resp, err := h.wsUpdateTaskRunner(context.Background(), runnerSwitchWSMessage(t, tc.id, tc.profile))
			if err != nil {
				t.Fatalf("wsUpdateTaskRunner: %v", err)
			}
			assertWSErrorCode(t, resp, tc.wantCode)
		})
	}
}

func assertWSErrorCode(t *testing.T, resp *ws.Message, want string) {
	t.Helper()
	if resp.Type != ws.MessageTypeError {
		t.Fatalf("response type = %v, want error (payload: %s)", resp.Type, resp.Payload)
	}
	var body struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(resp.Payload, &body); err != nil {
		t.Fatalf("unmarshal error payload: %v", err)
	}
	if body.Code != want {
		t.Fatalf("error code = %q, want %q (payload: %s)", body.Code, want, resp.Payload)
	}
}

func TestRunnerSwitchWSErrorMapsEvaluationUnavailable(t *testing.T) {
	msg := &ws.Message{ID: "msg-1", Action: ws.ActionTaskRunner}
	resp, err := runnerSwitchWSError(msg, errors.New("wrapped: "+repoerrors.ErrRunnerEvaluationUnavailable.Error()))
	if err != nil {
		t.Fatalf("runnerSwitchWSError: %v", err)
	}
	assertWSErrorCode(t, resp, ws.ErrorCodeInternalError)

	resp, err = runnerSwitchWSError(msg, repoerrors.ErrRunnerEvaluationUnavailable)
	if err != nil {
		t.Fatalf("runnerSwitchWSError: %v", err)
	}
	assertWSErrorCode(t, resp, ws.ErrorCodeUnavailable)
}
