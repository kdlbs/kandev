package repository

import (
	"context"
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
