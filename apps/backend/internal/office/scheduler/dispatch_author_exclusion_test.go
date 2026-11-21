package scheduler_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"

	settingsstore "github.com/kandev/kandev/internal/agent/settings/store"
	officemodels "github.com/kandev/kandev/internal/office/models"
	officesqlite "github.com/kandev/kandev/internal/office/repository/sqlite"
	"github.com/kandev/kandev/internal/office/routing"
	"github.com/kandev/kandev/internal/office/scheduler"
	runsmodels "github.com/kandev/kandev/internal/runs/models"
	taskrepo "github.com/kandev/kandev/internal/task/repository/sqlite"
)

const (
	exclTaskID        = "task-excl"
	exclReviewEvents  = `{"on_enter":[{"type":"ensure_participant_seat","config":{"role":"reviewer","exclude_author_provider":true}}]}`
	exclPlainEvents   = `{"on_enter":[{"type":"ensure_participant_seat","config":{"role":"reviewer"}}]}`
	exclAuthorEvent   = "routing.author_exclusion"
	exclReviewPayload = `{"task_id":"task-excl","workflow_step_id":"review"}`
)

// newExclusionRepo builds an office repository on the full task schema so
// workflow_steps, tasks and workflow_step_participants match production.
func newExclusionRepo(t *testing.T, reviewEvents string) *officesqlite.Repository {
	t.Helper()
	db, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	if _, _, err := settingsstore.Provide(db, db, nil); err != nil {
		t.Fatalf("settings store: %v", err)
	}
	if _, err := taskrepo.NewWithDB(db, db, nil); err != nil {
		t.Fatalf("task repo: %v", err)
	}
	repo, err := officesqlite.NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("office repo: %v", err)
	}
	ctx := context.Background()
	for _, step := range []struct{ id, stage, events string }{
		{"work", "work", `{}`},
		{"review", "review", reviewEvents},
	} {
		if _, err := repo.ExecRaw(ctx,
			`INSERT INTO workflow_steps (id, workflow_id, name, position, stage_type, events) VALUES (?, 'wf-excl', ?, 0, ?, ?)`,
			step.id, step.id, step.stage, step.events); err != nil {
			t.Fatalf("insert step %s: %v", step.id, err)
		}
	}
	if _, err := repo.ExecRaw(ctx,
		`INSERT INTO tasks (id, workspace_id, workflow_id, workflow_step_id, title, description, state, created_at, updated_at)
		 VALUES (?, ?, 'wf-excl', 'review', 'T', '', 'REVIEW', ?, ?)`,
		exclTaskID, testWorkspaceID, time.Now().UTC(), time.Now().UTC()); err != nil {
		t.Fatalf("insert task: %v", err)
	}
	return repo
}

func seatExclusionReviewer(t *testing.T, repo *officesqlite.Repository) {
	t.Helper()
	if _, err := repo.ExecRaw(context.Background(),
		`INSERT INTO workflow_step_participants (id, step_id, task_id, role, agent_profile_id, decision_required, position)
		 VALUES ('seat-reviewer', 'review', ?, 'reviewer', ?, 1, 0)`, exclTaskID, testAgentID); err != nil {
		t.Fatalf("seat reviewer: %v", err)
	}
}

func seedWorkAuthor(t *testing.T, repo *officesqlite.Repository, provider string) {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC().Add(-time.Hour)
	run := &runsmodels.Run{
		ID: "run-work-" + provider, AgentProfileID: "worker", Reason: "task_assigned",
		Payload: `{"task_id":"task-excl","workflow_step_id":"work"}`,
		Status:  "finished", CoalescedCount: 1, RequestedAt: now, ClaimedAt: &now,
	}
	if err := repo.CreateRun(ctx, run); err != nil {
		t.Fatalf("create work run: %v", err)
	}
	if _, err := repo.ExecRaw(ctx, `UPDATE runs SET claimed_at = ? WHERE id = ?`, now, run.ID); err != nil {
		t.Fatalf("claim work run: %v", err)
	}
	if err := repo.SetRunResolvedRoute(ctx, run.ID, provider+"-profile", provider, provider+"-bal"); err != nil {
		t.Fatalf("resolve work run: %v", err)
	}
}

func authorExclusionEvents(t *testing.T, repo *officesqlite.Repository, runID string) []map[string]interface{} {
	t.Helper()
	events, err := repo.ListRunEvents(context.Background(), runID, -1, 0)
	if err != nil {
		t.Fatalf("list run events: %v", err)
	}
	var out []map[string]interface{}
	for _, e := range events {
		if string(e.EventType) != exclAuthorEvent {
			continue
		}
		var payload map[string]interface{}
		if err := json.Unmarshal([]byte(e.Payload), &payload); err != nil {
			t.Fatalf("decode payload: %v", err)
		}
		out = append(out, payload)
	}
	return out
}

func dispatchExclusionRun(t *testing.T, repo *officesqlite.Repository, starter *fakeTaskStarter, payload string) (*runsmodels.Run, bool, bool, error) {
	t.Helper()
	ss := buildScheduler(t, repo, starter)
	run := seedRun(t, repo, payload)
	launched, parked, err := ss.DispatchWithRouting(context.Background(), run, makeAgent(), scheduler.LaunchContext{})
	return run, launched, parked, err
}

func TestDispatchAuthorExclusion_LaunchesOnNonAuthorProvider(t *testing.T) {
	repo := newExclusionRepo(t, exclReviewEvents)
	seedRoutingConfig(t, repo, []routing.ProviderID{"claude-acp", "codex-acp"})
	seatExclusionReviewer(t, repo)
	seedWorkAuthor(t, repo, "claude-acp")
	starter := newFakeTaskStarter()

	run, launched, parked, err := dispatchExclusionRun(t, repo, starter, exclReviewPayload)
	if err != nil || !launched || parked {
		t.Fatalf("launched=%v parked=%v err=%v, want launch", launched, parked, err)
	}
	if got := starter.lastCall().route.ProviderID; got != "codex-acp" {
		t.Fatalf("launched on %q, want codex-acp", got)
	}
	events := authorExclusionEvents(t, repo, run.ID)
	if len(events) != 1 || events[0]["applied"] != true || events[0]["author_provider_id"] != "claude-acp" {
		t.Fatalf("author exclusion events = %+v", events)
	}
}

func TestDispatchAuthorExclusion_ParksWaitingWhenAlternativeDegraded(t *testing.T) {
	repo := newExclusionRepo(t, exclReviewEvents)
	seedRoutingConfig(t, repo, []routing.ProviderID{"claude-acp", "codex-acp"})
	seatExclusionReviewer(t, repo)
	seedWorkAuthor(t, repo, "claude-acp")
	retryAt := time.Now().UTC().Add(time.Hour)
	if err := repo.MarkProviderDegraded(context.Background(), officemodels.ProviderHealth{
		WorkspaceID: testWorkspaceID, ProviderID: "codex-acp",
		Scope: officesqlite.HealthScopeProvider, State: officesqlite.HealthStateDegraded,
		ErrorCode: "rate_limited", RetryAt: &retryAt,
	}); err != nil {
		t.Fatalf("degrade codex: %v", err)
	}
	starter := newFakeTaskStarter()

	run, launched, parked, err := dispatchExclusionRun(t, repo, starter, exclReviewPayload)
	if err != nil || launched || !parked || starter.callCount() != 0 {
		t.Fatalf("launched=%v parked=%v err=%v calls=%d, want park", launched, parked, err, starter.callCount())
	}
	assertParkedStatus(t, repo, run.ID, routing.StatusWaitingForCapacity)
	assertAuthorSkipAttempt(t, repo, run.ID, "claude-acp")
}

func TestDispatchAuthorExclusion_OnlyAuthorProviderBlocksForAction(t *testing.T) {
	repo := newExclusionRepo(t, exclReviewEvents)
	seedRoutingConfig(t, repo, []routing.ProviderID{"claude-acp"})
	seatExclusionReviewer(t, repo)
	seedWorkAuthor(t, repo, "claude-acp")
	starter := newFakeTaskStarter()

	run, launched, parked, err := dispatchExclusionRun(t, repo, starter, exclReviewPayload)
	if err != nil || launched || !parked || starter.callCount() != 0 {
		t.Fatalf("launched=%v parked=%v err=%v calls=%d, want park", launched, parked, err, starter.callCount())
	}
	assertParkedStatus(t, repo, run.ID, routing.StatusBlockedActionRequired)
	assertAuthorSkipAttempt(t, repo, run.ID, "claude-acp")
}

func TestDispatchAuthorExclusion_NoAuthorRecordedLaunchesAndRecords(t *testing.T) {
	repo := newExclusionRepo(t, exclReviewEvents)
	seedRoutingConfig(t, repo, []routing.ProviderID{"claude-acp", "codex-acp"})
	seatExclusionReviewer(t, repo)
	starter := newFakeTaskStarter()

	run, launched, _, err := dispatchExclusionRun(t, repo, starter, exclReviewPayload)
	if err != nil || !launched || starter.lastCall().route.ProviderID != "claude-acp" {
		t.Fatalf("launched=%v err=%v provider=%q, want claude-acp launch", launched, err, starter.lastCall().route.ProviderID)
	}
	events := authorExclusionEvents(t, repo, run.ID)
	if len(events) != 1 || events[0]["applied"] != false || events[0]["reason"] != "no_author_provider" {
		t.Fatalf("author exclusion events = %+v", events)
	}
}

func TestDispatchAuthorExclusion_RoutingDisabledRecordsNotApplied(t *testing.T) {
	repo := newExclusionRepo(t, exclReviewEvents)
	seedRoutingConfig(t, repo, []routing.ProviderID{"claude-acp", "codex-acp"})
	cfg, err := repo.GetWorkspaceRouting(context.Background(), testWorkspaceID)
	if err != nil {
		t.Fatalf("get routing: %v", err)
	}
	cfg.Enabled = false
	if err := repo.UpsertWorkspaceRouting(context.Background(), testWorkspaceID, cfg); err != nil {
		t.Fatalf("disable routing: %v", err)
	}
	seatExclusionReviewer(t, repo)
	seedWorkAuthor(t, repo, "claude-acp")
	starter := newFakeTaskStarter()

	run, launched, _, err := dispatchExclusionRun(t, repo, starter, exclReviewPayload)
	if err != nil || !launched || starter.lastCall().route.ProviderID != "claude-acp" {
		t.Fatalf("launched=%v err=%v provider=%q, want unchanged claude-acp launch", launched, err, starter.lastCall().route.ProviderID)
	}
	events := authorExclusionEvents(t, repo, run.ID)
	if len(events) != 1 || events[0]["applied"] != false || events[0]["reason"] != "routing_disabled" ||
		events[0]["author_provider_id"] != "claude-acp" {
		t.Fatalf("author exclusion events = %+v", events)
	}
}

func TestDispatchAuthorExclusion_NotApplicableDispatchesUnchanged(t *testing.T) {
	cases := []struct {
		name    string
		events  string
		seat    bool
		payload string
	}{
		{"step does not declare", exclPlainEvents, true, exclReviewPayload},
		{"agent holds no seat", exclReviewEvents, false, exclReviewPayload},
		{"run names no step", exclReviewEvents, true, `{"task_id":"task-excl"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := newExclusionRepo(t, tc.events)
			seedRoutingConfig(t, repo, []routing.ProviderID{"claude-acp", "codex-acp"})
			if tc.seat {
				seatExclusionReviewer(t, repo)
			}
			seedWorkAuthor(t, repo, "claude-acp")
			starter := newFakeTaskStarter()

			run, launched, _, err := dispatchExclusionRun(t, repo, starter, tc.payload)
			if err != nil || !launched || starter.lastCall().route.ProviderID != "claude-acp" {
				t.Fatalf("launched=%v err=%v provider=%q, want claude-acp", launched, err, starter.lastCall().route.ProviderID)
			}
			if events := authorExclusionEvents(t, repo, run.ID); len(events) != 0 {
				t.Fatalf("author exclusion events = %+v, want none", events)
			}
			attempts, err := repo.ListRouteAttempts(context.Background(), run.ID)
			if err != nil || len(attempts) != 1 {
				t.Fatalf("attempts = %+v, err=%v; want one launch attempt", attempts, err)
			}
		})
	}
}

func TestDispatchAuthorExclusion_CorruptStepEventsFailsDispatch(t *testing.T) {
	repo := newExclusionRepo(t, `{not json`)
	seedRoutingConfig(t, repo, []routing.ProviderID{"claude-acp", "codex-acp"})
	seatExclusionReviewer(t, repo)
	starter := newFakeTaskStarter()

	_, launched, parked, err := dispatchExclusionRun(t, repo, starter, exclReviewPayload)
	if err == nil || launched || parked || starter.callCount() != 0 {
		t.Fatalf("launched=%v parked=%v err=%v calls=%d, want dispatch error without launch",
			launched, parked, err, starter.callCount())
	}
}

func assertParkedStatus(t *testing.T, repo *officesqlite.Repository, runID, want string) {
	t.Helper()
	got, err := repo.GetRun(context.Background(), runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if got.RoutingBlockedStatus == nil || string(*got.RoutingBlockedStatus) != want {
		t.Fatalf("routing blocked status = %v, want %q", got.RoutingBlockedStatus, want)
	}
}

func assertAuthorSkipAttempt(t *testing.T, repo *officesqlite.Repository, runID, provider string) {
	t.Helper()
	attempts, err := repo.ListRouteAttempts(context.Background(), runID)
	if err != nil {
		t.Fatalf("list attempts: %v", err)
	}
	for _, a := range attempts {
		if a.ProviderID == provider && a.Outcome == scheduler.RouteAttemptOutcomeSkippedAuthorProvider {
			return
		}
	}
	t.Fatalf("attempts = %+v, want %s skipped_author_provider", attempts, provider)
}

func TestDispatchAuthorExclusion_FallbackNeverReachesAuthor(t *testing.T) {
	repo := newExclusionRepo(t, exclReviewEvents)
	seedRoutingConfig(t, repo, []routing.ProviderID{"claude-acp", "codex-acp", "opencode-acp"})
	seatExclusionReviewer(t, repo)
	seedWorkAuthor(t, repo, "claude-acp")
	starter := newFakeTaskStarter()
	starter.failFor["codex-acp"] = fmt.Errorf("insufficient_quota")

	_, launched, _, err := dispatchExclusionRun(t, repo, starter, exclReviewPayload)
	if err != nil || !launched {
		t.Fatalf("launched=%v err=%v, want fallback launch", launched, err)
	}
	if got := starter.lastCall().route.ProviderID; got != "opencode-acp" {
		t.Fatalf("fallback launched on %q, want opencode-acp", got)
	}
	for _, call := range starter.calls {
		if call.route.ProviderID == "claude-acp" {
			t.Fatalf("author provider was attempted: %+v", starter.calls)
		}
	}
}

func TestDispatchAuthorExclusion_RetryReappliesAndRecordsEachResolution(t *testing.T) {
	repo := newExclusionRepo(t, exclReviewEvents)
	seedRoutingConfig(t, repo, []routing.ProviderID{"claude-acp"})
	seatExclusionReviewer(t, repo)
	seedWorkAuthor(t, repo, "claude-acp")
	starter := newFakeTaskStarter()
	ss := buildScheduler(t, repo, starter)
	run := seedRun(t, repo, exclReviewPayload)

	for i := 0; i < 2; i++ {
		fresh, err := repo.GetRun(context.Background(), run.ID)
		if err != nil {
			t.Fatalf("get run: %v", err)
		}
		launched, parked, err := ss.DispatchWithRouting(context.Background(), fresh, makeAgent(), scheduler.LaunchContext{})
		if err != nil || launched || !parked {
			t.Fatalf("dispatch %d: launched=%v parked=%v err=%v, want park", i, launched, parked, err)
		}
	}
	if starter.callCount() != 0 {
		t.Fatalf("launch calls = %d, want 0", starter.callCount())
	}
	if events := authorExclusionEvents(t, repo, run.ID); len(events) != 2 {
		t.Fatalf("author exclusion events = %d, want one per resolution (2)", len(events))
	}
}
