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

// TestMCPPlanTruncationGuard_WarnsAndPreservesHistory pins the defect this
// card fixes: a write that drops the majority of a substantial plan today
// returns plain success with no signal that anything shrank (WO-38, task
// 809498b3, measured two incidents dropping 76-77% of a 40k+ char plan).
//
// This asserts against a response shape (plan_write_warning,
// prior_revision_number) that does not exist yet, so it fails first.
func TestMCPPlanTruncationGuard_WarnsAndPreservesHistory(t *testing.T) {
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
	if warning, ok := created["plan_write_warning"]; ok {
		t.Errorf("unexpected warning on initial create (no prior plan to truncate): %v", warning)
	}

	updateOut, err := h.handleUpdateTaskPlan(ctx, mcpPlanMsg(t, ws.ActionMCPUpdateTaskPlan,
		mustMarshalPlanPayload(t, map[string]any{
			"task_id": mcpPlanTaskID,
			"content": small,
		})))
	if err != nil {
		t.Fatalf("handleUpdateTaskPlan: %v", err)
	}
	updated := decodeMCPPlanPayload(t, updateOut)
	if updated["content"] != small {
		t.Errorf("content = %v, want the new (truncated) content", updated["content"])
	}

	warning, _ := updated["plan_write_warning"].(string)
	if warning == "" {
		t.Fatal("expected a truncation warning naming the character drop, got none")
	}
	if !strings.Contains(warning, "40000") || !strings.Contains(warning, "10000") {
		t.Errorf("warning does not name the character drop (40000 -> 10000): %q", warning)
	}
	if !strings.Contains(strings.ToLower(warning), "entire") && !strings.Contains(strings.ToLower(warning), "whole document") {
		t.Errorf("warning does not explain the write replaced the whole document: %q", warning)
	}

	priorRev, ok := updated["prior_revision_number"].(float64)
	if !ok || int(priorRev) != 1 {
		t.Errorf("prior_revision_number = %v, want 1", updated["prior_revision_number"])
	}

	// The truncating write must NOT coalesce: revision 1 must survive as its
	// own row with the full pre-truncation content, not be overwritten
	// in-place by mergeRevisionInTx.
	revisions, err := h.planService.ListRevisions(ctx, mcpPlanTaskID)
	if err != nil {
		t.Fatalf("ListRevisions: %v", err)
	}
	if len(revisions) != 2 {
		t.Fatalf("expected 2 separate revisions (no coalesce), got %d", len(revisions))
	}
	rev1 := revisionWithNumber(t, revisions, 1)
	fullRev1, err := h.planService.GetRevision(ctx, rev1.ID)
	if err != nil {
		t.Fatalf("GetRevision(rev1): %v", err)
	}
	if fullRev1.Content != large {
		t.Errorf("revision 1 content was mutated by the truncating write; got len=%d, want len=%d",
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
		_, err := h.handleCreateTaskPlan(ctx, mcpPlanMsg(t, ws.ActionMCPCreateTaskPlan,
			mustMarshalPlanPayload(t, map[string]any{
				"task_id": mcpPlanTaskID,
				"content": "a short plan",
			})))
		if err != nil {
			t.Fatalf("handleCreateTaskPlan: %v", err)
		}
		out, err := h.handleUpdateTaskPlan(ctx, mcpPlanMsg(t, ws.ActionMCPUpdateTaskPlan,
			mustMarshalPlanPayload(t, map[string]any{
				"task_id": mcpPlanTaskID,
				"content": "x",
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
		_, err := h.handleCreateTaskPlan(ctx, mcpPlanMsg(t, ws.ActionMCPCreateTaskPlan,
			mustMarshalPlanPayload(t, map[string]any{
				"task_id": mcpPlanlessID,
				"content": large,
			})))
		if err != nil {
			t.Fatalf("handleCreateTaskPlan: %v", err)
		}
		out, err := h.handleUpdateTaskPlan(ctx, mcpPlanMsg(t, ws.ActionMCPUpdateTaskPlan,
			mustMarshalPlanPayload(t, map[string]any{
				"task_id": mcpPlanlessID,
				"content": retained,
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

	_, err := h.handleCreateTaskPlan(ctx, mcpPlanMsg(t, ws.ActionMCPCreateTaskPlan,
		mustMarshalPlanPayload(t, map[string]any{
			"task_id": mcpPlanTaskID,
			"content": large,
		})))
	if err != nil {
		t.Fatalf("handleCreateTaskPlan: %v", err)
	}

	out, err := h.handleUpdateTaskPlan(ctx, mcpPlanMsg(t, ws.ActionMCPUpdateTaskPlan,
		mustMarshalPlanPayload(t, map[string]any{
			"task_id": mcpPlanTaskID,
			"content": small,
		})))
	if err != nil {
		t.Fatalf("handleUpdateTaskPlan: %v", err)
	}
	updated := decodeMCPPlanPayload(t, out)

	warning, _ := updated["plan_write_warning"].(string)
	if warning == "" {
		t.Fatal("expected a truncation warning for an 80 percent character drop disguised as a 60 percent byte retain, got none")
	}
	if !strings.Contains(warning, "4000") || !strings.Contains(warning, "800") {
		t.Errorf("warning does not name the character counts (4000 -> 800): %q", warning)
	}
}

// TestMCPPlanTruncationGuard_CreateOverExistingPlanWarns pins the deliberate
// scope extension in handleCreateTaskPlan: CreatePlan upserts, so a create
// call over an existing large plan is the same destructive write as update,
// through a different door, and must be guarded identically.
func TestMCPPlanTruncationGuard_CreateOverExistingPlanWarns(t *testing.T) {
	h := newMCPPlanTestHandlers(t)
	ctx := context.Background()

	large := strings.Repeat("x", 40000)
	small := strings.Repeat("y", 10000)

	_, err := h.handleCreateTaskPlan(ctx, mcpPlanMsg(t, ws.ActionMCPCreateTaskPlan,
		mustMarshalPlanPayload(t, map[string]any{
			"task_id": mcpPlanlessID,
			"content": large,
		})))
	if err != nil {
		t.Fatalf("handleCreateTaskPlan (initial): %v", err)
	}

	out, err := h.handleCreateTaskPlan(ctx, mcpPlanMsg(t, ws.ActionMCPCreateTaskPlan,
		mustMarshalPlanPayload(t, map[string]any{
			"task_id": mcpPlanlessID,
			"content": small,
		})))
	if err != nil {
		t.Fatalf("handleCreateTaskPlan (overwrite): %v", err)
	}
	updated := decodeMCPPlanPayload(t, out)

	warning, _ := updated["plan_write_warning"].(string)
	if warning == "" {
		t.Fatal("expected a truncation warning when create_task_plan_kandev overwrites an existing large plan, got none")
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

// TestMCPPlanTruncationGuard_RevisionLookupFailureOmitsRevisionNumber pins
// Review round 2 Finding 3: revision numbering starts at 1
// (NextTaskPlanRevisionNumber), so "plan revision 0" can never be a real
// revision. When the latest-revision lookup fails on a truncating write, the
// plan service must still force a new revision and still warn — coalescing
// here would overwrite the only surviving copy of the pre-truncation content
// — but the rendered warning must not claim the content lives in revision 0.
func TestMCPPlanTruncationGuard_RevisionLookupFailureOmitsRevisionNumber(t *testing.T) {
	h, _ := newMCPPlanTestHandlersWithFailingRevisionLookup(t)
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

	out, err := h.handleUpdateTaskPlan(ctx, mcpPlanMsg(t, ws.ActionMCPUpdateTaskPlan,
		mustMarshalPlanPayload(t, map[string]any{
			"task_id": mcpPlanTaskID,
			"content": small,
		})))
	if err != nil {
		t.Fatalf("handleUpdateTaskPlan: %v", err)
	}
	updated := decodeMCPPlanPayload(t, out)

	warning, _ := updated["plan_write_warning"].(string)
	if warning == "" {
		t.Fatal("expected a truncation warning even when the revision lookup fails, got none")
	}
	if strings.Contains(warning, "revision 0") {
		t.Errorf("warning names a nonexistent revision 0 (revision numbering starts at 1): %q", warning)
	}
	if !strings.Contains(warning, "40000") || !strings.Contains(warning, "10000") {
		t.Errorf("warning does not name the character drop (40000 -> 10000): %q", warning)
	}

	if _, ok := updated["prior_revision_number"]; ok {
		t.Errorf("prior_revision_number should be omitted when the revision number is unknown, got %v",
			updated["prior_revision_number"])
	}
}

func TestMCPPlanTruncationGuard_PlanLookupFailurePreservesHistory(t *testing.T) {
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
			"task_id": mcpPlanTaskID,
			"content": small,
		})))
	if err != nil {
		t.Fatalf("handleUpdateTaskPlan: %v", err)
	}
	updated := decodeMCPPlanPayload(t, out)
	if warning, ok := updated["plan_write_warning"]; ok {
		t.Errorf("unexpected warning when the guard could not read the prior plan: %v", warning)
	}

	revisions, err := repo.Repository.ListTaskPlanRevisions(ctx, mcpPlanTaskID, 0)
	if err != nil {
		t.Fatalf("ListTaskPlanRevisions: %v", err)
	}
	if len(revisions) != 2 {
		t.Fatalf("expected 2 revisions after a guarded read failure, got %d", len(revisions))
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
