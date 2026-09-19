package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/task/service"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	ws "github.com/kandev/kandev/pkg/websocket"
)

func mustMarshalPlanPayload(t *testing.T, v map[string]any) string {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	return string(data)
}

func revisionWithNumber(t *testing.T, revisions []*models.TaskPlanRevision, number int) *models.TaskPlanRevision {
	t.Helper()
	for _, rev := range revisions {
		if rev.RevisionNumber == number {
			return rev
		}
	}
	t.Fatalf("no revision with number %d among %d revisions", number, len(revisions))
	return nil
}

func TestPlanTruncationWarningUnknownRevisionDoesNotClaimPreservation(t *testing.T) {
	warning := planTruncationWarning(40000, 10000, 0)
	lower := strings.ToLower(warning)
	if strings.Contains(lower, "preserved") || strings.Contains(lower, "recoverable") {
		t.Fatalf("warning claims unverified recovery when no prior revision was established: %q", warning)
	}
	if !strings.Contains(lower, "could not verify") {
		t.Errorf("warning does not explain that prior-content preservation is unverified: %q", warning)
	}
}

// TestMCPPlanTruncationGuard_RejectsUnacknowledgedReduction verifies that a
// suspicious replacement is rejected before it can alter HEAD or history.
func TestMCPPlanTruncationGuard_RejectsUnacknowledgedReduction(t *testing.T) {
	h := newMCPPlanTestHandlers(t)
	ctx := context.Background()

	large := strings.Repeat("x", 40000)
	small := strings.Repeat("y", 10000) // ~25% retained, matching the WO-38 magnitude

	createOut, err := h.handleCreateTaskPlan(ctx, mcpPlanMsg(t, ws.ActionMCPCreateTaskPlan,
		mustMarshalPlanPayload(t, map[string]any{
			"task_id": mcpPlanTaskID,
			"title":   "Ship it",
			"content": large,
		})))
	if err != nil {
		t.Fatalf("handleCreateTaskPlan: %v", err)
	}
	created := decodeMCPPlanPayload(t, createOut)
	version, _ := created["version"].(string)
	if version == "" {
		t.Fatal("initial create omitted write version")
	}

	updateOut, err := h.handleUpdateTaskPlan(ctx, mcpPlanMsg(t, ws.ActionMCPUpdateTaskPlan,
		mustMarshalPlanPayload(t, map[string]any{
			"task_id": mcpPlanTaskID, "content": small, "expected_version": version,
		})))
	if err != nil {
		t.Fatalf("handleUpdateTaskPlan: %v", err)
	}
	if updateOut.Type != ws.MessageTypeError {
		t.Fatalf("type = %q, want an error", updateOut.Type)
	}
	var rejection ws.ErrorPayload
	if err := json.Unmarshal(updateOut.Payload, &rejection); err != nil {
		t.Fatalf("decode rejection: %v", err)
	}
	if rejection.Details["reason"] != "plan_truncation_rejected" || rejection.Details["write_applied"] != false {
		t.Fatalf("rejection details = %#v, want truncation/no-write", rejection.Details)
	}
	if !strings.Contains(rejection.Message, "not changed") {
		t.Fatalf("rejection message = %q, want explicit no-write guidance", rejection.Message)
	}

	revisions, err := h.planService.ListRevisions(ctx, mcpPlanTaskID)
	if err != nil {
		t.Fatalf("ListRevisions: %v", err)
	}
	if len(revisions) != 1 {
		t.Fatalf("expected one revision after rejected write, got %d", len(revisions))
	}
	fullRev1, err := h.planService.GetRevision(ctx, revisions[0].ID)
	if err != nil {
		t.Fatalf("GetRevision(rev1): %v", err)
	}
	if fullRev1.Content != large {
		t.Errorf("revision content was mutated by the rejected write; got len=%d, want len=%d",
			len(fullRev1.Content), len(large))
	}
}

// TestMCPPlanTruncationGuard_SmallDropsAreQuiet pins the two false-positive
// guards from the threshold: a small plan (under the 2,000-char floor) and a
// modest, legitimate shrink (well above the 50% retain line) must not warn.
func TestMCPPlanTruncationGuard_SmallDropsAreQuiet(t *testing.T) {
	h := newMCPPlanTestHandlers(t)
	ctx := context.Background()

	t.Run("small plan under the floor", func(t *testing.T) {
		createOut, err := h.handleCreateTaskPlan(ctx, mcpPlanMsg(t, ws.ActionMCPCreateTaskPlan,
			mustMarshalPlanPayload(t, map[string]any{
				"task_id": mcpPlanTaskID,
				"content": "a short plan",
			})))
		if err != nil {
			t.Fatalf("handleCreateTaskPlan: %v", err)
		}
		version := decodeMCPPlanPayload(t, createOut)["version"]
		out, err := h.handleUpdateTaskPlan(ctx, mcpPlanMsg(t, ws.ActionMCPUpdateTaskPlan,
			mustMarshalPlanPayload(t, map[string]any{
				"task_id": mcpPlanTaskID, "content": "x", "expected_version": version,
			})))
		if err != nil {
			t.Fatalf("handleUpdateTaskPlan: %v", err)
		}
		updated := decodeMCPPlanPayload(t, out)
		if warning, ok := updated["plan_write_warning"]; ok {
			t.Errorf("unexpected warning for a sub-floor plan: %v", warning)
		}
	})

	t.Run("legitimate prune retains more than half", func(t *testing.T) {
		large := strings.Repeat("x", 40000)
		retained := strings.Repeat("y", 25000) // 62.5% retained, above the 50% line
		createOut, err := h.handleCreateTaskPlan(ctx, mcpPlanMsg(t, ws.ActionMCPCreateTaskPlan,
			mustMarshalPlanPayload(t, map[string]any{
				"task_id": mcpPlanlessID,
				"content": large,
			})))
		if err != nil {
			t.Fatalf("handleCreateTaskPlan: %v", err)
		}
		version := decodeMCPPlanPayload(t, createOut)["version"]
		out, err := h.handleUpdateTaskPlan(ctx, mcpPlanMsg(t, ws.ActionMCPUpdateTaskPlan,
			mustMarshalPlanPayload(t, map[string]any{
				"task_id": mcpPlanlessID, "content": retained, "expected_version": version,
			})))
		if err != nil {
			t.Fatalf("handleUpdateTaskPlan: %v", err)
		}
		updated := decodeMCPPlanPayload(t, out)
		if warning, ok := updated["plan_write_warning"]; ok {
			t.Errorf("unexpected warning for a legitimate >50%% retain: %v", warning)
		}
	})
}

// TestMCPPlanTruncationGuard_NonASCIIUsesCharacterCount pins Review round 1
// Finding 1: len() on a Go string counts UTF-8 bytes, not characters. A plan
// rewritten from ASCII into a CJK script can drop 80% of its characters
// while retaining 60% of its bytes, so a byte-counting guard stays silent on
// a loss larger than either real WO-38 incident. This asserts the guard is
// measured in runes: it must fire on this drop even though the byte ratio
// alone would not cross the threshold.
func TestMCPPlanTruncationGuard_NonASCIIUsesCharacterCount(t *testing.T) {
	h := newMCPPlanTestHandlers(t)
	ctx := context.Background()

	// 4000 ASCII runes == 4000 bytes.
	large := strings.Repeat("x", 4000)
	// 800 CJK runes, each 3 UTF-8 bytes == 2400 bytes: 20% of characters
	// retained, but 60% of bytes retained — the byte ratio alone would not
	// cross the 50% line, but the character ratio must.
	small := strings.Repeat("好", 800)

	createOut, err := h.handleCreateTaskPlan(ctx, mcpPlanMsg(t, ws.ActionMCPCreateTaskPlan,
		mustMarshalPlanPayload(t, map[string]any{
			"task_id": mcpPlanTaskID,
			"content": large,
		})))
	if err != nil {
		t.Fatalf("handleCreateTaskPlan: %v", err)
	}
	version := decodeMCPPlanPayload(t, createOut)["version"]

	out, err := h.handleUpdateTaskPlan(ctx, mcpPlanMsg(t, ws.ActionMCPUpdateTaskPlan,
		mustMarshalPlanPayload(t, map[string]any{
			"task_id": mcpPlanTaskID, "content": small, "expected_version": version,
		})))
	if err != nil {
		t.Fatalf("handleUpdateTaskPlan: %v", err)
	}
	if out.Type != ws.MessageTypeError {
		t.Fatalf("type = %q, want a truncation rejection", out.Type)
	}
	var rejection ws.ErrorPayload
	if err := json.Unmarshal(out.Payload, &rejection); err != nil {
		t.Fatalf("decode rejection: %v", err)
	}
	if rejection.Details["reason"] != "plan_truncation_rejected" {
		t.Fatalf("reason = %v, want plan_truncation_rejected", rejection.Details["reason"])
	}
}

// TestMCPPlanTruncationGuard_CreateOverExistingPlanWarns pins the deliberate
// scope extension in handleCreateTaskPlan: CreatePlan upserts, so a create
// call over an existing large plan is the same destructive write as update,
// through a different door, and must be guarded identically.
func TestMCPPlanTruncationGuard_CreateOverExistingPlanRejects(t *testing.T) {
	h := newMCPPlanTestHandlers(t)
	ctx := context.Background()

	large := strings.Repeat("x", 40000)
	small := strings.Repeat("y", 10000)

	createOut, err := h.handleCreateTaskPlan(ctx, mcpPlanMsg(t, ws.ActionMCPCreateTaskPlan,
		mustMarshalPlanPayload(t, map[string]any{
			"task_id": mcpPlanlessID,
			"content": large,
		})))
	if err != nil {
		t.Fatalf("handleCreateTaskPlan (initial): %v", err)
	}
	version := decodeMCPPlanPayload(t, createOut)["version"]

	out, err := h.handleCreateTaskPlan(ctx, mcpPlanMsg(t, ws.ActionMCPCreateTaskPlan,
		mustMarshalPlanPayload(t, map[string]any{
			"task_id": mcpPlanlessID, "content": small, "expected_version": version,
		})))
	if err != nil {
		t.Fatalf("handleCreateTaskPlan (overwrite): %v", err)
	}
	if out.Type != ws.MessageTypeError {
		t.Fatalf("type = %q, want a truncation rejection", out.Type)
	}
}

// failingRevisionsRepo wraps the real sqlite repository but forces
// GetLatestTaskPlanRevision to always fail, simulating a transient lookup
// failure (e.g. SQLITE_BUSY). PlanService reads the latest revision through
// this method both to decide coalesce-vs-append and to compute the prior
// revision number named in a truncation warning.
type failingRevisionsRepo struct {
	*sqlite.Repository
	failGetPlan bool
}

func (r *failingRevisionsRepo) GetLatestTaskPlanRevision(context.Context, string) (*models.TaskPlanRevision, error) {
	return nil, errors.New("simulated revision lookup failure")
}

func (r *failingRevisionsRepo) GetTaskPlan(ctx context.Context, taskID string) (*models.TaskPlan, error) {
	if r.failGetPlan {
		r.failGetPlan = false
		return nil, errors.New("simulated plan lookup failure")
	}
	return r.Repository.GetTaskPlan(ctx, taskID)
}

// newMCPPlanTestHandlersWithFailingRevisionLookup builds Handlers like
// newMCPPlanTestHandlers, but backed by a plan service whose
// GetLatestTaskPlanRevision call always fails.
func newMCPPlanTestHandlersWithFailingRevisionLookup(t *testing.T) (*Handlers, *failingRevisionsRepo) {
	t.Helper()
	dbConn, err := db.OpenSQLite(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("OpenSQLite: %v", err)
	}
	sqlxDB := sqlx.NewDb(dbConn, "sqlite3")
	t.Cleanup(func() {
		if err := sqlxDB.Close(); err != nil {
			t.Errorf("close database: %v", err)
		}
	})
	repo, err := sqlite.NewWithDB(sqlxDB, sqlxDB, nil)
	if err != nil {
		t.Fatalf("sqlite.NewWithDB: %v", err)
	}
	t.Cleanup(func() {
		if err := repo.Close(); err != nil {
			t.Errorf("close repository: %v", err)
		}
	})

	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json", OutputPath: "stdout"})
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}
	eventBus := bus.NewMemoryEventBus(log)
	t.Cleanup(func() { eventBus.Close() })

	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: mcpPlanWS, Name: "Plan WS"}); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: mcpPlanWF, WorkspaceID: mcpPlanWS, Name: "WF"}); err != nil {
		t.Fatalf("CreateWorkflow: %v", err)
	}
	now := time.Now().UTC()
	task := &models.Task{
		ID:          mcpPlanTaskID,
		WorkspaceID: mcpPlanWS,
		WorkflowID:  mcpPlanWF,
		Title:       "Plan target",
		State:       v1.TaskStateCreated,
		Priority:    "medium",
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := repo.CreateTask(ctx, task); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	wrapped := &failingRevisionsRepo{Repository: repo}
	return &Handlers{planService: service.NewPlanService(wrapped, eventBus, log), logger: log}, wrapped
}

// TestMCPPlanTruncationGuard_RevisionLookupFailureRejects verifies that an
// intentional reduction cannot proceed when the predecessor cannot be proven
// to exist in revision history.
func TestMCPPlanTruncationGuard_RevisionLookupFailureRejects(t *testing.T) {
	h, _ := newMCPPlanTestHandlersWithFailingRevisionLookup(t)
	ctx := context.Background()

	large := strings.Repeat("x", 40000)
	small := strings.Repeat("y", 10000)

	createOut, err := h.handleCreateTaskPlan(ctx, mcpPlanMsg(t, ws.ActionMCPCreateTaskPlan,
		mustMarshalPlanPayload(t, map[string]any{
			"task_id": mcpPlanTaskID,
			"content": large,
		})))
	if err != nil {
		t.Fatalf("handleCreateTaskPlan: %v", err)
	}
	version := decodeMCPPlanPayload(t, createOut)["version"]

	out, err := h.handleUpdateTaskPlan(ctx, mcpPlanMsg(t, ws.ActionMCPUpdateTaskPlan,
		mustMarshalPlanPayload(t, map[string]any{
			"task_id": mcpPlanTaskID, "content": small, "expected_version": version, "allow_truncation": true,
		})))
	if err != nil {
		t.Fatalf("handleUpdateTaskPlan: %v", err)
	}
	if out.Type != ws.MessageTypeError {
		t.Fatalf("type = %q, want a history rejection", out.Type)
	}
	var rejection ws.ErrorPayload
	if err := json.Unmarshal(out.Payload, &rejection); err != nil {
		t.Fatalf("decode rejection: %v", err)
	}
	if rejection.Details["reason"] != "plan_history_unavailable" || rejection.Details["write_applied"] != false {
		t.Fatalf("rejection details = %#v, want history-unavailable/no-write", rejection.Details)
	}
}

func TestMCPPlanTruncationGuard_PlanLookupFailureRejects(t *testing.T) {
	h, repo := newMCPPlanTestHandlersWithFailingRevisionLookup(t)
	ctx := context.Background()

	large := strings.Repeat("x", 40000)
	small := strings.Repeat("y", 10000)

	_, err := h.handleCreateTaskPlan(ctx, mcpPlanMsg(t, ws.ActionMCPCreateTaskPlan,
		mustMarshalPlanPayload(t, map[string]any{
			"task_id": mcpPlanTaskID,
			"content": large,
		})))
	if err != nil {
		t.Fatalf("handleCreateTaskPlan: %v", err)
	}

	repo.failGetPlan = true

	out, err := h.handleUpdateTaskPlan(ctx, mcpPlanMsg(t, ws.ActionMCPUpdateTaskPlan,
		mustMarshalPlanPayload(t, map[string]any{
			"task_id": mcpPlanTaskID, "content": small, "expected_version": "unknown",
		})))
	if err != nil {
		t.Fatalf("handleUpdateTaskPlan: %v", err)
	}
	if out.Type != ws.MessageTypeError {
		t.Fatalf("type = %q, want a head-read rejection", out.Type)
	}
	var rejection ws.ErrorPayload
	if err := json.Unmarshal(out.Payload, &rejection); err != nil {
		t.Fatalf("decode rejection: %v", err)
	}
	if rejection.Details["reason"] != "plan_head_unavailable" || rejection.Details["write_applied"] != false {
		t.Fatalf("rejection details = %#v, want head-unavailable/no-write", rejection.Details)
	}

	revisions, err := repo.Repository.ListTaskPlanRevisions(ctx, mcpPlanTaskID, 0)
	if err != nil {
		t.Fatalf("ListTaskPlanRevisions: %v", err)
	}
	if len(revisions) != 1 {
		t.Fatalf("expected one revision after a guarded read failure, got %d", len(revisions))
	}
	rev1 := revisionWithNumber(t, revisions, 1)
	fullRev1, err := repo.GetTaskPlanRevision(ctx, rev1.ID)
	if err != nil {
		t.Fatalf("GetTaskPlanRevision(rev1): %v", err)
	}
	if fullRev1.Content != large {
		t.Errorf("revision 1 content was mutated after a guarded read failure; got len=%d, want len=%d",
			len(fullRev1.Content), len(large))
	}
}
