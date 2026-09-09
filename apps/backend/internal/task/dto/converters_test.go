package dto

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
)

func TestFromWorkflowStep_PreservesGenericEvents(t *testing.T) {
	step := &wfmodels.WorkflowStep{
		ID:         "step-1",
		WorkflowID: "wf-1",
		Name:       "Work",
		Events: wfmodels.StepEvents{
			OnChildrenCompleted: []wfmodels.GenericAction{{
				Type: wfmodels.GenericActionQueueRun,
				Config: map[string]interface{}{
					"target": "primary",
					"reason": "children_completed",
				},
			}},
		},
	}

	got := FromWorkflowStep(step)
	if got.Events == nil {
		t.Fatal("Events nil, want on_children_completed")
	}
	if len(got.Events.OnChildrenCompleted) != 1 {
		t.Fatalf("OnChildrenCompleted len = %d, want 1", len(got.Events.OnChildrenCompleted))
	}
	action := got.Events.OnChildrenCompleted[0]
	if action.Type != "queue_run" {
		t.Fatalf("action type = %q, want queue_run", action.Type)
	}
	if action.Config["reason"] != "children_completed" {
		t.Fatalf("action reason = %v, want children_completed", action.Config["reason"])
	}
}

func TestFromTaskSerializesWorkspaceFolders(t *testing.T) {
	now := time.Now().UTC()
	got := FromTask(&models.Task{
		ID: "task-folders",
		WorkspaceFolders: []*models.TaskWorkspaceFolder{{
			ID: "folder-1", TaskID: "task-folders", LocalPath: "/canonical/docs", DisplayName: "docs", Position: 2, CreatedAt: now, UpdatedAt: now,
		}},
	})
	payload, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("marshal task DTO: %v", err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(payload, &fields); err != nil {
		t.Fatalf("decode task DTO: %v", err)
	}
	if _, ok := fields["workspace_folders"]; !ok {
		t.Fatalf("workspace_folders missing from task DTO: %s", payload)
	}
}

func TestFromTaskSerializesAutopilot(t *testing.T) {
	got := FromTask(&models.Task{ID: "task-autopilot", Autopilot: true})
	if !got.Autopilot {
		t.Fatal("Autopilot = false, want true")
	}
	payload, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("marshal task DTO: %v", err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(payload, &fields); err != nil {
		t.Fatalf("decode task DTO: %v", err)
	}
	if raw, ok := fields["autopilot"]; !ok || string(raw) != "true" {
		t.Fatalf("autopilot field = %s, want true", raw)
	}
}

func TestFromTaskWithSessionInfoSerializesExecutorProfileID(t *testing.T) {
	profileID := "executor-profile-1"
	task := &models.Task{ID: "task-executor-profile"}

	got := FromTaskWithSessionInfo(
		task,
		nil,
		nil,
		models.ReviewStatusNone,
		nil,
		&profileID,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
	)

	if got.PrimaryExecutorProfileID == nil || *got.PrimaryExecutorProfileID != profileID {
		t.Fatalf("PrimaryExecutorProfileID = %v, want %q", got.PrimaryExecutorProfileID, profileID)
	}
}

func TestFromTaskUsesExecutorProfileIDMetadata(t *testing.T) {
	task := &models.Task{
		ID: "task-executor-profile-metadata",
		Metadata: map[string]interface{}{
			models.MetaKeyExecutorProfileID: "executor-profile-2",
		},
	}

	got := FromTask(task)
	if got.PrimaryExecutorProfileID == nil || *got.PrimaryExecutorProfileID != "executor-profile-2" {
		t.Fatalf("PrimaryExecutorProfileID = %v, want metadata value", got.PrimaryExecutorProfileID)
	}
}

func TestFromWorkflowStep_PreservesWIPFields(t *testing.T) {
	step := &wfmodels.WorkflowStep{
		ID:             "step-1",
		WorkflowID:     "wf-1",
		Name:           "Work",
		WIPLimit:       3,
		PullFromStepID: "queue-step",
	}

	got := FromWorkflowStep(step)

	if got.WIPLimit != 3 {
		t.Fatalf("WIPLimit = %d, want 3", got.WIPLimit)
	}
	if got.PullFromStepID != "queue-step" {
		t.Fatalf("PullFromStepID = %q, want queue-step", got.PullFromStepID)
	}
}

func TestFromWorkflowStep_PreservesCancelTriggersTurnComplete(t *testing.T) {
	got := FromWorkflowStep(&wfmodels.WorkflowStep{
		ID:                         "step-cancel",
		WorkflowID:                 "wf-1",
		CancelTriggersTurnComplete: true,
	})
	payload, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("marshal workflow step DTO: %v", err)
	}
	var fields map[string]any
	if err := json.Unmarshal(payload, &fields); err != nil {
		t.Fatalf("decode workflow step DTO: %v", err)
	}
	if gotValue, ok := fields["cancel_triggers_turn_complete"].(bool); !ok || !gotValue {
		t.Fatalf("cancel trigger DTO field = %#v, want true", fields["cancel_triggers_turn_complete"])
	}
}

func TestFromTaskSession_IncludesAllWorktrees(t *testing.T) {
	session := &models.TaskSession{
		ID:            "session-1",
		TaskID:        "task-1",
		WorkspacePath: "/task-root",
		Worktrees: []*models.TaskEnvironmentRepo{
			{ID: "assoc-1", WorktreeID: "wt-1", RepositoryID: "repo-a", WorktreePath: "/x/a"},
			{ID: "assoc-2", WorktreeID: "wt-2", RepositoryID: "repo-b", WorktreePath: "/x/b"},
		},
	}

	full := FromTaskSession(session)
	if len(full.Worktrees) != 2 {
		t.Fatalf("FromTaskSession Worktrees len = %d, want 2", len(full.Worktrees))
	}
	if full.WorktreePath != "/x/a" {
		t.Fatalf("WorktreePath = %q, want /x/a (first worktree)", full.WorktreePath)
	}
	if full.WorkspacePath != "/task-root" {
		t.Fatalf("WorkspacePath = %q, want /task-root", full.WorkspacePath)
	}

	summary := FromTaskSessionSummary(session)
	if len(summary.Worktrees) != 2 {
		t.Fatalf("FromTaskSessionSummary Worktrees len = %d, want 2", len(summary.Worktrees))
	}
	if summary.Worktrees[1]["worktree_id"] != "wt-2" {
		t.Fatalf("second worktree id = %v, want wt-2", summary.Worktrees[1]["worktree_id"])
	}
	if summary.Worktrees[0]["session_id"] != "session-1" {
		t.Fatalf("worktree session_id = %v, want session-1 (stable wire shape)", summary.Worktrees[0]["session_id"])
	}
	if summary.WorkspacePath != "/task-root" {
		t.Fatalf("summary WorkspacePath = %q, want /task-root", summary.WorkspacePath)
	}
}

// TestFromTaskSession_CopiesRollupColumns pins docs/specs/task-cost-ledger/
// spec.md AC-28/AC-29: TaskSessionDTO (the full session detail shape) must
// surface the four usage/cost rollup columns internal/task/usage's writer
// maintains on task_sessions.
func TestFromTaskSession_CopiesRollupColumns(t *testing.T) {
	session := &models.TaskSession{
		ID:             "session-1",
		TaskID:         "task-1",
		CostSubcents:   79118,
		TokensIn:       80,
		TokensCachedIn: 8203943,
		TokensOut:      44979,
	}

	got := FromTaskSession(session)
	if got.CostSubcents != 79118 || got.TokensIn != 80 || got.TokensCachedIn != 8203943 || got.TokensOut != 44979 {
		t.Fatalf("got rollup fields = %+v, want copied verbatim from session", got)
	}
}

// TestFromTaskSessionSummary_ExcludesRollupColumns pins the deliberate scope
// decision (task-cost-ledger plan #11): the cross-task summary projection is
// NOT widened by this card, only the full TaskSessionDTO is.
func TestFromTaskSessionSummary_ExcludesRollupColumns(t *testing.T) {
	session := &models.TaskSession{
		ID:             "session-1",
		TaskID:         "task-1",
		CostSubcents:   79118,
		TokensIn:       80,
		TokensCachedIn: 8203943,
		TokensOut:      44979,
	}

	got := FromTaskSessionSummary(session)
	raw, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded map[string]interface{}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, key := range []string{"cost_subcents", "tokens_in", "tokens_cached_in", "tokens_out"} {
		if _, ok := decoded[key]; ok {
			t.Errorf("TaskSessionSummaryDTO JSON contains %q, want it absent", key)
		}
	}
}

func TestFromTask_DerivesInterruptedFromMetadata(t *testing.T) {
	now := time.Now().UTC()

	t.Run("marked", func(t *testing.T) {
		task := &models.Task{
			ID:        "t1",
			CreatedAt: now,
			UpdatedAt: now,
			Metadata: map[string]interface{}{
				models.MetaKeyInterruptedAt: "2026-08-02T10:00:00Z",
			},
		}
		got := FromTask(task)
		if !got.Interrupted {
			t.Fatal("Interrupted = false, want true when interrupted_at metadata is present")
		}
	})

	t.Run("unmarked", func(t *testing.T) {
		task := &models.Task{ID: "t1", CreatedAt: now, UpdatedAt: now}
		got := FromTask(task)
		if got.Interrupted {
			t.Fatal("Interrupted = true, want false when interrupted_at metadata is absent")
		}
	})
}

func TestFromTaskRedactsDeferredLaunchAttribution(t *testing.T) {
	task := &models.Task{
		ID: "task-private-attribution",
		Metadata: map[string]interface{}{
			models.MetaKeyDeferredLaunch: map[string]interface{}{
				"intent":                                "start",
				"agent_profile_id":                      "profile-1",
				models.DeferredLaunchUserIDKey:          "user-private",
				models.DeferredLaunchRecordRecentUseKey: true,
			},
		},
	}

	got := FromTask(task)
	launch, ok := got.Metadata[models.MetaKeyDeferredLaunch].(map[string]interface{})
	if !ok {
		t.Fatalf("DTO deferred launch metadata = %#v, want a map", got.Metadata[models.MetaKeyDeferredLaunch])
	}
	if _, exists := launch[models.DeferredLaunchUserIDKey]; exists {
		t.Fatalf("DTO exposed deferred launch user_id = %v", launch[models.DeferredLaunchUserIDKey])
	}
	if _, exists := launch[models.DeferredLaunchRecordRecentUseKey]; exists {
		t.Fatalf("DTO exposed deferred launch recency marker = %v", launch[models.DeferredLaunchRecordRecentUseKey])
	}
	if got := task.Metadata[models.MetaKeyDeferredLaunch].(map[string]interface{})[models.DeferredLaunchUserIDKey]; got != "user-private" {
		t.Fatalf("redacting the DTO mutated the source task user_id = %v", got)
	}
	apiTask := task.ToAPI()
	apiLaunch, ok := apiTask.Metadata[models.MetaKeyDeferredLaunch].(map[string]interface{})
	if !ok {
		t.Fatalf("API deferred launch metadata = %#v, want a map", apiTask.Metadata[models.MetaKeyDeferredLaunch])
	}
	if _, exists := apiLaunch[models.DeferredLaunchUserIDKey]; exists {
		t.Fatalf("API exposed deferred launch user_id = %v", apiLaunch[models.DeferredLaunchUserIDKey])
	}
	if _, exists := apiLaunch[models.DeferredLaunchRecordRecentUseKey]; exists {
		t.Fatalf("API exposed deferred launch recency marker = %v", apiLaunch[models.DeferredLaunchRecordRecentUseKey])
	}
}

func TestTaskToAPI_DerivesInterruptedFromMetadata(t *testing.T) {
	now := time.Now().UTC()

	marked := (&models.Task{
		ID:        "t1",
		CreatedAt: now,
		UpdatedAt: now,
		Metadata:  map[string]interface{}{models.MetaKeyInterruptedAt: "2026-08-02T10:00:00Z"},
	}).ToAPI()
	if !marked.Interrupted {
		t.Fatal("ToAPI Interrupted = false, want true when interrupted_at metadata is present")
	}

	unmarked := (&models.Task{ID: "t1", CreatedAt: now, UpdatedAt: now}).ToAPI()
	if unmarked.Interrupted {
		t.Fatal("ToAPI Interrupted = true, want false when interrupted_at metadata is absent")
	}
}

func TestFromTurnPreservesSubsecondOrderingPrecision(t *testing.T) {
	base := time.Date(2026, time.August, 15, 12, 0, 0, 100000000, time.UTC)
	completedAt := base.Add(3 * time.Nanosecond)
	got := FromTurn(&models.Turn{
		ID:          "turn-nano",
		StartedAt:   base,
		CompletedAt: &completedAt,
		CreatedAt:   base.Add(time.Nanosecond),
		UpdatedAt:   base.Add(2 * time.Nanosecond),
	})

	if got.StartedAt != "2026-08-15T12:00:00.100000000Z" {
		t.Fatalf("StartedAt = %q, want nanosecond precision", got.StartedAt)
	}
	if got.CreatedAt != "2026-08-15T12:00:00.100000001Z" {
		t.Fatalf("CreatedAt = %q, want nanosecond precision", got.CreatedAt)
	}
	if got.UpdatedAt != "2026-08-15T12:00:00.100000002Z" {
		t.Fatalf("UpdatedAt = %q, want nanosecond precision", got.UpdatedAt)
	}
	if got.CompletedAt == nil || *got.CompletedAt != "2026-08-15T12:00:00.100000003Z" {
		t.Fatalf("CompletedAt = %v, want nanosecond precision", got.CompletedAt)
	}
	if got.StartedAt >= got.CreatedAt || got.CreatedAt >= got.UpdatedAt || got.UpdatedAt >= *got.CompletedAt {
		t.Fatalf("turn timestamps are not lexically chronological: %+v", got)
	}
	if _, err := time.Parse(time.RFC3339, got.StartedAt); err != nil {
		t.Fatalf("time.Parse(RFC3339, StartedAt): %v", err)
	}
}

// TestFromTurnPreservesLifecycleOnlyMetadata is AC-11's DTO half: the
// GET .../turns handler (internal/task/handlers/task_http_handlers.go) does
// nothing but call FromTurn per row and gin.Context.JSON the result, so
// pinning that FromTurn copies metadata verbatim - and that json.Marshal
// keeps the lifecycle_only key - proves the response payload carries it,
// without standing up an HTTP server. turnHistoryPredicate (proven unchanged
// by TestTurnReadsHideEmptyUnpublishedReservationUntilMessageEvidence and
// friends in the sqlite package) is what keeps this turn in the result set
// FromTurn is ever handed.
func TestFromTurnPreservesLifecycleOnlyMetadata(t *testing.T) {
	now := time.Date(2026, time.August, 24, 9, 0, 0, 0, time.UTC)
	got := FromTurn(&models.Turn{
		ID:        "turn-lifecycle",
		StartedAt: now,
		CreatedAt: now,
		UpdatedAt: now,
		Metadata:  map[string]interface{}{models.TurnMetaKeyLifecycleOnly: true},
	})

	if lifecycleOnly, ok := got.Metadata[models.TurnMetaKeyLifecycleOnly]; !ok || lifecycleOnly != true {
		t.Fatalf("TurnDTO.Metadata[%q] = %v, want true", models.TurnMetaKeyLifecycleOnly, lifecycleOnly)
	}

	raw, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("json.Marshal(TurnDTO): %v", err)
	}
	var decoded struct {
		Metadata map[string]interface{} `json:"metadata"`
	}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("json.Unmarshal(TurnDTO JSON): %v", err)
	}
	if lifecycleOnly, ok := decoded.Metadata[models.TurnMetaKeyLifecycleOnly]; !ok || lifecycleOnly != true {
		t.Fatalf("decoded metadata.lifecycle_only = %v, want true (raw JSON: %s)", lifecycleOnly, raw)
	}
}
