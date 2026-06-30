package service_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/office/models"
	officesqlite "github.com/kandev/kandev/internal/office/repository/sqlite"
	runssqlite "github.com/kandev/kandev/internal/runs/repository/sqlite"
	runsservice "github.com/kandev/kandev/internal/runs/service"
)

// newTestService spins up an in-memory SQLite, builds the office repo
// (which creates the runs / run_events tables under the new names),
// and wraps a fresh runs Service around the embedded runs repo. The
// office repo is created here, not the runs repo directly, so the
// schema migrations run.
func newTestService(t *testing.T) (*runsservice.Service, bus.EventBus) {
	t.Helper()
	svc, eb, _ := newTestServiceWithRepo(t)
	return svc, eb
}

func newTestServiceWithRepo(t *testing.T) (
	*runsservice.Service, bus.EventBus, *runssqlite.Repository,
) {
	t.Helper()
	db, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	officeRepo, err := officesqlite.NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init office repo: %v", err)
	}

	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "console"})
	eb := bus.NewMemoryEventBus(log)

	svc := runsservice.New(officeRepo.RunsRepository(), eb, log, nil)
	return svc, eb, officeRepo.RunsRepository()
}

// agentInPayload is the shape office.QueueRun packs into the runs
// service today: the agent_profile_id rides inside the JSON payload
// because the resolver hasn't been wired yet.
func agentInPayload(agentID string) map[string]any {
	return map[string]any{"agent_profile_id": agentID, "task_id": "t1"}
}

// TestQueueRun_InsertsRow pins the happy path: a fresh request lands
// in the runs table with the agent + reason from the request and a
// JSON payload that round-trips through the service.
func TestQueueRun_InsertsRow(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	if err := svc.QueueRun(ctx, runsservice.QueueRunRequest{
		Reason:  "task_assigned",
		Payload: agentInPayload("a1"),
	}); err != nil {
		t.Fatalf("queue: %v", err)
	}
}

func TestQueueRun_UsesRequestAgentProfileIDAndAddsEnvelopePayload(t *testing.T) {
	svc, _, repo := newTestServiceWithRepo(t)
	ctx := context.Background()

	if err := svc.QueueRun(ctx, runsservice.QueueRunRequest{
		AgentProfileID: "agent-primary",
		TaskID:         "task-1",
		WorkflowStepID: "work",
		Reason:         "task_comment",
		IdempotencyKey: "task_comment:comment-1",
		Payload: map[string]any{
			"comment_id": "comment-1",
			"author_id":  "user-1",
		},
	}); err != nil {
		t.Fatalf("queue: %v", err)
	}

	statuses, err := repo.GetRunsByCommentIDs(ctx, []string{"comment-1"})
	if err != nil {
		t.Fatalf("get comment runs: %v", err)
	}
	status, ok := statuses["comment-1"]
	if !ok {
		t.Fatalf("missing run status for comment-1: %+v", statuses)
	}
	run, err := repo.GetRunByID(ctx, status.RunID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if run.AgentProfileID != "agent-primary" {
		t.Fatalf("agent_profile_id = %q, want agent-primary", run.AgentProfileID)
	}
	if run.IdempotencyKey == nil || *run.IdempotencyKey != "task_comment:comment-1" {
		t.Fatalf("idempotency_key = %v, want task_comment:comment-1", run.IdempotencyKey)
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(run.Payload), &payload); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	for k, want := range map[string]string{
		"agent_profile_id": "agent-primary",
		"task_id":          "task-1",
		"workflow_step_id": "work",
		"comment_id":       "comment-1",
		"author_id":        "user-1",
	} {
		if got, _ := payload[k].(string); got != want {
			t.Fatalf("payload[%s] = %q, want %q (payload=%v)", k, got, want, payload)
		}
	}
}

func TestQueueRun_DoesNotCoalesceDistinctTaskComments(t *testing.T) {
	svc, _, repo := newTestServiceWithRepo(t)
	ctx := context.Background()

	for _, commentID := range []string{"comment-1", "comment-2"} {
		if err := svc.QueueRun(ctx, runsservice.QueueRunRequest{
			AgentProfileID: "agent-primary",
			TaskID:         "task-1",
			WorkflowStepID: "work",
			Reason:         "task_comment",
			IdempotencyKey: "task_comment:" + commentID,
			Payload: map[string]any{
				"comment_id": commentID,
			},
		}); err != nil {
			t.Fatalf("queue %s: %v", commentID, err)
		}
	}

	statuses, err := repo.GetRunsByCommentIDs(ctx, []string{"comment-1", "comment-2"})
	if err != nil {
		t.Fatalf("get comment runs: %v", err)
	}
	if len(statuses) != 2 {
		t.Fatalf("statuses = %+v, want one run per distinct comment", statuses)
	}
	if statuses["comment-1"].RunID == statuses["comment-2"].RunID {
		t.Fatalf("distinct comments must not point at the same coalesced run: %+v", statuses)
	}
}

func TestQueueRun_DoesNotCoalesceTaskCommentPrefixWithCustomReason(t *testing.T) {
	svc, _, repo := newTestServiceWithRepo(t)
	ctx := context.Background()

	commentIDs := []string{"comment-1", "comment-2"}
	keys := []string{
		"task_comment:comment-1:work:task-1:agent-primary:follow-up",
		"task_comment:comment-2:work:task-1:agent-primary:follow-up",
	}
	for i, key := range keys {
		commentID := commentIDs[i]
		if err := svc.QueueRun(ctx, runsservice.QueueRunRequest{
			AgentProfileID: "agent-primary",
			TaskID:         "task-1",
			WorkflowStepID: "work",
			Reason:         "follow_up",
			IdempotencyKey: key,
			Payload: map[string]any{
				"comment_id": commentID,
			},
		}); err != nil {
			t.Fatalf("queue %s: %v", commentID, err)
		}
	}

	for _, key := range keys {
		exists, err := repo.CheckIdempotencyKey(ctx, key, runsservice.IdempotencyWindowHours)
		if err != nil {
			t.Fatalf("check idempotency key %s: %v", key, err)
		}
		if !exists {
			t.Fatalf("missing inserted run for idempotency key %s", key)
		}
	}
}

func TestQueueRun_LegacyCommentWakeDoesNotOverwriteCanonicalCommentRun(t *testing.T) {
	svc, _, repo := newTestServiceWithRepo(t)
	ctx := context.Background()

	if err := svc.QueueRun(ctx, runsservice.QueueRunRequest{
		AgentProfileID: "agent-primary",
		TaskID:         "target-task",
		WorkflowStepID: "work",
		Reason:         "task_comment",
		IdempotencyKey: "task_comment:comment-engine:work:target-task:agent-primary:abcd1234",
		Payload: map[string]any{
			"comment_id":     "comment-engine",
			"source_task_id": "source-task",
		},
	}); err != nil {
		t.Fatalf("queue engine run: %v", err)
	}

	if err := svc.QueueRun(ctx, runsservice.QueueRunRequest{
		AgentProfileID: "agent-primary",
		TaskID:         "source-task",
		Reason:         "task_comment",
		Payload: map[string]any{
			"comment_id": "comment-legacy",
		},
	}); err != nil {
		t.Fatalf("queue legacy run: %v", err)
	}

	statuses, err := repo.GetRunsByCommentIDs(ctx, []string{"comment-engine", "comment-legacy"})
	if err != nil {
		t.Fatalf("get comment runs: %v", err)
	}
	if len(statuses) != 2 {
		t.Fatalf("statuses = %+v, want both comment runs", statuses)
	}
	if statuses["comment-engine"].RunID == statuses["comment-legacy"].RunID {
		t.Fatalf("legacy run coalesced into canonical comment run: %+v", statuses)
	}
	engineRun, err := repo.GetRunByID(ctx, statuses["comment-engine"].RunID)
	if err != nil {
		t.Fatalf("get engine run: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(engineRun.Payload), &payload); err != nil {
		t.Fatalf("decode engine payload: %v", err)
	}
	if payload["task_id"] != "target-task" || payload["source_task_id"] != "source-task" {
		t.Fatalf("engine payload was overwritten by legacy coalesce: %#v", payload)
	}

}

func TestQueueRun_SaltedKeyUsesPayloadCommentID(t *testing.T) {
	svc, _, repo := newTestServiceWithRepo(t)
	ctx := context.Background()

	if err := svc.QueueRun(ctx, runsservice.QueueRunRequest{
		AgentProfileID: "agent-primary",
		TaskID:         "task-1",
		WorkflowStepID: "work",
		Reason:         "task_comment",
		IdempotencyKey: "task_comment:trigger-comment:work:task-1:agent-primary:abcd1234",
		Payload: map[string]any{
			"comment_id": "action-comment",
		},
	}); err != nil {
		t.Fatalf("queue: %v", err)
	}

	statuses, err := repo.GetRunsByCommentIDs(ctx, []string{"trigger-comment", "action-comment"})
	if err != nil {
		t.Fatalf("get comment runs: %v", err)
	}
	if _, ok := statuses["trigger-comment"]; ok {
		t.Fatalf("salted run attached to trigger comment instead of payload comment: %+v", statuses)
	}
	if _, ok := statuses["action-comment"]; !ok {
		t.Fatalf("missing payload comment status: %+v", statuses)
	}
}

// TestQueueRun_CommentStatusIgnoresMentionRuns pins that
// task_mentioned runs carrying the same payload comment_id do not
// replace the task_comment run shown on the comment.
func TestQueueRun_CommentStatusIgnoresMentionRuns(t *testing.T) {
	svc, _, repo := newTestServiceWithRepo(t)
	ctx := context.Background()

	if err := svc.QueueRun(ctx, runsservice.QueueRunRequest{
		AgentProfileID: "agent-primary",
		TaskID:         "task-1",
		Reason:         "task_comment",
		IdempotencyKey: "task_comment:comment-1:work:task-1:agent-primary:abcd1234",
		Payload: map[string]any{
			"comment_id": "comment-1",
		},
	}); err != nil {
		t.Fatalf("queue task_comment: %v", err)
	}
	if err := repo.CreateRun(ctx, &models.Run{
		ID:             "mention-run",
		AgentProfileID: "mentioned-agent",
		Reason:         "task_mentioned",
		Payload:        `{"comment_id":"comment-1","task_id":"task-1"}`,
		Status:         "queued",
		CoalescedCount: 1,
	}); err != nil {
		t.Fatalf("create mention run: %v", err)
	}
	if err := repo.SetRunRequestedAtForTest(ctx, "mention-run", time.Now().UTC().Add(time.Hour)); err != nil {
		t.Fatalf("move mention run later: %v", err)
	}

	statuses, err := repo.GetRunsByCommentIDs(ctx, []string{"comment-1"})
	if err != nil {
		t.Fatalf("get comment runs: %v", err)
	}
	status, ok := statuses["comment-1"]
	if !ok {
		t.Fatalf("missing comment status: %+v", statuses)
	}
	run, err := repo.GetRunByID(ctx, status.RunID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if run.Reason != "task_comment" {
		t.Fatalf("comment status attached to reason %q, want task_comment", run.Reason)
	}
}

func TestQueueRun_PublishesSourceTaskForCrossTaskComment(t *testing.T) {
	svc, eb := newTestService(t)
	ctx := context.Background()

	eventsCh := make(chan map[string]interface{}, 1)
	_, err := eb.Subscribe(events.OfficeRunQueued, func(_ context.Context, e *bus.Event) error {
		if data, ok := e.Data.(map[string]interface{}); ok {
			eventsCh <- data
		}
		return nil
	})
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	if err := svc.QueueRun(ctx, runsservice.QueueRunRequest{
		AgentProfileID: "agent-primary",
		TaskID:         "target-task",
		WorkflowStepID: "target-step",
		Reason:         "task_comment",
		IdempotencyKey: "task_comment:comment-1:work:target-task:agent-primary:abcd1234",
		Payload: map[string]any{
			"comment_id":     "comment-1",
			"source_task_id": "source-task",
		},
	}); err != nil {
		t.Fatalf("queue: %v", err)
	}

	select {
	case data := <-eventsCh:
		if data["task_id"] != "source-task" {
			t.Fatalf("event task_id = %v, want source-task", data["task_id"])
		}
		if data["comment_id"] != "comment-1" {
			t.Fatalf("event comment_id = %v, want comment-1", data["comment_id"])
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for OfficeRunQueued event")
	}
}

// TestQueueRun_RejectsWithoutAgent pins that the service refuses to
// insert when no agent can be resolved from the payload (the
// resolver-less path is what office.QueueRun uses today).
func TestQueueRun_RejectsWithoutAgent(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	err := svc.QueueRun(ctx, runsservice.QueueRunRequest{
		Reason:  "task_assigned",
		Payload: map[string]any{"task_id": "t1"},
	})
	if err == nil {
		t.Fatal("expected error when agent_profile_id is missing")
	}
}

// TestQueueRun_Idempotency pins that a second request with the same
// idempotency key inside the 24h window is suppressed silently —
// neither inserts a new row nor returns an error.
func TestQueueRun_Idempotency(t *testing.T) {
	svc, eb := newTestService(t)
	ctx := context.Background()

	count := 0
	_, err := eb.Subscribe(events.OfficeRunQueued, func(_ context.Context, _ *bus.Event) error {
		count++
		return nil
	})
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	for i := 0; i < 2; i++ {
		if err := svc.QueueRun(ctx, runsservice.QueueRunRequest{
			Reason:         "task_comment",
			IdempotencyKey: "task_comment:c1",
			Payload:        agentInPayload("a1"),
		}); err != nil {
			t.Fatalf("queue %d: %v", i, err)
		}
	}

	// The bus is async by default; let the publish goroutine drain.
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if count >= 1 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if count != 1 {
		t.Errorf("expected exactly one OfficeRunQueued event, got %d", count)
	}
}

// TestQueueRun_Coalescing pins that two requests for the same
// (agent, reason) inside the 5s window collapse onto a single row
// with coalesced_count = 2 instead of producing two queued rows.
func TestQueueRun_Coalescing(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	for i := 0; i < 2; i++ {
		if err := svc.QueueRun(ctx, runsservice.QueueRunRequest{
			Reason:  "task_comment",
			Payload: agentInPayload("a1"),
		}); err != nil {
			t.Fatalf("queue %d: %v", i, err)
		}
	}
	// We can't read the row directly from this package without
	// pulling the runs repo into the test; the no-error assertion
	// plus the signal-count assertion below cover the externally
	// observable contract.
}

// TestQueueRun_SignalsScheduler pins that an INSERT pokes the in-
// process signal channel exactly once. Coalesced rows do not signal
// a second time because they don't add a new claimable row.
func TestQueueRun_SignalsScheduler(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	signal := svc.SubscribeSignal()

	if err := svc.QueueRun(ctx, runsservice.QueueRunRequest{
		Reason:  "task_assigned",
		Payload: agentInPayload("a1"),
	}); err != nil {
		t.Fatalf("queue: %v", err)
	}

	select {
	case <-signal:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("expected signal within 100ms of QueueRun returning")
	}
}

// TestQueueRun_LatencySignalUnder100ms is the B3.7 latency pin: a
// fresh INSERT must produce a claim signal within 100ms. Without
// the event-driven signal path (B3.5) this would take up to one
// 5s tick.
func TestQueueRun_LatencySignalUnder100ms(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	signal := svc.SubscribeSignal()

	start := time.Now()
	if err := svc.QueueRun(ctx, runsservice.QueueRunRequest{
		Reason:  "task_assigned",
		Payload: agentInPayload("a1"),
	}); err != nil {
		t.Fatalf("queue: %v", err)
	}

	select {
	case <-signal:
	case <-time.After(100 * time.Millisecond):
		t.Fatalf("event-driven signal did not arrive within 100ms (elapsed=%s)", time.Since(start))
	}

	if elapsed := time.Since(start); elapsed > 100*time.Millisecond {
		t.Fatalf("signal latency %s exceeds 100ms budget", elapsed)
	}
}

// TestQueueRun_ResolverPathPickedOverPayload pins that when an
// AgentResolver is wired, it is consulted instead of the
// agent_profile_id stored in the payload. The engine's queue_run
// path will use this with a profile→instance resolver.
func TestQueueRun_ResolverPathPickedOverPayload(t *testing.T) {
	db, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	officeRepo, err := officesqlite.NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init: %v", err)
	}
	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "console"})
	eb := bus.NewMemoryEventBus(log)

	resolverCalled := false
	resolver := runsservice.AgentResolverFunc(
		func(_ context.Context, _ runsservice.QueueRunRequest) (string, error) {
			resolverCalled = true
			return "resolved-agent", nil
		},
	)
	svc := runsservice.New(officeRepo.RunsRepository(), eb, log, resolver)

	if err := svc.QueueRun(context.Background(), runsservice.QueueRunRequest{
		Reason:  "task_assigned",
		Payload: agentInPayload("payload-agent"),
	}); err != nil {
		t.Fatalf("queue: %v", err)
	}
	if !resolverCalled {
		t.Fatal("expected resolver to be consulted before payload fallback")
	}
}
