package repository

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/workflow/models"
)

func newPhase2TestStep(t *testing.T, repo *Repository, name string) *models.WorkflowStep {
	t.Helper()
	step := &models.WorkflowStep{
		WorkflowID: "wf-test",
		Name:       name,
		Position:   0,
		StageType:  models.StageTypeReview,
	}
	if err := repo.CreateStep(context.Background(), step); err != nil {
		t.Fatalf("create step: %v", err)
	}
	return step
}

func TestUpsertStepParticipant_InsertAndUpdate(t *testing.T) {
	repo := setupTestRepo(t)
	ctx := context.Background()
	step := newPhase2TestStep(t, repo, "Review")

	p := &models.WorkflowStepParticipant{
		StepID:           step.ID,
		Role:             models.ParticipantRoleReviewer,
		AgentProfileID:   "profile-alice",
		DecisionRequired: true,
		Position:         1,
	}
	if err := repo.UpsertStepParticipant(ctx, p); err != nil {
		t.Fatalf("upsert participant: %v", err)
	}
	if p.ID == "" {
		t.Fatalf("expected upsert to assign id")
	}

	// Update the same row.
	p.Position = 5
	p.AgentProfileID = "profile-bob"
	if err := repo.UpsertStepParticipant(ctx, p); err != nil {
		t.Fatalf("upsert participant (update): %v", err)
	}

	got, err := repo.GetStepParticipant(ctx, p.ID)
	if err != nil {
		t.Fatalf("get participant: %v", err)
	}
	if got.AgentProfileID != "profile-bob" || got.Position != 5 {
		t.Fatalf("unexpected updated participant: %+v", got)
	}
	if !got.DecisionRequired {
		t.Fatalf("decision_required should round-trip true")
	}
}

func TestUpsertStepParticipant_RejectsBadInput(t *testing.T) {
	repo := setupTestRepo(t)
	step := newPhase2TestStep(t, repo, "Review")

	cases := []struct {
		name string
		p    *models.WorkflowStepParticipant
	}{
		{"nil", nil},
		{"missing step", &models.WorkflowStepParticipant{Role: models.ParticipantRoleReviewer, AgentProfileID: "a"}},
		{"missing profile", &models.WorkflowStepParticipant{StepID: step.ID, Role: models.ParticipantRoleReviewer}},
		{"bad role", &models.WorkflowStepParticipant{StepID: step.ID, Role: "ceo", AgentProfileID: "a"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := repo.UpsertStepParticipant(context.Background(), tc.p); err == nil {
				t.Fatalf("expected error for %s", tc.name)
			}
		})
	}
}

func TestListStepParticipants_OrderedByRoleAndPosition(t *testing.T) {
	repo := setupTestRepo(t)
	ctx := context.Background()
	step := newPhase2TestStep(t, repo, "Review")

	mustUpsert := func(role models.ParticipantRole, profile string, pos int) {
		t.Helper()
		if err := repo.UpsertStepParticipant(ctx, &models.WorkflowStepParticipant{
			StepID: step.ID, Role: role, AgentProfileID: profile, Position: pos,
		}); err != nil {
			t.Fatalf("upsert: %v", err)
		}
	}
	mustUpsert(models.ParticipantRoleReviewer, "rev-2", 2)
	mustUpsert(models.ParticipantRoleApprover, "app-1", 0)
	mustUpsert(models.ParticipantRoleReviewer, "rev-1", 1)

	got, err := repo.ListStepParticipants(ctx, step.ID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 participants, got %d", len(got))
	}
	// Ordering: role ASC then position ASC. "approver" < "reviewer".
	if got[0].Role != models.ParticipantRoleApprover {
		t.Fatalf("expected approver first, got %s", got[0].Role)
	}
	if got[1].AgentProfileID != "rev-1" || got[2].AgentProfileID != "rev-2" {
		t.Fatalf("unexpected reviewer ordering: %s, %s", got[1].AgentProfileID, got[2].AgentProfileID)
	}
}

func TestListParticipantsForTaskAnyStep_ReturnsPerTaskRowsAcrossSteps(t *testing.T) {
	repo := setupTestRepo(t)
	ctx := context.Background()
	review := newPhase2TestStep(t, repo, "Review")
	approval := newPhase2TestStep(t, repo, "Approval")

	mustUpsert := func(stepID, taskID string, role models.ParticipantRole, profile string) {
		t.Helper()
		if err := repo.UpsertStepParticipant(ctx, &models.WorkflowStepParticipant{
			StepID: stepID, TaskID: taskID, Role: role, AgentProfileID: profile,
		}); err != nil {
			t.Fatalf("upsert: %v", err)
		}
	}
	mustUpsert(review.ID, "task-1", models.ParticipantRoleReviewer, "rev-A")
	mustUpsert(approval.ID, "task-1", models.ParticipantRoleApprover, "app-A")
	mustUpsert(review.ID, "", models.ParticipantRoleReviewer, "rev-template") // template row, not per-task
	mustUpsert(review.ID, "task-2", models.ParticipantRoleReviewer, "rev-B")  // different task

	got, err := repo.ListParticipantsForTaskAnyStep(ctx, "task-1")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 per-task rows across both steps, got %d: %+v", len(got), got)
	}
	steps := map[string]bool{}
	for _, p := range got {
		if p.TaskID != "task-1" {
			t.Fatalf("unexpected task_id in result: %+v", p)
		}
		steps[p.StepID] = true
	}
	if !steps[review.ID] || !steps[approval.ID] {
		t.Fatalf("expected rows from both review and approval steps, got %+v", got)
	}
}

func TestListParticipantsForTaskAnyStep_RejectsEmptyTaskID(t *testing.T) {
	repo := setupTestRepo(t)
	if _, err := repo.ListParticipantsForTaskAnyStep(context.Background(), ""); err == nil {
		t.Fatal("expected error for empty task_id")
	}
}

func TestListParticipantsForTaskWorkflow_ScopesRowsToActiveWorkflow(t *testing.T) {
	repo, db := setupTestRepoWithDB(t)
	ctx := context.Background()
	if _, err := db.Exec(`INSERT INTO workflows (id, workspace_id, name, created_at, updated_at)
		VALUES ('wf-other', '', 'Other', datetime('now'), datetime('now'))`); err != nil {
		t.Fatalf("insert second workflow: %v", err)
	}
	review := newPhase2TestStep(t, repo, "Review")
	other := &models.WorkflowStep{WorkflowID: "wf-other", Name: "Other review", Position: 0}
	if err := repo.CreateStep(ctx, other); err != nil {
		t.Fatalf("create other step: %v", err)
	}
	for _, p := range []*models.WorkflowStepParticipant{
		{StepID: review.ID, TaskID: "task-1", Role: models.ParticipantRoleReviewer, AgentProfileID: "active", DecisionRequired: true},
		{StepID: other.ID, TaskID: "task-1", Role: models.ParticipantRoleReviewer, AgentProfileID: "stale", DecisionRequired: true},
	} {
		if err := repo.UpsertStepParticipant(ctx, p); err != nil {
			t.Fatalf("upsert participant: %v", err)
		}
	}
	got, err := repo.ListParticipantsForTaskWorkflow(ctx, "task-1", "wf-test")
	if err != nil {
		t.Fatalf("list scoped participants: %v", err)
	}
	if len(got) != 1 || got[0].AgentProfileID != "active" {
		t.Fatalf("got scoped participants %+v, want only active row", got)
	}
}

func TestDeleteStepParticipant_RemovesRow(t *testing.T) {
	repo := setupTestRepo(t)
	ctx := context.Background()
	step := newPhase2TestStep(t, repo, "Review")
	p := &models.WorkflowStepParticipant{StepID: step.ID, Role: models.ParticipantRoleReviewer, AgentProfileID: "p"}
	if err := repo.UpsertStepParticipant(ctx, p); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	if err := repo.DeleteStepParticipant(ctx, p.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	rows, err := repo.ListStepParticipants(ctx, step.ID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("expected 0 participants after delete, got %d", len(rows))
	}

	if err := repo.DeleteStepParticipant(ctx, ""); err == nil {
		t.Fatalf("expected error deleting empty id")
	}
}

func TestEnsureRoleSeat_InsertsWhenAbsent(t *testing.T) {
	repo := setupTestRepo(t)
	ctx := context.Background()
	step := newPhase2TestStep(t, repo, "Review")

	seat, inserted, err := repo.EnsureRoleSeat(ctx, "wf-test", step.ID, "task-1", string(models.ParticipantRoleReviewer), "agent-a")
	if err != nil {
		t.Fatalf("ensure role seat: %v", err)
	}
	if !inserted {
		t.Fatalf("expected inserted=true for a fresh seat")
	}
	if seat.StepID != step.ID || seat.TaskID != "task-1" || seat.AgentProfileID != "agent-a" {
		t.Fatalf("unexpected seat: %+v", seat)
	}
	if seat.Role != models.ParticipantRoleReviewer {
		t.Fatalf("expected role reviewer, got %q", seat.Role)
	}
	if !seat.DecisionRequired {
		t.Fatalf("expected a newly ensured seat to be decision_required")
	}
	if seat.Position != 0 {
		t.Fatalf("expected position 0, got %d", seat.Position)
	}
	if seat.CreatedAt.IsZero() {
		t.Fatalf("expected created_at to be set")
	}

	got, err := repo.GetStepParticipant(ctx, seat.ID)
	if err != nil {
		t.Fatalf("get participant: %v", err)
	}
	if got.AgentProfileID != "agent-a" {
		t.Fatalf("seat not persisted: %+v", got)
	}
}

func TestEnsureRoleSeat_StampsAutoProvenance(t *testing.T) {
	repo := setupTestRepo(t)
	ctx := context.Background()
	step := newPhase2TestStep(t, repo, "Review")

	seat, _, err := repo.EnsureRoleSeat(ctx, "wf-test", step.ID, "task-1", string(models.ParticipantRoleReviewer), "agent-a")
	if err != nil {
		t.Fatalf("ensure role seat: %v", err)
	}
	if seat.Provenance != models.ParticipantProvenanceAuto {
		t.Fatalf("expected provenance %q, got %q", models.ParticipantProvenanceAuto, seat.Provenance)
	}

	got, err := repo.GetStepParticipant(ctx, seat.ID)
	if err != nil {
		t.Fatalf("get participant: %v", err)
	}
	if got.Provenance != models.ParticipantProvenanceAuto {
		t.Fatalf("persisted provenance = %q, want %q", got.Provenance, models.ParticipantProvenanceAuto)
	}
}

func TestEnsureRoleSeat_NoOpWhenSeatExistsAtAnyStepInWorkflow(t *testing.T) {
	repo := setupTestRepo(t)
	ctx := context.Background()
	review := newPhase2TestStep(t, repo, "Review")
	done := newPhase2TestStep(t, repo, "Done")

	first, inserted, err := repo.EnsureRoleSeat(ctx, "wf-test", review.ID, "task-1", string(models.ParticipantRoleReviewer), "agent-a")
	if err != nil {
		t.Fatalf("ensure role seat (first): %v", err)
	}
	if !inserted {
		t.Fatalf("expected first call to insert")
	}

	// Entering a later step in the same workflow, for the same task and
	// role, must observe the seat already placed at Review and write
	// nothing new — even though the seat lives at a different step than
	// the one now being entered (AC-OFFICE-REVIEW-SEATS-001.5, -003.5).
	second, inserted, err := repo.EnsureRoleSeat(ctx, "wf-test", done.ID, "task-1", string(models.ParticipantRoleReviewer), "agent-b")
	if err != nil {
		t.Fatalf("ensure role seat (second): %v", err)
	}
	if inserted {
		t.Fatalf("expected second call to be a no-op, got inserted=true")
	}
	if second.ID != first.ID || second.StepID != review.ID || second.AgentProfileID != "agent-a" {
		t.Fatalf("expected the pre-existing seat back unchanged, got %+v", second)
	}

	rows, err := repo.ListParticipantsForTaskWorkflow(ctx, "task-1", "wf-test")
	if err != nil {
		t.Fatalf("list workflow participants: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected exactly one seat for the role in the workflow, got %d: %+v", len(rows), rows)
	}
}

func TestEnsureRoleSeat_ScopedToWorkflowNotOtherWorkflows(t *testing.T) {
	repo, db := setupTestRepoWithDB(t)
	ctx := context.Background()
	if _, err := db.Exec(`INSERT INTO workflows (id, workspace_id, name, created_at, updated_at)
		VALUES ('wf-other', '', 'Other', datetime('now'), datetime('now'))`); err != nil {
		t.Fatalf("insert second workflow: %v", err)
	}
	stepA := newPhase2TestStep(t, repo, "Review")
	stepB := &models.WorkflowStep{WorkflowID: "wf-other", Name: "Review", Position: 0}
	if err := repo.CreateStep(ctx, stepB); err != nil {
		t.Fatalf("create step in other workflow: %v", err)
	}

	if _, inserted, err := repo.EnsureRoleSeat(ctx, "wf-test", stepA.ID, "task-1", string(models.ParticipantRoleReviewer), "agent-a"); err != nil || !inserted {
		t.Fatalf("ensure role seat in wf-test: inserted=%v err=%v", inserted, err)
	}

	// A seat in a different workflow for the same task+role must not
	// suppress a fresh seat here — the existence check is workflow-scoped.
	seat, inserted, err := repo.EnsureRoleSeat(ctx, "wf-other", stepB.ID, "task-1", string(models.ParticipantRoleReviewer), "agent-b")
	if err != nil {
		t.Fatalf("ensure role seat in wf-other: %v", err)
	}
	if !inserted {
		t.Fatalf("expected a new seat in the other workflow, got no-op")
	}
	if seat.StepID != stepB.ID || seat.AgentProfileID != "agent-b" {
		t.Fatalf("unexpected seat: %+v", seat)
	}
}

func TestEnsureRoleSeat_RejectsBadInput(t *testing.T) {
	repo := setupTestRepo(t)
	ctx := context.Background()
	step := newPhase2TestStep(t, repo, "Review")

	cases := []struct {
		name                                             string
		workflowID, stepID, taskID, role, agentProfileID string
	}{
		{"empty workflow", "", step.ID, "task-1", string(models.ParticipantRoleReviewer), "agent-a"},
		{"empty step", "wf-test", "", "task-1", string(models.ParticipantRoleReviewer), "agent-a"},
		{"empty task", "wf-test", step.ID, "", string(models.ParticipantRoleReviewer), "agent-a"},
		{"empty agent", "wf-test", step.ID, "task-1", string(models.ParticipantRoleReviewer), ""},
		{"invalid role", "wf-test", step.ID, "task-1", "not-a-role", "agent-a"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, _, err := repo.EnsureRoleSeat(ctx, tc.workflowID, tc.stepID, tc.taskID, tc.role, tc.agentProfileID); err == nil {
				t.Fatalf("expected error for %s", tc.name)
			}
		})
	}
}

func TestEnsureRoleSeat_ConcurrentEntriesConvergeOnOneSeat(t *testing.T) {
	repo := setupTestRepo(t)
	ctx := context.Background()
	review := newPhase2TestStep(t, repo, "Review")
	done := newPhase2TestStep(t, repo, "Done")

	// SQLite's single-writer lock serializes these transactions, so this
	// exercises EnsureRoleSeat's retry-on-natural-key-violation path
	// (rather than true concurrent execution, which the Postgres-gated
	// counterpart covers) whenever both check-then-insert attempts race
	// into the same natural key.
	var wg sync.WaitGroup
	results := make([]*models.WorkflowStepParticipant, 2)
	errs := make([]error, 2)
	steps := []*models.WorkflowStep{review, done}
	agents := []string{"agent-a", "agent-b"}
	for i := range 2 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			seat, _, err := repo.EnsureRoleSeat(ctx, "wf-test", steps[i].ID, "task-race", string(models.ParticipantRoleReviewer), agents[i])
			results[i] = seat
			errs[i] = err
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("ensure role seat[%d]: %v", i, err)
		}
	}
	if results[0].ID != results[1].ID {
		t.Fatalf("expected both callers to converge on the same seat id, got %q and %q", results[0].ID, results[1].ID)
	}

	rows, err := repo.ListParticipantsForTaskWorkflow(ctx, "task-race", "wf-test")
	if err != nil {
		t.Fatalf("list workflow participants: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected exactly one seat to survive the race, got %d: %+v", len(rows), rows)
	}
}

func TestHasRoleSeatForTaskWorkflow(t *testing.T) {
	repo := setupTestRepo(t)
	ctx := context.Background()
	step := newPhase2TestStep(t, repo, "Review")

	has, err := repo.HasRoleSeatForTaskWorkflow(ctx, "wf-test", "task-1", string(models.ParticipantRoleReviewer))
	if err != nil {
		t.Fatalf("has role seat: %v", err)
	}
	if has {
		t.Fatalf("expected no seat before any is written")
	}

	if _, _, err := repo.EnsureRoleSeat(ctx, "wf-test", step.ID, "task-1", string(models.ParticipantRoleReviewer), "agent-a"); err != nil {
		t.Fatalf("ensure role seat: %v", err)
	}

	has, err = repo.HasRoleSeatForTaskWorkflow(ctx, "wf-test", "task-1", string(models.ParticipantRoleReviewer))
	if err != nil {
		t.Fatalf("has role seat: %v", err)
	}
	if !has {
		t.Fatalf("expected a seat after EnsureRoleSeat")
	}

	has, err = repo.HasRoleSeatForTaskWorkflow(ctx, "wf-test", "task-1", string(models.ParticipantRoleApprover))
	if err != nil {
		t.Fatalf("has role seat (other role): %v", err)
	}
	if has {
		t.Fatalf("expected no seat for a different role")
	}

	if _, err := repo.HasRoleSeatForTaskWorkflow(ctx, "", "task-1", string(models.ParticipantRoleReviewer)); err == nil {
		t.Fatalf("expected error for empty workflow id")
	}
}

func TestRecordStepDecision_AndList(t *testing.T) {
	repo := setupTestRepo(t)
	ctx := context.Background()
	step := newPhase2TestStep(t, repo, "Review")
	p := &models.WorkflowStepParticipant{StepID: step.ID, Role: models.ParticipantRoleReviewer, AgentProfileID: "p"}
	if err := repo.UpsertStepParticipant(ctx, p); err != nil {
		t.Fatalf("upsert participant: %v", err)
	}

	now := time.Now().UTC().Truncate(time.Millisecond)
	d := &models.WorkflowStepDecision{
		TaskID:        "task-1",
		StepID:        step.ID,
		ParticipantID: p.ID,
		Decision:      "approved",
		Note:          "looks good",
		DecidedAt:     now,
	}
	if err := repo.RecordStepDecision(ctx, d); err != nil {
		t.Fatalf("record decision: %v", err)
	}
	if d.ID == "" {
		t.Fatalf("expected decision id to be assigned")
	}

	got, err := repo.ListStepDecisions(ctx, "task-1", step.ID)
	if err != nil {
		t.Fatalf("list decisions: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 decision, got %d", len(got))
	}
	if got[0].Decision != "approved" || got[0].Note != "looks good" {
		t.Fatalf("decision did not round-trip: %+v", got[0])
	}
}

func TestRecordStepDecision_DefaultsDecidedAtAndID(t *testing.T) {
	repo := setupTestRepo(t)
	ctx := context.Background()
	step := newPhase2TestStep(t, repo, "Review")
	d := &models.WorkflowStepDecision{
		TaskID:        "task-1",
		StepID:        step.ID,
		ParticipantID: "anyone",
		Decision:      "rejected",
	}
	if err := repo.RecordStepDecision(ctx, d); err != nil {
		t.Fatalf("record: %v", err)
	}
	if d.ID == "" {
		t.Fatalf("expected id generation")
	}
	if d.DecidedAt.IsZero() {
		t.Fatalf("expected decided_at default")
	}
}

func TestRecordStepDecision_RejectsBadInput(t *testing.T) {
	repo := setupTestRepo(t)
	ctx := context.Background()
	cases := []*models.WorkflowStepDecision{
		nil,
		{StepID: "s", ParticipantID: "p", Decision: "approved"}, // missing task_id
		{TaskID: "t", ParticipantID: "p", Decision: "approved"}, // missing step_id
		{TaskID: "t", StepID: "s", Decision: "approved"},        // missing participant_id
		{TaskID: "t", StepID: "s", ParticipantID: "p"},          // missing decision
	}
	for i, d := range cases {
		if err := repo.RecordStepDecision(ctx, d); err == nil {
			t.Fatalf("case %d: expected error", i)
		}
	}
}

func TestClearStepDecisions_RemovesRowsForPair(t *testing.T) {
	repo := setupTestRepo(t)
	ctx := context.Background()
	step := newPhase2TestStep(t, repo, "Review")

	mustRecord := func(taskID, decision string) {
		t.Helper()
		if err := repo.RecordStepDecision(ctx, &models.WorkflowStepDecision{
			TaskID: taskID, StepID: step.ID, ParticipantID: "p", Decision: decision,
		}); err != nil {
			t.Fatalf("record: %v", err)
		}
	}
	mustRecord("task-1", "approved")
	mustRecord("task-1", "rejected")
	mustRecord("task-2", "approved")

	rows, err := repo.ClearStepDecisions(ctx, "task-1", step.ID)
	if err != nil {
		t.Fatalf("clear: %v", err)
	}
	if rows != 2 {
		t.Fatalf("expected 2 rows cleared, got %d", rows)
	}
	remaining, _ := repo.ListStepDecisions(ctx, "task-1", step.ID)
	if len(remaining) != 0 {
		t.Fatalf("task-1 decisions should be empty after clear, got %d", len(remaining))
	}
	other, _ := repo.ListStepDecisions(ctx, "task-2", step.ID)
	if len(other) != 1 {
		t.Fatalf("task-2 decisions should be untouched, got %d", len(other))
	}
}

func TestClearStepDecisions_RejectsEmptyKey(t *testing.T) {
	repo := setupTestRepo(t)
	if _, err := repo.ClearStepDecisions(context.Background(), "", "s"); err == nil {
		t.Fatalf("expected error for empty task_id")
	}
	if _, err := repo.ClearStepDecisions(context.Background(), "t", ""); err == nil {
		t.Fatalf("expected error for empty step_id")
	}
}

// TestRecordStepDecision_SupersedesPriorByDeciderRole verifies the ADR 0005
// Wave D office-style supersede semantics: a second RecordStepDecision with
// the same (task, step, decider_id, role) marks the prior row superseded
// rather than producing two active rows.
func TestRecordStepDecision_SupersedesPriorByDeciderRole(t *testing.T) {
	repo := setupTestRepo(t)
	ctx := context.Background()
	step := newPhase2TestStep(t, repo, "Review")

	first := &models.WorkflowStepDecision{
		TaskID: "t-supersede", StepID: step.ID, ParticipantID: "p1",
		Decision: "changes_requested", Note: "needs work",
		DeciderType: "agent", DeciderID: "alice", Role: "reviewer",
		Comment: "needs work",
	}
	if err := repo.RecordStepDecision(ctx, first); err != nil {
		t.Fatalf("record first: %v", err)
	}
	second := &models.WorkflowStepDecision{
		TaskID: "t-supersede", StepID: step.ID, ParticipantID: "p1",
		Decision:    "approved",
		DeciderType: "agent", DeciderID: "alice", Role: "reviewer",
	}
	if err := repo.RecordStepDecision(ctx, second); err != nil {
		t.Fatalf("record second: %v", err)
	}

	all, err := repo.ListStepDecisions(ctx, "t-supersede", step.ID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("expected 2 rows (one superseded), got %d", len(all))
	}
	supersededCount := 0
	activeCount := 0
	for _, d := range all {
		if d.SupersededAt != nil {
			supersededCount++
		} else {
			activeCount++
		}
	}
	if supersededCount != 1 || activeCount != 1 {
		t.Fatalf("expected 1 superseded + 1 active, got superseded=%d active=%d",
			supersededCount, activeCount)
	}

	active, err := repo.ListActiveTaskDecisions(ctx, "t-supersede")
	if err != nil {
		t.Fatalf("list active: %v", err)
	}
	if len(active) != 1 {
		t.Fatalf("expected 1 active, got %d", len(active))
	}
	if active[0].Decision != "approved" {
		t.Fatalf("active decision should be the latest 'approved', got %q", active[0].Decision)
	}
}

// TestIsDecisionActiveDeciderViolation_MatchesSQLiteConstraintMessage forces
// the exact constraint violation decisionActiveDeciderIndexName exists to
// catch — a second active row for the same (task, step, decider, role) —
// by inserting it directly, bypassing RecordStepDecision's own supersede
// step (which would otherwise correctly find and supersede the first row,
// never reaching the index). This is what a writer that lost the AC-27/29
// race produces, and confirms the SQLite branch of the classifier
// RecordStepDecision's retry loop depends on actually recognizes it.
func TestIsDecisionActiveDeciderViolation_MatchesSQLiteConstraintMessage(t *testing.T) {
	repo := setupTestRepo(t)
	ctx := context.Background()
	step := newPhase2TestStep(t, repo, "Review")

	first := &models.WorkflowStepDecision{
		TaskID: "t-race-classify", StepID: step.ID, ParticipantID: "p1",
		Decision: "approved", DeciderType: "agent", DeciderID: "alice", Role: "reviewer",
	}
	if err := repo.RecordStepDecision(ctx, first); err != nil {
		t.Fatalf("record first: %v", err)
	}

	_, err := repo.db.Exec(repo.db.Rebind(`
		INSERT INTO workflow_step_decisions
			(id, task_id, step_id, participant_id, decision, decided_at, decider_type, decider_id, role)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`), "manual-conflict-id", "t-race-classify", step.ID, "p1", "approved", time.Now().UTC(), "agent", "alice", "reviewer")
	if !isDecisionActiveDeciderViolation(err) {
		t.Fatalf("expected isDecisionActiveDeciderViolation to recognize this constraint violation, got err=%v", err)
	}
}

// TestInitPhase2Schema_DedupesPreExistingDuplicateActiveDecisionsBeforeUniqueIndex
// reproduces an existing install that hit the pre-fix AC-27/29 concurrent
// double-insert race (commit 73226d29b): two active rows already share the
// same (task_id, step_id, decider_id, role) identity by the time the new
// uniq_workflow_step_decisions_active_decider index is introduced. Without
// dedupeActiveStepDecisionsBeforeUniqueIndex running first,
// CREATE UNIQUE INDEX fails outright and aborts backend startup. This drops
// the index the initial setupTestRepo call already created, reinserts the
// duplicate-row scenario directly (bypassing RecordStepDecision's own
// supersede step, matching the sibling classifier test above), and reruns
// initPhase2Schema to prove it is safe to run again against a database that
// already has the conflicting rows.
func TestInitPhase2Schema_DedupesPreExistingDuplicateActiveDecisionsBeforeUniqueIndex(t *testing.T) {
	repo := setupTestRepo(t)
	ctx := context.Background()
	step := newPhase2TestStep(t, repo, "Review")

	if _, err := repo.db.Exec(`DROP INDEX IF EXISTS ` + decisionActiveDeciderIndexName); err != nil {
		t.Fatalf("drop unique index to simulate a pre-fix install: %v", err)
	}

	older := time.Now().UTC().Add(-time.Hour).Truncate(time.Millisecond)
	newer := time.Now().UTC().Truncate(time.Millisecond)
	insertActive := func(id, taskID, deciderID, role string, decidedAt time.Time) {
		t.Helper()
		if _, err := repo.db.Exec(repo.db.Rebind(`
			INSERT INTO workflow_step_decisions
				(id, task_id, step_id, participant_id, decision, decided_at, decider_type, decider_id, role)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		`), id, taskID, step.ID, "p1", "approved", decidedAt, "agent", deciderID, role); err != nil {
			t.Fatalf("insert duplicate active decision %s: %v", id, err)
		}
	}
	// Duplicate group: same (task_id, step_id, decider_id, role) identity,
	// both active (superseded_at NULL) — the exact shape the pre-fix race left
	// behind.
	insertActive("dup-older", "t-dedupe", "alice", "reviewer", older)
	insertActive("dup-newer", "t-dedupe", "alice", "reviewer", newer)
	// A distinct decider's active row must survive untouched.
	insertActive("solo-active", "t-dedupe", "bob", "reviewer", newer)

	if err := repo.initPhase2Schema(); err != nil {
		t.Fatalf("initPhase2Schema did not tolerate pre-existing duplicate active decisions: %v", err)
	}

	active, err := repo.ListActiveTaskDecisions(ctx, "t-dedupe")
	if err != nil {
		t.Fatalf("list active: %v", err)
	}
	if len(active) != 2 {
		t.Fatalf("expected 2 active decisions after dedupe (1 per decider), got %d: %+v", len(active), active)
	}
	byID := map[string]*models.WorkflowStepDecision{}
	for _, d := range active {
		byID[d.ID] = d
	}
	if byID["dup-newer"] == nil {
		t.Fatalf("expected the newest duplicate (dup-newer) to remain active, got %+v", active)
	}
	if byID["solo-active"] == nil {
		t.Fatalf("expected the unrelated decider's active row (solo-active) to remain untouched, got %+v", active)
	}
	if byID["dup-older"] != nil {
		t.Fatalf("expected the older duplicate (dup-older) to be superseded, but it is still active")
	}

	// The index must now genuinely be enforcing uniqueness again: a fresh
	// conflicting insert should fail the same way it would on a database that
	// never had duplicates.
	_, err = repo.db.Exec(repo.db.Rebind(`
		INSERT INTO workflow_step_decisions
			(id, task_id, step_id, participant_id, decision, decided_at, decider_type, decider_id, role)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`), "post-dedupe-conflict", "t-dedupe", step.ID, "p1", "approved", time.Now().UTC(), "agent", "alice", "reviewer")
	if !isDecisionActiveDeciderViolation(err) {
		t.Fatalf("expected the recreated unique index to reject a new conflicting active row, got err=%v", err)
	}
}

// TestInitPhase2Schema_DedupesPreExistingDuplicateParticipantsBeforeUniqueIndex
// mirrors TestInitPhase2Schema_DedupesPreExistingDuplicateActiveDecisionsBeforeUniqueIndex
// above for the new participantsNaturalKeyIndexName: an install that predates
// the unique (step_id, task_id, role, agent_profile_id) index can already
// hold more than one row sharing that identity (no DB-level guard existed
// before this index), and CREATE UNIQUE INDEX over them fails outright,
// aborting backend startup. Drops the index the initial setupTestRepo call
// already created, reinserts a duplicate-identity scenario directly
// (bypassing UpsertStepParticipant's own upsert-by-id path), and reruns
// initPhase2Schema to prove it tolerates and repairs a database that already
// has the conflicting rows.
func TestInitPhase2Schema_DedupesPreExistingDuplicateParticipantsBeforeUniqueIndex(t *testing.T) {
	repo := setupTestRepo(t)
	ctx := context.Background()
	step := newPhase2TestStep(t, repo, "Review")

	if _, err := repo.db.Exec(`DROP INDEX IF EXISTS ` + participantsNaturalKeyIndexName); err != nil {
		t.Fatalf("drop unique index to simulate a pre-fix install: %v", err)
	}

	insertRow := func(id string, position int) {
		t.Helper()
		if _, err := repo.db.ExecContext(ctx, repo.db.Rebind(`
			INSERT INTO workflow_step_participants
				(id, step_id, task_id, role, agent_profile_id, decision_required, position, created_at)
			VALUES (?, ?, '', 'reviewer', 'profile-dup', 0, ?, ?)
		`), id, step.ID, position, time.Now().UTC()); err != nil {
			t.Fatalf("insert duplicate participant %s: %v", id, err)
		}
	}
	// Duplicate group: same (step_id, task_id, role, agent_profile_id)
	// identity — the exact shape a pre-index install could accumulate.
	insertRow("dup-higher-position", 5)
	insertRow("dup-lower-position", 1)
	// A distinct agent_profile_id's row must survive untouched.
	if err := repo.UpsertStepParticipant(ctx, &models.WorkflowStepParticipant{
		StepID: step.ID, Role: models.ParticipantRoleReviewer, AgentProfileID: "profile-solo",
	}); err != nil {
		t.Fatalf("upsert solo participant: %v", err)
	}

	// A decision recorded against the losing duplicate must not be orphaned
	// by the delete below — it should be remapped onto the surviving row so
	// mapDecisionsToSeats can still find it.
	strandedDecision := &models.WorkflowStepDecision{
		ID: "stranded-decision", TaskID: "t-dedupe-participants", StepID: step.ID,
		ParticipantID: "dup-higher-position",
		Decision:      "approved", DecidedAt: time.Now().UTC(),
		DeciderType: "human", DeciderID: "carol", Role: "reviewer",
	}
	if err := repo.RecordStepDecision(ctx, strandedDecision); err != nil {
		t.Fatalf("record decision against duplicate participant: %v", err)
	}

	if err := repo.initPhase2Schema(); err != nil {
		t.Fatalf("initPhase2Schema did not tolerate pre-existing duplicate participants: %v", err)
	}

	got, err := repo.ListStepParticipants(ctx, step.ID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 participants after dedupe (1 per agent_profile_id), got %d: %+v", len(got), got)
	}
	byID := map[string]*models.WorkflowStepParticipant{}
	for _, p := range got {
		byID[p.ID] = p
	}
	if byID["dup-lower-position"] == nil {
		t.Fatalf("expected the lowest-position duplicate (dup-lower-position) to survive, got %+v", got)
	}
	if byID["dup-higher-position"] != nil {
		t.Fatalf("expected the higher-position duplicate to be deleted, but it is still present")
	}
	if byID["profile-solo"] == nil && findParticipantByAgent(got, "profile-solo") == nil {
		t.Fatalf("expected the unrelated agent's row (profile-solo) to remain untouched, got %+v", got)
	}

	remapped, err := repo.ListStepDecisions(ctx, "t-dedupe-participants", step.ID)
	if err != nil {
		t.Fatalf("list decisions: %v", err)
	}
	if len(remapped) != 1 {
		t.Fatalf("expected the stranded decision to survive the dedupe, got %d: %+v", len(remapped), remapped)
	}
	if remapped[0].ParticipantID != "dup-lower-position" {
		t.Fatalf("expected the decision to be remapped onto the surviving participant dup-lower-position, got %q",
			remapped[0].ParticipantID)
	}

	// The index must now genuinely be enforcing uniqueness: a fresh
	// conflicting insert should fail the same way it would on a database
	// that never had duplicates.
	_, err = repo.db.ExecContext(ctx, repo.db.Rebind(`
		INSERT INTO workflow_step_participants
			(id, step_id, task_id, role, agent_profile_id, decision_required, position, created_at)
		VALUES (?, ?, '', 'reviewer', 'profile-dup', 0, 9, ?)
	`), "post-dedupe-conflict", step.ID, time.Now().UTC())
	if err == nil {
		t.Fatalf("expected the recreated unique index to reject a new conflicting participant row")
	}
}

func findParticipantByAgent(rows []*models.WorkflowStepParticipant, agentProfileID string) *models.WorkflowStepParticipant {
	for _, p := range rows {
		if p.AgentProfileID == agentProfileID {
			return p
		}
	}
	return nil
}

// TestRecordStepDecision_DifferentDecidersIndependent verifies decisions
// recorded by distinct deciders coexist as separate active rows.
func TestRecordStepDecision_DifferentDecidersIndependent(t *testing.T) {
	repo := setupTestRepo(t)
	ctx := context.Background()
	step := newPhase2TestStep(t, repo, "Review")

	a := &models.WorkflowStepDecision{
		TaskID: "t-diff", StepID: step.ID, ParticipantID: "pa",
		Decision:    "approved",
		DeciderType: "agent", DeciderID: "alice", Role: "reviewer",
	}
	if err := repo.RecordStepDecision(ctx, a); err != nil {
		t.Fatalf("record alice: %v", err)
	}
	b := &models.WorkflowStepDecision{
		TaskID: "t-diff", StepID: step.ID, ParticipantID: "pb",
		Decision:    "changes_requested",
		DeciderType: "agent", DeciderID: "bob", Role: "reviewer",
	}
	if err := repo.RecordStepDecision(ctx, b); err != nil {
		t.Fatalf("record bob: %v", err)
	}

	active, err := repo.ListActiveTaskDecisions(ctx, "t-diff")
	if err != nil {
		t.Fatalf("list active: %v", err)
	}
	if len(active) != 2 {
		t.Fatalf("expected 2 active decisions, got %d", len(active))
	}
}

// TestSupersedeTaskDecisions verifies SupersedeTaskDecisions clears every
// active row for a task across all steps.
func TestSupersedeTaskDecisions(t *testing.T) {
	repo := setupTestRepo(t)
	ctx := context.Background()
	step := newPhase2TestStep(t, repo, "Review")

	for i, decider := range []string{"alice", "bob", "carol"} {
		d := &models.WorkflowStepDecision{
			TaskID: "t-rework", StepID: step.ID, ParticipantID: "p" + decider,
			Decision:    "approved",
			DeciderType: "agent", DeciderID: decider, Role: "reviewer",
		}
		if err := repo.RecordStepDecision(ctx, d); err != nil {
			t.Fatalf("record %d: %v", i, err)
		}
	}

	if err := repo.SupersedeTaskDecisions(ctx, "t-rework"); err != nil {
		t.Fatalf("supersede: %v", err)
	}
	active, err := repo.ListActiveTaskDecisions(ctx, "t-rework")
	if err != nil {
		t.Fatalf("list active: %v", err)
	}
	if len(active) != 0 {
		t.Fatalf("expected 0 active after supersede, got %d", len(active))
	}
	all, _ := repo.ListStepDecisions(ctx, "t-rework", step.ID)
	if len(all) != 3 {
		t.Fatalf("expected all 3 rows preserved (history), got %d", len(all))
	}
}

// TestSupersedeTaskDecisions_RejectsEmptyTaskID covers the input validation.
func TestSupersedeTaskDecisions_RejectsEmptyTaskID(t *testing.T) {
	repo := setupTestRepo(t)
	if err := repo.SupersedeTaskDecisions(context.Background(), ""); err == nil {
		t.Fatalf("expected error for empty task_id")
	}
}

// TestListActiveTaskDecisions_RejectsEmptyTaskID covers the input validation.
func TestListActiveTaskDecisions_RejectsEmptyTaskID(t *testing.T) {
	repo := setupTestRepo(t)
	if _, err := repo.ListActiveTaskDecisions(context.Background(), ""); err == nil {
		t.Fatalf("expected error for empty task_id")
	}
}

// TestResolveCurrentRunner_FallsBackToStepPrimary verifies the resolver
// returns the workflow step's primary agent_profile_id when no runner
// participant exists for the (step, task) pair. This is the kanban-style
// default after ADR 0005 Wave D drops tasks.assignee_agent_profile_id.
func TestResolveCurrentRunner_FallsBackToStepPrimary(t *testing.T) {
	repo := setupTestRepo(t)
	ctx := context.Background()
	step := &models.WorkflowStep{
		WorkflowID: "wf-test", Name: "Work", Position: 0,
		AgentProfileID: "primary-agent",
	}
	if err := repo.CreateStep(ctx, step); err != nil {
		t.Fatalf("create step: %v", err)
	}
	got, err := repo.ResolveCurrentRunner(ctx, step.ID, "task-1")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if got != "primary-agent" {
		t.Fatalf("expected primary-agent, got %q", got)
	}
}

// TestResolveCurrentRunner_PrefersRunnerParticipant verifies that a
// runner participant for the (step, task) pair takes precedence over
// the step's primary agent.
func TestResolveCurrentRunner_PrefersRunnerParticipant(t *testing.T) {
	repo := setupTestRepo(t)
	ctx := context.Background()
	step := &models.WorkflowStep{
		WorkflowID: "wf-test", Name: "Work", Position: 0,
		AgentProfileID: "primary-agent",
	}
	if err := repo.CreateStep(ctx, step); err != nil {
		t.Fatalf("create step: %v", err)
	}
	if err := repo.SetTaskRunner(ctx, step.ID, "task-r", "runner-agent"); err != nil {
		t.Fatalf("set runner: %v", err)
	}
	got, err := repo.ResolveCurrentRunner(ctx, step.ID, "task-r")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if got != "runner-agent" {
		t.Fatalf("expected runner-agent, got %q", got)
	}
}

func TestResolveCurrentRunner_FallsBackToTaskRunnerOnOtherStep(t *testing.T) {
	repo := setupTestRepo(t)
	ctx := context.Background()
	work := newPhase2TestStep(t, repo, "Work")
	done := newPhase2TestStep(t, repo, "Done")

	if err := repo.SetTaskRunner(ctx, work.ID, "task-done", "runner-agent"); err != nil {
		t.Fatalf("set runner: %v", err)
	}
	got, err := repo.ResolveCurrentRunner(ctx, done.ID, "task-done")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if got != "runner-agent" {
		t.Fatalf("expected runner-agent, got %q", got)
	}
}

func TestResolveCurrentRunner_FallsBackToLatestTaskRunner(t *testing.T) {
	repo := setupTestRepo(t)
	ctx := context.Background()
	work := newPhase2TestStep(t, repo, "Work")
	review := newPhase2TestStep(t, repo, "Review")
	done := newPhase2TestStep(t, repo, "Done")

	if err := repo.SetTaskRunner(ctx, work.ID, "task-done", "runner-on-work"); err != nil {
		t.Fatalf("set work runner: %v", err)
	}
	if err := repo.SetTaskRunner(ctx, review.ID, "task-done", "runner-on-review"); err != nil {
		t.Fatalf("set review runner: %v", err)
	}
	got, err := repo.ResolveCurrentRunner(ctx, done.ID, "task-done")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if got != "runner-on-review" {
		t.Fatalf("expected runner-on-review, got %q", got)
	}
}

// TestResolveCurrentRunner_LatestTaskRunnerOrdersByCreatedAtNotInsertionOrder
// pins the AC fix for the third tier's Postgres-incompatible `ORDER BY rowid
// DESC` (rowid is SQLite-only and has no Postgres equivalent). The fix
// reorders by `created_at DESC, agent_profile_id ASC` instead. This test
// decouples row-insertion order from created_at order — the row inserted
// LAST has the EARLIEST created_at — so a lingering rowid/insertion-order
// dependency would pick the wrong runner.
func TestResolveCurrentRunner_LatestTaskRunnerOrdersByCreatedAtNotInsertionOrder(t *testing.T) {
	repo := setupTestRepo(t)
	ctx := context.Background()
	work := newPhase2TestStep(t, repo, "Work")
	review := newPhase2TestStep(t, repo, "Review")
	done := newPhase2TestStep(t, repo, "Done")

	older := time.Now().UTC().Add(-time.Hour).Truncate(time.Millisecond)
	newer := time.Now().UTC().Truncate(time.Millisecond)

	// Inserted FIRST (lowest rowid/insertion order) but carries the NEWER
	// created_at timestamp.
	if _, err := repo.db.ExecContext(ctx, repo.db.Rebind(`
		INSERT INTO workflow_step_participants
			(id, step_id, task_id, role, agent_profile_id, decision_required, position, created_at)
		VALUES (?, ?, ?, 'runner', ?, 0, 0, ?)
	`), "runner-row-inserted-first", work.ID, "task-reorder", "runner-newer-created-at", newer); err != nil {
		t.Fatalf("insert first runner row: %v", err)
	}
	// Inserted SECOND (highest rowid/insertion order) but carries the OLDER
	// created_at timestamp.
	if _, err := repo.db.ExecContext(ctx, repo.db.Rebind(`
		INSERT INTO workflow_step_participants
			(id, step_id, task_id, role, agent_profile_id, decision_required, position, created_at)
		VALUES (?, ?, ?, 'runner', ?, 0, 0, ?)
	`), "runner-row-inserted-second", review.ID, "task-reorder", "runner-older-created-at", older); err != nil {
		t.Fatalf("insert second runner row: %v", err)
	}

	got, err := repo.ResolveCurrentRunner(ctx, done.ID, "task-reorder")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if got != "runner-newer-created-at" {
		t.Fatalf("expected the row with the newer created_at (runner-newer-created-at) regardless of insertion order, got %q", got)
	}
}

func TestResolveCurrentRunner_TreatsEmptyTaskRunnerAsMissing(t *testing.T) {
	repo := setupTestRepo(t)
	ctx := context.Background()
	work := newPhase2TestStep(t, repo, "Work")
	done := newPhase2TestStep(t, repo, "Done")

	_, err := repo.db.ExecContext(ctx, repo.db.Rebind(`
		INSERT INTO workflow_step_participants
			(id, step_id, task_id, role, agent_profile_id, decision_required, position)
		VALUES (?, ?, ?, 'runner', '', 0, 0)
	`), "empty-runner", work.ID, "task-done")
	if err != nil {
		t.Fatalf("insert empty runner: %v", err)
	}
	got, err := repo.ResolveCurrentRunner(ctx, done.ID, "task-done")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if got != "" {
		t.Fatalf("expected no runner, got %q", got)
	}
}

func TestResolveCurrentRunner_PrefersStepPrimaryOverOtherStepRunner(t *testing.T) {
	repo := setupTestRepo(t)
	ctx := context.Background()
	work := &models.WorkflowStep{
		WorkflowID: "wf-test", Name: "Work", Position: 0,
		AgentProfileID: "primary-on-work",
	}
	done := &models.WorkflowStep{
		WorkflowID: "wf-test", Name: "Done", Position: 1,
		AgentProfileID: "primary-on-done",
	}
	if err := repo.CreateStep(ctx, work); err != nil {
		t.Fatalf("create work step: %v", err)
	}
	if err := repo.CreateStep(ctx, done); err != nil {
		t.Fatalf("create done step: %v", err)
	}
	if err := repo.SetTaskRunner(ctx, work.ID, "task-done", "runner-on-work"); err != nil {
		t.Fatalf("set runner: %v", err)
	}
	got, err := repo.ResolveCurrentRunner(ctx, done.ID, "task-done")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if got != "primary-on-done" {
		t.Fatalf("expected primary-on-done, got %q", got)
	}
}

// TestSetTaskRunner_Idempotent verifies SetTaskRunner replaces an
// existing runner participant rather than creating a second row.
func TestSetTaskRunner_Idempotent(t *testing.T) {
	repo := setupTestRepo(t)
	ctx := context.Background()
	step := newPhase2TestStep(t, repo, "Work")

	if err := repo.SetTaskRunner(ctx, step.ID, "task-iter", "agent-1"); err != nil {
		t.Fatalf("set 1: %v", err)
	}
	if err := repo.SetTaskRunner(ctx, step.ID, "task-iter", "agent-2"); err != nil {
		t.Fatalf("set 2: %v", err)
	}
	got, err := repo.ResolveCurrentRunner(ctx, step.ID, "task-iter")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if got != "agent-2" {
		t.Fatalf("expected last setter wins, got %q", got)
	}
	// Verify only a single row exists.
	rows, err := repo.ListStepParticipantsForTask(ctx, step.ID, "task-iter")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	runnerCount := 0
	for _, p := range rows {
		if p.Role == models.ParticipantRoleRunner {
			runnerCount++
		}
	}
	if runnerCount != 1 {
		t.Fatalf("expected 1 runner row, got %d", runnerCount)
	}
}

// TestClearTaskRunner_RemovesRow verifies ClearTaskRunner deletes the
// runner participant; the resolver then falls back to the step's primary.
func TestClearTaskRunner_RemovesRow(t *testing.T) {
	repo := setupTestRepo(t)
	ctx := context.Background()
	step := &models.WorkflowStep{
		WorkflowID: "wf-test", Name: "Work", Position: 0,
		AgentProfileID: "primary-agent",
	}
	if err := repo.CreateStep(ctx, step); err != nil {
		t.Fatalf("create step: %v", err)
	}
	if err := repo.SetTaskRunner(ctx, step.ID, "task-clr", "runner-agent"); err != nil {
		t.Fatalf("set runner: %v", err)
	}
	if err := repo.ClearTaskRunner(ctx, step.ID, "task-clr"); err != nil {
		t.Fatalf("clear runner: %v", err)
	}
	got, err := repo.ResolveCurrentRunner(ctx, step.ID, "task-clr")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if got != "primary-agent" {
		t.Fatalf("expected fallback to primary-agent, got %q", got)
	}
}

// TestResolveCurrentRunner_RejectsEmptyKey covers input validation.
func TestResolveCurrentRunner_RejectsEmptyKey(t *testing.T) {
	repo := setupTestRepo(t)
	if _, err := repo.ResolveCurrentRunner(context.Background(), "", "t"); err == nil {
		t.Fatalf("expected error for empty step_id")
	}
	if _, err := repo.ResolveCurrentRunner(context.Background(), "s", ""); err == nil {
		t.Fatalf("expected error for empty task_id")
	}
}

func TestParticipantsCascadeOnStepDelete(t *testing.T) {
	repo := setupTestRepo(t)
	ctx := context.Background()
	step := newPhase2TestStep(t, repo, "Review")

	if err := repo.UpsertStepParticipant(ctx, &models.WorkflowStepParticipant{
		StepID: step.ID, Role: models.ParticipantRoleReviewer, AgentProfileID: "p",
	}); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	// Sanity: row exists.
	rows, err := repo.ListStepParticipants(ctx, step.ID)
	if err != nil || len(rows) != 1 {
		t.Fatalf("expected one participant, got %d (err=%v)", len(rows), err)
	}

	if err := repo.DeleteStep(ctx, step.ID); err != nil {
		t.Fatalf("delete step: %v", err)
	}
	rows, err = repo.ListStepParticipants(ctx, step.ID)
	if err != nil {
		t.Fatalf("list after cascade: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("expected cascade delete to remove participants, got %d", len(rows))
	}
}

func TestStageType_RoundTripsThroughCreateAndUpdate(t *testing.T) {
	repo := setupTestRepo(t)
	ctx := context.Background()

	step := &models.WorkflowStep{
		WorkflowID: "wf-test",
		Name:       "StageWork",
		Position:   0,
		StageType:  models.StageTypeWork,
	}
	if err := repo.CreateStep(ctx, step); err != nil {
		t.Fatalf("create: %v", err)
	}

	got, err := repo.GetStep(ctx, step.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.StageType != models.StageTypeWork {
		t.Fatalf("expected stage_type 'work', got %q", got.StageType)
	}

	got.StageType = models.StageTypeApproval
	if err := repo.UpdateStep(ctx, got); err != nil {
		t.Fatalf("update: %v", err)
	}
	got2, _ := repo.GetStep(ctx, step.ID)
	if got2.StageType != models.StageTypeApproval {
		t.Fatalf("expected stage_type 'approval' after update, got %q", got2.StageType)
	}
}

func TestStageType_DefaultsToCustomWhenUnset(t *testing.T) {
	repo := setupTestRepo(t)
	ctx := context.Background()

	step := &models.WorkflowStep{
		WorkflowID: "wf-test",
		Name:       "NoStage",
		Position:   0,
	}
	if err := repo.CreateStep(ctx, step); err != nil {
		t.Fatalf("create: %v", err)
	}
	got, err := repo.GetStep(ctx, step.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.StageType != models.StageTypeCustom {
		t.Fatalf("expected default stage_type 'custom', got %q", got.StageType)
	}
}
