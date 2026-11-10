package orchestrator

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/agent/planinjection"
	dynamicruntime "github.com/kandev/kandev/internal/agent/runtime/dynamic"
	commonlogger "github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

// newTestServiceWithObservedLogger builds a minimal Service — just the repo
// and an in-memory log sink — sufficient for addDynamicPlan, which only
// touches those two fields. Used to assert on the AC-002.6 log fields.
func newTestServiceWithObservedLogger(t *testing.T, repo sessionExecutorStore) (*Service, *observer.ObservedLogs) {
	t.Helper()
	core, logs := observer.New(zapcore.InfoLevel)
	log, err := commonlogger.NewFromZap(zap.New(core))
	if err != nil {
		t.Fatalf("NewFromZap: %v", err)
	}
	return &Service{repo: repo, logger: log}, logs
}

// fieldValue returns the field with the given key from a logged entry's
// context, and whether it was present at all — a present field can still
// carry a zero value, which the absent-vs-zero AC-002.6 tests need to tell
// apart.
func fieldValue(fields []zapcore.Field, key string) (zapcore.Field, bool) {
	for _, f := range fields {
		if f.Key == key {
			return f, true
		}
	}
	return zapcore.Field{}, false
}

// TestAddDynamicPlanBoundsOverBudgetPlan is the AC-002.2 anti-bypass test for
// the dynamic continuation site: it exercises addDynamicPlan itself, not the
// reducer, so a site that stopped calling the reducer would fail this test
// even though the reducer's own tests still pass.
func TestAddDynamicPlanBoundsOverBudgetPlan(t *testing.T) {
	ctx := context.Background()
	const taskID = "task-dynamic-plan-over-budget"
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, taskID, "session-dynamic-plan-over-budget", models.TaskSessionStateRunning)

	var body strings.Builder
	for i := 1; i <= 400; i++ {
		body.WriteString("## Section ")
		body.WriteString(strings.Repeat("x", 40))
		body.WriteString("\n")
		body.WriteString(strings.Repeat("y", 80))
		body.WriteString("\n")
	}
	if err := repo.CreateTaskPlan(ctx, &models.TaskPlan{
		ID: "plan-dynamic-over-budget", TaskID: taskID, Title: "Big plan", Content: body.String(),
	}); err != nil {
		t.Fatalf("CreateTaskPlan: %v", err)
	}

	taskRepo := newMockTaskRepo()
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), taskRepo, &mockAgentManager{})

	var input dynamicruntime.ContinuationInput
	if err := svc.addDynamicPlan(ctx, taskID, &input); err != nil {
		t.Fatalf("addDynamicPlan: %v", err)
	}

	if !strings.Contains(input.PlanSummary, "sections omitted") || !strings.Contains(input.PlanSummary, "get_task_plan_kandev") {
		t.Fatalf("PlanSummary does not carry the omission notice: %q", input.PlanSummary)
	}
	if len(input.PlanSummary) > 4000 {
		t.Fatalf("PlanSummary len = %d, exceeds the dynamic budget", len(input.PlanSummary))
	}
}

func TestAddDynamicPlanIsByteIdenticalUnderBudget(t *testing.T) {
	ctx := context.Background()
	const taskID = "task-dynamic-plan-under-budget"
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, taskID, "session-dynamic-plan-under-budget", models.TaskSessionStateRunning)

	if err := repo.CreateTaskPlan(ctx, &models.TaskPlan{
		ID: "plan-dynamic-under-budget", TaskID: taskID, Title: "Small plan", Content: "## Notes\nJust a few notes.\n",
	}); err != nil {
		t.Fatalf("CreateTaskPlan: %v", err)
	}

	taskRepo := newMockTaskRepo()
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), taskRepo, &mockAgentManager{})

	var input dynamicruntime.ContinuationInput
	if err := svc.addDynamicPlan(ctx, taskID, &input); err != nil {
		t.Fatalf("addDynamicPlan: %v", err)
	}

	want := "Small plan\n## Notes\nJust a few notes."
	if input.PlanSummary != want {
		t.Fatalf("PlanSummary = %q, want %q (byte-identical to today's composition)", input.PlanSummary, want)
	}
}

// TestAddDynamicPlanInjectsBareTitleWhenContentIsWhitespaceOnly is the
// AC-001.12 named exception for the dynamic site: a plan with a title but
// whitespace-only content composes to a non-empty document (the bare
// title), unlike the handover site where the composed document is content
// alone and this same input injects nothing.
func TestAddDynamicPlanInjectsBareTitleWhenContentIsWhitespaceOnly(t *testing.T) {
	ctx := context.Background()
	const taskID = "task-dynamic-plan-bare-title"
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, taskID, "session-dynamic-plan-bare-title", models.TaskSessionStateRunning)

	if err := repo.CreateTaskPlan(ctx, &models.TaskPlan{
		ID: "plan-dynamic-bare-title", TaskID: taskID, Title: "Some title", Content: "   \n\t \n",
	}); err != nil {
		t.Fatalf("CreateTaskPlan: %v", err)
	}

	taskRepo := newMockTaskRepo()
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), taskRepo, &mockAgentManager{})

	var input dynamicruntime.ContinuationInput
	if err := svc.addDynamicPlan(ctx, taskID, &input); err != nil {
		t.Fatalf("addDynamicPlan: %v", err)
	}

	if input.PlanSummary != "Some title" {
		t.Fatalf("PlanSummary = %q, want %q (bare title)", input.PlanSummary, "Some title")
	}
}

// TestAddDynamicPlanLogsReductionFields is the AC-002.6 assertion for the
// dynamic site: on a reducing plan, the "reducing dynamic continuation plan"
// record carries exactly the five documented keys, "site" set to the literal
// "dynamic_continuation", and byte/section counts matching what the reducer
// actually returned.
func TestAddDynamicPlanLogsReductionFields(t *testing.T) {
	ctx := context.Background()
	const taskID = "task-dynamic-plan-log-reduced"
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, taskID, "session-dynamic-plan-log-reduced", models.TaskSessionStateRunning)

	var body strings.Builder
	for i := 1; i <= 400; i++ {
		body.WriteString("## Section ")
		body.WriteString(strings.Repeat("x", 40))
		body.WriteString("\n")
		body.WriteString(strings.Repeat("y", 80))
		body.WriteString("\n")
	}
	title := "Big plan"
	content := body.String()
	if err := repo.CreateTaskPlan(ctx, &models.TaskPlan{
		ID: "plan-dynamic-log-reduced", TaskID: taskID, Title: title, Content: content,
	}); err != nil {
		t.Fatalf("CreateTaskPlan: %v", err)
	}

	composed := strings.TrimSpace(title + "\n" + content)
	wantOutput, wantReduced, wantOmitted := planinjection.Reduce(composed, planinjection.DynamicBudget)
	if !wantReduced {
		t.Fatal("fixture did not actually reduce; strengthen it")
	}

	svc, logs := newTestServiceWithObservedLogger(t, repo)
	var input dynamicruntime.ContinuationInput
	if err := svc.addDynamicPlan(ctx, taskID, &input); err != nil {
		t.Fatalf("addDynamicPlan: %v", err)
	}

	entries := logs.FilterMessage("reducing dynamic continuation plan").All()
	if len(entries) != 1 {
		t.Fatalf("log entries = %d, want 1; all=%v", len(entries), logs.All())
	}
	fields := entries[0].Context

	if f, ok := fieldValue(fields, "site"); !ok || f.String != "dynamic_continuation" {
		t.Fatalf(`"site" field = %+v (present=%v), want "dynamic_continuation"`, f, ok)
	}
	if f, ok := fieldValue(fields, "task_id"); !ok || f.String != taskID {
		t.Fatalf(`"task_id" field = %+v (present=%v), want %q`, f, ok, taskID)
	}
	if f, ok := fieldValue(fields, "plan_input_bytes"); !ok || f.Integer != int64(len(composed)) {
		t.Fatalf(`"plan_input_bytes" field = %+v (present=%v), want %d`, f, ok, len(composed))
	}
	if f, ok := fieldValue(fields, "plan_output_bytes"); !ok || f.Integer != int64(len(wantOutput)) {
		t.Fatalf(`"plan_output_bytes" field = %+v (present=%v), want %d`, f, ok, len(wantOutput))
	}
	if f, ok := fieldValue(fields, "plan_sections_omitted"); !ok || f.Integer != int64(wantOmitted) {
		t.Fatalf(`"plan_sections_omitted" field = %+v (present=%v), want %d`, f, ok, wantOmitted)
	}
}

// TestAddDynamicPlanLogsZeroOutputWhenNothingFits is the AC-001.13 shape of
// AC-002.6: when the reducer cannot emit any plan content at all, that IS a
// reduction and must be logged, with plan_output_bytes at zero and
// plan_sections_omitted equal to the document's total section count.
func TestAddDynamicPlanLogsZeroOutputWhenNothingFits(t *testing.T) {
	ctx := context.Background()
	const taskID = "task-dynamic-plan-log-nothing-fits"
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, taskID, "session-dynamic-plan-log-nothing-fits", models.TaskSessionStateRunning)

	// A title alone, with no content: composing trims the sole "\n" this
	// site inserts between title and content, leaving one line — the title
	// itself — far larger than DynamicBudget, so not even one complete line
	// fits after reservations. (An empty title would default to "Plan" at
	// the repository, which fits trivially and defeats the fixture.)
	title := strings.Repeat("T", planinjection.DynamicBudget*2)
	if err := repo.CreateTaskPlan(ctx, &models.TaskPlan{
		ID: "plan-dynamic-log-nothing-fits", TaskID: taskID, Title: title, Content: "",
	}); err != nil {
		t.Fatalf("CreateTaskPlan: %v", err)
	}

	svc, logs := newTestServiceWithObservedLogger(t, repo)
	var input dynamicruntime.ContinuationInput
	if err := svc.addDynamicPlan(ctx, taskID, &input); err != nil {
		t.Fatalf("addDynamicPlan: %v", err)
	}
	if input.PlanSummary != "" {
		t.Fatalf("PlanSummary = %q, want empty when nothing fits", input.PlanSummary)
	}

	entries := logs.FilterMessage("reducing dynamic continuation plan").All()
	if len(entries) != 1 {
		t.Fatalf("log entries = %d, want 1; all=%v", len(entries), logs.All())
	}
	fields := entries[0].Context

	if f, ok := fieldValue(fields, "plan_output_bytes"); !ok || f.Integer != 0 {
		t.Fatalf(`"plan_output_bytes" field = %+v (present=%v), want 0`, f, ok)
	}
	if f, ok := fieldValue(fields, "plan_sections_omitted"); !ok || f.Integer != 1 {
		t.Fatalf(`"plan_sections_omitted" field = %+v (present=%v), want 1 (single section, nothing represented)`, f, ok)
	}
}

// TestAddDynamicPlanEmitsNoRecordWhenNotReduced is the negative half of
// AC-002.6 for the dynamic site: unlike the handover site, which always logs
// its own "injecting session handover context" record, the dynamic site
// emits no log at all when nothing was reduced.
func TestAddDynamicPlanEmitsNoRecordWhenNotReduced(t *testing.T) {
	ctx := context.Background()
	const taskID = "task-dynamic-plan-log-not-reduced"
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, taskID, "session-dynamic-plan-log-not-reduced", models.TaskSessionStateRunning)

	if err := repo.CreateTaskPlan(ctx, &models.TaskPlan{
		ID: "plan-dynamic-log-not-reduced", TaskID: taskID, Title: "Small plan", Content: "## Notes\nJust a few notes.\n",
	}); err != nil {
		t.Fatalf("CreateTaskPlan: %v", err)
	}

	svc, logs := newTestServiceWithObservedLogger(t, repo)
	var input dynamicruntime.ContinuationInput
	if err := svc.addDynamicPlan(ctx, taskID, &input); err != nil {
		t.Fatalf("addDynamicPlan: %v", err)
	}

	if got := logs.Len(); got != 0 {
		t.Fatalf("log entries = %d, want 0 (dynamic site emits no record when not reduced); all=%v", got, logs.All())
	}
}

type queuedTranscriptDispatchRepository interface {
	ListPendingQueueDispatches(context.Context) ([]messagequeue.PendingQueueDispatch, error)
	MarkPendingQueueDispatchAccepted(context.Context, *messagequeue.QueuedMessage) error
	DeletePendingQueueDispatch(context.Context, *messagequeue.QueuedMessage) error
}

type queuedTranscriptRaceRepository struct {
	messagequeue.Repository
	dispatches queuedTranscriptDispatchRepository
	sessionID  string
	armed      atomic.Bool

	snapshotRead    chan struct{}
	releaseSnapshot chan struct{}
	claimsRead      chan struct{}
	releaseClaims   chan struct{}
	restoreApplied  chan struct{}

	snapshotOnce sync.Once
	claimsOnce   sync.Once
	restoreOnce  sync.Once
}

func (r *queuedTranscriptRaceRepository) ListBySession(ctx context.Context, sessionID string) ([]messagequeue.QueuedMessage, error) {
	entries, err := r.Repository.ListBySession(ctx, sessionID)
	if err == nil && r.armed.Load() && sessionID == r.sessionID {
		r.snapshotOnce.Do(func() {
			close(r.snapshotRead)
			<-r.releaseSnapshot
		})
	}
	return entries, err
}

func (r *queuedTranscriptRaceRepository) ListPendingQueueDispatches(ctx context.Context) ([]messagequeue.PendingQueueDispatch, error) {
	if r.armed.Load() {
		r.claimsOnce.Do(func() {
			close(r.claimsRead)
			select {
			case <-r.restoreApplied:
			case <-r.releaseClaims:
			}
		})
	}
	return r.dispatches.ListPendingQueueDispatches(ctx)
}

func (r *queuedTranscriptRaceRepository) MarkPendingQueueDispatchAccepted(ctx context.Context, msg *messagequeue.QueuedMessage) error {
	return r.dispatches.MarkPendingQueueDispatchAccepted(ctx, msg)
}

func (r *queuedTranscriptRaceRepository) DeletePendingQueueDispatch(ctx context.Context, msg *messagequeue.QueuedMessage) error {
	return r.dispatches.DeletePendingQueueDispatch(ctx, msg)
}

func (r *queuedTranscriptRaceRepository) Restore(ctx context.Context, msg *messagequeue.QueuedMessage, maxPerSession int) error {
	if err := r.Repository.Restore(ctx, msg, maxPerSession); err != nil {
		return err
	}
	if r.armed.Load() && msg.SessionID == r.sessionID {
		r.restoreOnce.Do(func() { close(r.restoreApplied) })
	}
	return nil
}

func TestDynamicContinuationInventorySerializesConcurrentRestore(t *testing.T) {
	for _, consumer := range []string{"conversation", "relaunch"} {
		t.Run(consumer, func(t *testing.T) {
			assertQueuedTranscriptRestoreCannotSplitInventory(t, consumer)
		})
	}
}

func assertQueuedTranscriptRestoreCannotSplitInventory(t *testing.T, consumer string) {
	ctx := context.Background()
	taskID, sessionID, transcriptID := "race-task", "race-session", "race-transcript"
	repo := setupTestRepo(t)
	seedSession(t, repo, taskID, sessionID, "step-1")
	queue, claim, raceRepo := newQueuedTranscriptRaceQueue(t, taskID, sessionID, transcriptID)
	now := time.Now().UTC()
	require.NoError(t, repo.CreateTurn(ctx, &models.Turn{ID: "race-turn", TaskID: taskID, TaskSessionID: sessionID, StartedAt: now}))
	require.NoError(t, repo.CreateMessage(ctx, &models.Message{
		ID: transcriptID, TaskID: taskID, TaskSessionID: sessionID, TurnID: "race-turn",
		AuthorType: models.MessageAuthorUser, Content: "queued request", CreatedAt: now,
	}))
	svc := &Service{repo: repo, messageQueue: queue, logger: testLogger()}
	raceRepo.armed.Store(true)
	result := make(chan queuedTranscriptConsumerResult, 1)
	go func() { result <- runQueuedTranscriptConsumer(ctx, svc, consumer, sessionID) }()
	awaitQueueBarrier(t, raceRepo.snapshotRead)

	restoreResult := make(chan error, 1)
	restoreStarted := make(chan struct{})
	go func() {
		close(restoreStarted)
		_, err := queue.RestoreMessage(ctx, claim)
		restoreResult <- err
	}()
	awaitQueueBarrier(t, restoreStarted)
	assertQueueBarrierStaysClosed(t, raceRepo.restoreApplied, "restore reached storage during queue snapshot")
	close(raceRepo.releaseSnapshot)
	awaitQueueBarrier(t, raceRepo.claimsRead)

	select {
	case <-raceRepo.restoreApplied:
	case <-time.After(time.Second):
		close(raceRepo.releaseClaims)
	}
	got := awaitQueuedTranscriptResult(t, result)
	require.NoError(t, got.err)
	require.NoError(t, awaitQueuedTranscriptRestore(t, restoreResult))
	entries, _, err := queue.SnapshotSession(ctx, sessionID)
	require.NoError(t, err)
	require.Len(t, entries, 1, "the concurrent restore must retain queue ownership")
	if consumer == "conversation" {
		require.Empty(t, got.userMessages, "a restored but undelivered transcript must not enter continuation context")
		return
	}
	require.False(t, got.promptOK, "a restored but undelivered transcript must not become a relaunch prompt")
}

type queuedTranscriptConsumerResult struct {
	userMessages []string
	promptOK     bool
	err          error
}

func runQueuedTranscriptConsumer(ctx context.Context, svc *Service, consumer, sessionID string) queuedTranscriptConsumerResult {
	if consumer == "conversation" {
		input := &dynamicruntime.ContinuationInput{}
		err := svc.addDynamicConversation(ctx, sessionID, input)
		return queuedTranscriptConsumerResult{userMessages: input.UserMessages, err: err}
	}
	_, promptOK := svc.dynamicRelaunchPrompt(ctx, sessionID)
	return queuedTranscriptConsumerResult{promptOK: promptOK}
}

func newQueuedTranscriptRaceQueue(
	t *testing.T,
	taskID, sessionID, transcriptID string,
) (*messagequeue.Service, *messagequeue.QueuedMessage, *queuedTranscriptRaceRepository) {
	t.Helper()
	rawDB, err := sql.Open("sqlite3", filepath.Join(t.TempDir(), "queue.db"))
	require.NoError(t, err)
	rawDB.SetMaxOpenConns(1)
	queueDB := sqlx.NewDb(rawDB, "sqlite3")
	t.Cleanup(func() { _ = queueDB.Close() })
	baseRepo, err := messagequeue.NewSQLiteRepository(queueDB, queueDB)
	require.NoError(t, err)
	dispatches, ok := baseRepo.(queuedTranscriptDispatchRepository)
	require.True(t, ok)
	raceRepo := &queuedTranscriptRaceRepository{
		Repository: baseRepo, dispatches: dispatches, sessionID: sessionID,
		snapshotRead: make(chan struct{}), releaseSnapshot: make(chan struct{}),
		claimsRead: make(chan struct{}), releaseClaims: make(chan struct{}), restoreApplied: make(chan struct{}),
	}
	queue := messagequeue.NewService(raceRepo, messagequeue.DefaultMaxPerSession, testLogger())
	queued, err := queue.QueueMessageWithMetadata(context.Background(), sessionID, taskID, "queued request", "", "", false, nil, map[string]interface{}{
		messagequeue.MetadataDurableTranscriptMessageID: transcriptID,
	})
	require.NoError(t, err)
	claim, ok := queue.ReserveQueued(context.Background(), sessionID)
	require.True(t, ok)
	require.Equal(t, queued.ID, claim.ID)
	return queue, claim, raceRepo
}

func awaitQueueBarrier(t *testing.T, barrier <-chan struct{}) {
	t.Helper()
	select {
	case <-barrier:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for queue concurrency barrier")
	}
}

func assertQueueBarrierStaysClosed(t *testing.T, barrier <-chan struct{}, message string) {
	t.Helper()
	select {
	case <-barrier:
		t.Fatal(message)
	case <-time.After(time.Second):
	}
}

func awaitQueuedTranscriptResult(t *testing.T, result <-chan queuedTranscriptConsumerResult) queuedTranscriptConsumerResult {
	t.Helper()
	select {
	case got := <-result:
		return got
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for continuation consumer")
		return queuedTranscriptConsumerResult{}
	}
}

func awaitQueuedTranscriptRestore(t *testing.T, result <-chan error) error {
	t.Helper()
	select {
	case err := <-result:
		return err
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for queued message restore")
		return context.DeadlineExceeded
	}
}
