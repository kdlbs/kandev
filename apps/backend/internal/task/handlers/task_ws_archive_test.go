package handlers

import (
	"context"
	"encoding/json"
	"sync"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/service"
	ws "github.com/kandev/kandev/pkg/websocket"
)

// archiveStampRepo overrides just enough of mockRepository for the
// HandoffService archive path: GetTask returns a live task and
// ArchiveTaskIfActive records the cascade stamp.
type archiveStampRepo struct {
	mockRepository
	mu      sync.Mutex
	stamped map[string]string // taskID → cascadeID
}

func (r *archiveStampRepo) GetTask(_ context.Context, id string) (*models.Task, error) {
	return &models.Task{ID: id, WorkspaceID: "ws-1"}, nil
}

func (r *archiveStampRepo) ArchiveTaskIfActive(_ context.Context, id, cascadeID string) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.stamped == nil {
		r.stamped = map[string]string{}
	}
	r.stamped[id] = cascadeID
	return true, nil
}

func (r *archiveStampRepo) ArchiveTaskIfActiveWithVacatedStep(
	ctx context.Context,
	id string,
	cascadeID string,
) (string, bool, error) {
	changed, err := r.ArchiveTaskIfActive(ctx, id, cascadeID)
	return "", changed, err
}

// TestWsArchiveTask_StampsCascadeID is the WS-side parity test: archives
// issued over the WebSocket action must route through HandoffService and
// receive a cascade stamp, otherwise WS-archived tasks were permanently
// unrecoverable (UnarchiveTaskByCascade requires the stamp; the manual
// fallback only exists for legacy rows).
func TestWsArchiveTask_StampsCascadeID(t *testing.T) {
	repo := &archiveStampRepo{}
	h := &TaskHandlers{
		handoffSvc: service.NewHandoffService(repo, nil, nil, nil, nil, nil),
		logger:     newTestLogger(t),
	}

	msg := &ws.Message{
		ID:      "msg-1",
		Action:  ws.ActionTaskArchive,
		Payload: json.RawMessage(`{"id":"task-1"}`),
	}
	resp, err := h.wsArchiveTask(context.Background(), msg)
	if err != nil {
		t.Fatalf("wsArchiveTask: %v", err)
	}
	if resp == nil || resp.Type == ws.MessageTypeError {
		t.Fatalf("wsArchiveTask response = %+v, want success", resp)
	}

	repo.mu.Lock()
	defer repo.mu.Unlock()
	cascadeID, ok := repo.stamped["task-1"]
	if !ok {
		t.Fatal("task-1 was not archived through the handoff cascade path")
	}
	if cascadeID == "" {
		t.Fatal("cascade ID must be stamped so the task stays unarchivable")
	}
}

type alreadyArchivedRepo struct {
	mockRepository
}

func (r *alreadyArchivedRepo) GetTask(_ context.Context, id string) (*models.Task, error) {
	return &models.Task{ID: id, WorkspaceID: "ws-1"}, nil
}

func (r *alreadyArchivedRepo) ArchiveTaskIfActive(_ context.Context, _, _ string) (bool, error) {
	return false, nil
}

func (r *alreadyArchivedRepo) ArchiveTaskIfActiveWithVacatedStep(
	ctx context.Context,
	id string,
	cascadeID string,
) (string, bool, error) {
	changed, err := r.ArchiveTaskIfActive(ctx, id, cascadeID)
	return "", changed, err
}

func TestWsArchiveTask_ReportsAlreadyArchivedOutcome(t *testing.T) {
	repo := &alreadyArchivedRepo{}
	h := &TaskHandlers{
		handoffSvc: service.NewHandoffService(repo, nil, nil, nil, nil, nil),
		logger:     newTestLogger(t),
	}

	msg := &ws.Message{
		ID:      "msg-1",
		Action:  ws.ActionTaskArchive,
		Payload: json.RawMessage(`{"id":"task-1"}`),
	}
	resp, err := h.wsArchiveTask(context.Background(), msg)
	if err != nil {
		t.Fatalf("wsArchiveTask: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(resp.Payload, &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload["already_archived"] != true {
		t.Fatalf("already_archived = %v, want true", payload["already_archived"])
	}
}

type missingArchiveRepo struct {
	mockRepository
}

func (r *missingArchiveRepo) GetTask(_ context.Context, _ string) (*models.Task, error) {
	return nil, nil
}

func TestWsArchiveTask_ReportsMissingTask(t *testing.T) {
	h := &TaskHandlers{
		handoffSvc: service.NewHandoffService(&missingArchiveRepo{}, nil, nil, nil, nil, nil),
		logger:     newTestLogger(t),
	}
	msg := &ws.Message{
		ID:      "msg-1",
		Action:  ws.ActionTaskArchive,
		Payload: json.RawMessage(`{"id":"missing-task"}`),
	}

	resp, err := h.wsArchiveTask(context.Background(), msg)
	if err != nil {
		t.Fatalf("wsArchiveTask: %v", err)
	}
	if resp.Type != ws.MessageTypeError {
		t.Fatalf("response type = %s, want error", resp.Type)
	}
	var payload struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(resp.Payload, &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Code != string(ws.ErrorCodeNotFound) {
		t.Fatalf("error code = %q, want %q", payload.Code, ws.ErrorCodeNotFound)
	}
}
