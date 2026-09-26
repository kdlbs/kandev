package review

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/models"
	taskservice "github.com/kandev/kandev/internal/task/service"
)

// fakeStore records everything the runner persists, so a test can assert on the
// run lifecycle without a database.
type fakeStore struct {
	mu         sync.Mutex
	runs       map[string]*models.TaskReviewRun
	active     *models.TaskReviewRun
	published  []taskservice.PublishFindingsRequest
	completed  []taskservice.CompleteRunRequest
	failures   []fakeFailure
	statuses   []models.ReviewRunStatus
	publishErr error
	nextID     int
}

type fakeFailure struct {
	code    string
	message string
}

func newFakeStore() *fakeStore {
	return &fakeStore{runs: map[string]*models.TaskReviewRun{}}
}

func (f *fakeStore) CreateRun(_ context.Context, req taskservice.CreateRunRequest) (*models.TaskReviewRun, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.nextID++
	run := &models.TaskReviewRun{
		ID:             fmt.Sprintf("run-%d", f.nextID),
		TaskID:         req.TaskID,
		SessionID:      req.SessionID,
		Trigger:        req.Trigger,
		WorkflowStepID: req.WorkflowStepID,
		AgentID:        req.AgentID,
		Model:          req.Model,
		EntryID:        req.EntryID,
		Status:         models.ReviewRunPending,
	}
	f.runs[run.ID] = run
	f.statuses = append(f.statuses, models.ReviewRunPending)
	return run, nil
}

func (f *fakeStore) ActiveRun(context.Context, string) (*models.TaskReviewRun, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.active, nil
}

func (f *fakeStore) FindRunByEntryID(_ context.Context, entryID string) (*models.TaskReviewRun, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if entryID == "" {
		return nil, nil
	}
	for _, run := range f.runs {
		if run.EntryID == entryID {
			return run, nil
		}
	}
	return nil, nil
}

func (f *fakeStore) MarkRunRunning(_ context.Context, runID string) (*models.TaskReviewRun, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	run := f.runs[runID]
	if run == nil {
		return nil, models.ErrTaskReviewRunNotFound
	}
	run.Status = models.ReviewRunRunning
	f.statuses = append(f.statuses, models.ReviewRunRunning)
	return run, nil
}

func (f *fakeStore) CompleteRun(_ context.Context, req taskservice.CompleteRunRequest) (*models.TaskReviewRun, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	// Mirror ReviewService.mutateRunIfLive: a terminal run is never resurrected.
	if run := f.runs[req.RunID]; run != nil && run.Status.IsTerminal() {
		return run, nil
	}
	f.completed = append(f.completed, req)
	f.statuses = append(f.statuses, models.ReviewRunCompleted)
	run := f.runs[req.RunID]
	if run != nil {
		run.Status = models.ReviewRunCompleted
	}
	return run, nil
}

func (f *fakeStore) FailRun(_ context.Context, runID, code, message string, _ int) (*models.TaskReviewRun, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	// Mirror ReviewService.mutateRunIfLive: a terminal run is never resurrected.
	if run := f.runs[runID]; run != nil && run.Status.IsTerminal() {
		return run, nil
	}
	f.failures = append(f.failures, fakeFailure{code: code, message: message})
	f.statuses = append(f.statuses, models.ReviewRunFailed)
	run := f.runs[runID]
	if run != nil {
		run.Status = models.ReviewRunFailed
	}
	return run, nil
}

func (f *fakeStore) CancelRun(_ context.Context, runID string) (*models.TaskReviewRun, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.statuses = append(f.statuses, models.ReviewRunCancelled)
	run := f.runs[runID]
	if run == nil {
		return nil, models.ErrTaskReviewRunNotFound
	}
	// Mirror the real service: a terminal run is left alone.
	if run.Status.IsTerminal() {
		return run, nil
	}
	run.Status = models.ReviewRunCancelled
	return run, nil
}

func (f *fakeStore) statusOf(runID string) models.ReviewRunStatus {
	f.mu.Lock()
	defer f.mu.Unlock()
	if run := f.runs[runID]; run != nil {
		return run.Status
	}
	return ""
}

func (f *fakeStore) runCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.runs)
}

func (f *fakeStore) PublishFindings(_ context.Context, req taskservice.PublishFindingsRequest) (*models.TaskReviewRun, []*models.TaskReviewFinding, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.publishErr != nil {
		return nil, nil, f.publishErr
	}
	f.published = append(f.published, req)
	return f.runs[req.RunID], nil, nil
}

func (f *fakeStore) lastPublished() (taskservice.PublishFindingsRequest, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.published) == 0 {
		return taskservice.PublishFindingsRequest{}, false
	}
	return f.published[len(f.published)-1], true
}

func (f *fakeStore) lastCompleted() (taskservice.CompleteRunRequest, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.completed) == 0 {
		return taskservice.CompleteRunRequest{}, false
	}
	return f.completed[len(f.completed)-1], true
}

func (f *fakeStore) lastFailure() (fakeFailure, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.failures) == 0 {
		return fakeFailure{}, false
	}
	return f.failures[len(f.failures)-1], true
}

// fakeInference returns a canned response per call, cycling on the last one.
type fakeInference struct {
	mu        sync.Mutex
	responses []string
	err       error
	prompts   []string
	identity  ReviewerIdentity
}

func (f *fakeInference) Run(_ context.Context, identity ReviewerIdentity, _ string, prompt string) (*PromptResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.prompts = append(f.prompts, prompt)
	f.identity = identity
	if f.err != nil {
		return nil, f.err
	}
	idx := len(f.prompts) - 1
	if idx >= len(f.responses) {
		idx = len(f.responses) - 1
	}
	return &PromptResult{Response: f.responses[idx], Model: identity.Model, PromptTokens: 10, ResponseTokens: 5}, nil
}

func (f *fakeInference) promptCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.prompts)
}

type fakePrompts struct{ err error }

func (f *fakePrompts) Build(_ context.Context, batch []ChangedFile, _ PromptContext) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	names := make([]string, 0, len(batch))
	for _, b := range batch {
		names = append(names, b.Key())
	}
	return "review: " + strings.Join(names, ","), nil
}

type fakeSessions struct {
	sessionID string
	err       error
}

func (f *fakeSessions) ReviewSessionID(context.Context, string) (string, error) {
	return f.sessionID, f.err
}

type fakeTaskContext struct {
	ctx PromptContext
	err error
}

func (f *fakeTaskContext) ReviewPromptContext(context.Context, string, string) (PromptContext, error) {
	return f.ctx, f.err
}

func testLogger(t *testing.T) *logger.Logger {
	t.Helper()
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json", OutputPath: "stdout"})
	if err != nil {
		t.Fatalf("logger: %v", err)
	}
	return log
}

func okResponse(file string, line int) string {
	return fmt.Sprintf("```json\n{\"summary\":\"one issue\",\"findings\":[{\"file\":%q,\"line\":%d,"+
		"\"severity\":\"major\",\"category\":\"correctness\",\"title\":\"Bug\",\"body\":\"details\"}]}\n```", file, line)
}

type runnerHarness struct {
	runner    *Runner
	store     *fakeStore
	inference *fakeInference
	changes   *fakeChangeSource
}

func newRunnerHarness(t *testing.T, files map[string]any, responses []string) *runnerHarness {
	t.Helper()
	store := newFakeStore()
	inference := &fakeInference{responses: responses}
	changes := &fakeChangeSource{uncommitted: files}
	runner := NewRunner(RunnerDeps{
		Store:       store,
		Resolver:    NewResolver(nil, &fakeUtility{found: true, enabled: true, agentID: "claude-acp", model: "haiku"}, nil),
		Changes:     changes,
		Inference:   inference,
		Prompts:     &fakePrompts{},
		TaskContext: &fakeTaskContext{ctx: PromptContext{TaskTitle: "T"}},
		Sessions:    &fakeSessions{sessionID: "sess-1"},
		Logger:      testLogger(t),
	})
	runner.Start(context.Background())
	t.Cleanup(runner.Stop)
	return &runnerHarness{runner: runner, store: store, inference: inference, changes: changes}
}

func TestRunner_HappyPathPublishesAnchoredFindings(t *testing.T) {
	h := newRunnerHarness(t,
		map[string]any{"a.go": fileEntry("a.go", "@@ -1 +1,2 @@\n old\n+new line\n", "", "")},
		[]string{okResponse("a.go", 2)},
	)

	run, err := h.runner.Run(context.Background(), RunRequest{TaskID: "task-1", Trigger: models.ReviewTriggerManual})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if run.AgentID != "claude-acp" || run.Model != "haiku" {
		t.Fatalf("run should record the resolved reviewer identity, got %+v", run)
	}

	published, ok := h.store.lastPublished()
	if !ok {
		t.Fatal("expected findings to be published")
	}
	if len(published.Findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(published.Findings))
	}
	f := published.Findings[0]
	if f.FilePath != "a.go" || f.StartLine != 2 {
		t.Fatalf("unexpected anchor: %+v", f)
	}
	if f.AnchorText != "new line" {
		t.Fatalf("anchor text should come from the reviewed diff, got %q", f.AnchorText)
	}
	if f.FileDiffHash == "" {
		t.Fatal("finding must carry the reviewed diff's hash for staleness")
	}

	completed, ok := h.store.lastCompleted()
	if !ok {
		t.Fatal("expected the run to complete")
	}
	if completed.FindingCount != 1 || completed.FileCount != 1 || completed.RepositoryCount != 1 {
		t.Fatalf("unexpected run accounting: %+v", completed)
	}
	if completed.PromptTokens != 10 || completed.ResponseTokens != 5 {
		t.Fatalf("expected token accounting recorded, got %+v", completed)
	}
}

func TestRunner_CleanReviewCompletesWithoutPublishing(t *testing.T) {
	h := newRunnerHarness(t,
		map[string]any{"a.go": fileEntry("a.go", "@@ -1 +1,2 @@\n old\n+new\n", "", "")},
		[]string{"```json\n{\"summary\":\"Looks good.\",\"findings\":[]}\n```"},
	)

	if _, err := h.runner.Run(context.Background(), RunRequest{TaskID: "task-clean"}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if _, ok := h.store.lastPublished(); ok {
		t.Fatal("a clean review must not publish findings")
	}
	completed, ok := h.store.lastCompleted()
	if !ok {
		t.Fatal("a clean review must still complete the run")
	}
	if completed.FindingCount != 0 {
		t.Fatalf("expected zero findings, got %d", completed.FindingCount)
	}
	if !strings.Contains(completed.Summary, "Looks good.") {
		t.Fatalf("expected the reviewer summary retained, got %q", completed.Summary)
	}
}

func TestRunner_NoChangesFailsBeforeCreatingARun(t *testing.T) {
	h := newRunnerHarness(t, map[string]any{}, []string{okResponse("a.go", 1)})

	_, err := h.runner.Launch(context.Background(), RunRequest{TaskID: "task-empty"})
	if !errors.Is(err, ErrNoChanges) {
		t.Fatalf("expected ErrNoChanges, got %v", err)
	}
	h.store.mu.Lock()
	defer h.store.mu.Unlock()
	if len(h.store.runs) != 0 {
		t.Fatalf("no run row should exist for an empty change set, got %d", len(h.store.runs))
	}
}

func TestRunner_AgentUnavailableFailsBeforeCreatingARun(t *testing.T) {
	store := newFakeStore()
	runner := NewRunner(RunnerDeps{
		Store:     store,
		Resolver:  NewResolver(nil, nil, nil),
		Changes:   &fakeChangeSource{uncommitted: map[string]any{"a.go": fileEntry("a.go", "diff", "", "")}},
		Inference: &fakeInference{responses: []string{""}},
		Prompts:   &fakePrompts{},
		Sessions:  &fakeSessions{sessionID: "sess-1"},
		Logger:    testLogger(t),
	})
	runner.Start(context.Background())
	t.Cleanup(runner.Stop)

	_, err := runner.Launch(context.Background(), RunRequest{TaskID: "task-noagent"})
	if !errors.Is(err, ErrAgentUnavailable) {
		t.Fatalf("expected ErrAgentUnavailable, got %v", err)
	}
	if len(store.runs) != 0 {
		t.Fatalf("no run row should exist when no reviewer is configured, got %d", len(store.runs))
	}
}

func TestRunner_WorkspaceUnavailable(t *testing.T) {
	store := newFakeStore()
	runner := NewRunner(RunnerDeps{
		Store:     store,
		Resolver:  NewResolver(nil, &fakeUtility{found: true, enabled: true, agentID: "a", model: "m"}, nil),
		Changes:   &fakeChangeSource{statusErr: errors.New("agentctl down")},
		Inference: &fakeInference{responses: []string{""}},
		Prompts:   &fakePrompts{},
		Sessions:  &fakeSessions{sessionID: "sess-1"},
		Logger:    testLogger(t),
	})
	runner.Start(context.Background())
	t.Cleanup(runner.Stop)

	_, err := runner.Launch(context.Background(), RunRequest{TaskID: "task-nows"})
	if !errors.Is(err, ErrWorkspaceUnavailable) {
		t.Fatalf("expected ErrWorkspaceUnavailable, got %v", err)
	}
}

func TestRunner_MissingSessionFailsClosed(t *testing.T) {
	store := newFakeStore()
	runner := NewRunner(RunnerDeps{
		Store:     store,
		Resolver:  NewResolver(nil, &fakeUtility{found: true, enabled: true, agentID: "a", model: "m"}, nil),
		Changes:   &fakeChangeSource{},
		Inference: &fakeInference{responses: []string{""}},
		Prompts:   &fakePrompts{},
		Sessions:  &fakeSessions{sessionID: ""},
		Logger:    testLogger(t),
	})
	runner.Start(context.Background())
	t.Cleanup(runner.Stop)

	if _, err := runner.Launch(context.Background(), RunRequest{TaskID: "task-nosession"}); !errors.Is(err, ErrWorkspaceUnavailable) {
		t.Fatalf("expected ErrWorkspaceUnavailable, got %v", err)
	}
}

func TestRunner_UnparseableResponseFailsTheRun(t *testing.T) {
	h := newRunnerHarness(t,
		map[string]any{"a.go": fileEntry("a.go", "@@ -1 +1,2 @@\n old\n+new\n", "", "")},
		[]string{"I had a look and it seems fine."},
	)

	if _, err := h.runner.Run(context.Background(), RunRequest{TaskID: "task-bad"}); err != nil {
		t.Fatalf("Launch itself should succeed: %v", err)
	}
	failure, ok := h.store.lastFailure()
	if !ok {
		t.Fatal("expected the run to be recorded as failed")
	}
	if failure.code != CodeUnparseableResponse {
		t.Fatalf("expected %q, got %q", CodeUnparseableResponse, failure.code)
	}
	if _, published := h.store.lastPublished(); published {
		t.Fatal("an unparseable review must not publish findings")
	}
}

func TestRunner_InferenceErrorFailsTheRun(t *testing.T) {
	store := newFakeStore()
	runner := NewRunner(RunnerDeps{
		Store:     store,
		Resolver:  NewResolver(nil, &fakeUtility{found: true, enabled: true, agentID: "a", model: "m"}, nil),
		Changes:   &fakeChangeSource{uncommitted: map[string]any{"a.go": fileEntry("a.go", "@@ -1 +1,2 @@\n x\n+y\n", "", "")}},
		Inference: &fakeInference{err: errors.New("provider overloaded")},
		Prompts:   &fakePrompts{},
		Sessions:  &fakeSessions{sessionID: "sess-1"},
		Logger:    testLogger(t),
	})
	runner.Start(context.Background())
	t.Cleanup(runner.Stop)

	if _, err := runner.Run(context.Background(), RunRequest{TaskID: "task-err"}); err != nil {
		t.Fatalf("Launch should succeed: %v", err)
	}
	failure, ok := store.lastFailure()
	if !ok || failure.code != CodeExecutionFailed {
		t.Fatalf("expected an execution failure, got %+v ok=%v", failure, ok)
	}
	if !strings.Contains(failure.message, "provider overloaded") {
		t.Fatalf("failure should carry the provider error, got %q", failure.message)
	}
}

func TestRunner_PromptBuildErrorFailsTheRun(t *testing.T) {
	store := newFakeStore()
	runner := NewRunner(RunnerDeps{
		Store:     store,
		Resolver:  NewResolver(nil, &fakeUtility{found: true, enabled: true, agentID: "a", model: "m"}, nil),
		Changes:   &fakeChangeSource{uncommitted: map[string]any{"a.go": fileEntry("a.go", "@@ -1 +1,2 @@\n x\n+y\n", "", "")}},
		Inference: &fakeInference{responses: []string{""}},
		Prompts:   &fakePrompts{err: errors.New("template missing")},
		Sessions:  &fakeSessions{sessionID: "sess-1"},
		Logger:    testLogger(t),
	})
	runner.Start(context.Background())
	t.Cleanup(runner.Stop)

	if _, err := runner.Run(context.Background(), RunRequest{TaskID: "task-tmpl"}); err != nil {
		t.Fatalf("Launch should succeed: %v", err)
	}
	if failure, ok := store.lastFailure(); !ok || failure.code != CodeExecutionFailed {
		t.Fatalf("expected an execution failure, got %+v ok=%v", failure, ok)
	}
}

func TestRunner_ReturnsExistingActiveRunInsteadOfDuplicating(t *testing.T) {
	h := newRunnerHarness(t,
		map[string]any{"a.go": fileEntry("a.go", "@@ -1 +1,2 @@\n x\n+y\n", "", "")},
		[]string{okResponse("a.go", 2)},
	)
	existing := &models.TaskReviewRun{ID: "run-existing", TaskID: "task-dup", Status: models.ReviewRunRunning}
	h.store.active = existing

	got, err := h.runner.Launch(context.Background(), RunRequest{TaskID: "task-dup"})
	if err != nil {
		t.Fatalf("Launch: %v", err)
	}
	if got.ID != "run-existing" {
		t.Fatalf("expected the in-flight run returned, got %q", got.ID)
	}
	if h.inference.promptCount() != 0 {
		t.Fatal("a duplicate request must not call the provider again")
	}
}

// TestRunner_RedeliveryOfCompletedEntryDoesNotLaunchSecondRun covers
// AC-OFFICE-STEP-ENTRY-001.10: a step-entry redelivery (after the first run
// already completed, or after a restart cleared in-memory dedup state) must
// rejoin the run already created for that entry rather than starting a
// second one. The ActiveRun check alone cannot catch this because the first
// run is no longer pending/running by the time the redelivery arrives.
func TestRunner_RedeliveryOfCompletedEntryDoesNotLaunchSecondRun(t *testing.T) {
	h := newRunnerHarness(t,
		map[string]any{"a.go": fileEntry("a.go", "@@ -1 +1,2 @@\n x\n+y\n", "", "")},
		[]string{okResponse("a.go", 2)},
	)

	first, err := h.runner.Run(context.Background(), RunRequest{TaskID: "task-1", EntryID: "entry-1"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := h.store.statusOf(first.ID); got != models.ReviewRunCompleted {
		t.Fatalf("expected the first run to complete, got status %q", got)
	}

	second, err := h.runner.Run(context.Background(), RunRequest{TaskID: "task-1", EntryID: "entry-1"})
	if err != nil {
		t.Fatalf("Run (redelivery): %v", err)
	}
	if second.ID != first.ID {
		t.Fatalf("expected the redelivery to rejoin run %q, got %q", first.ID, second.ID)
	}
	if got := h.store.runCount(); got != 1 {
		t.Fatalf("expected exactly one run row, got %d", got)
	}
	if got := h.inference.promptCount(); got != 1 {
		t.Fatalf("expected exactly one inference pass, got %d", got)
	}
}

func TestRunner_BatchesLargeChangeSetsAndReportsSkips(t *testing.T) {
	big := strings.Repeat("x", 60)
	files := map[string]any{
		"a.go":    fileEntry("a.go", "@@ -1 +1,2 @@\n x\n+"+big+"\n", "", ""),
		"b.go":    fileEntry("b.go", "@@ -1 +1,2 @@\n x\n+"+big+"\n", "", ""),
		"huge.go": fileEntry("huge.go", strings.Repeat("y", 5000), "", ""),
	}
	store := newFakeStore()
	inference := &fakeInference{responses: []string{
		"```json\n{\"summary\":\"batch one\",\"findings\":[]}\n```",
		"```json\n{\"summary\":\"batch two\",\"findings\":[]}\n```",
	}}
	runner := NewRunner(RunnerDeps{
		Store:       store,
		Resolver:    NewResolver(nil, &fakeUtility{found: true, enabled: true, agentID: "a", model: "m"}, nil),
		Changes:     &fakeChangeSource{uncommitted: files},
		Inference:   inference,
		Prompts:     &fakePrompts{},
		Sessions:    &fakeSessions{sessionID: "sess-1"},
		Logger:      testLogger(t),
		BudgetBytes: 100,
	})
	runner.Start(context.Background())
	t.Cleanup(runner.Stop)

	if _, err := runner.Run(context.Background(), RunRequest{TaskID: "task-batch"}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if inference.promptCount() != 2 {
		t.Fatalf("expected 2 prompt batches, got %d", inference.promptCount())
	}
	completed, ok := store.lastCompleted()
	if !ok {
		t.Fatal("expected completion")
	}
	if completed.FileCount != 2 {
		t.Fatalf("expected 2 submitted files, got %d", completed.FileCount)
	}
	if !strings.Contains(completed.Summary, "huge.go") {
		t.Fatalf("the summary must name the skipped file, got %q", completed.Summary)
	}
	if !strings.Contains(completed.Summary, "batch one") || !strings.Contains(completed.Summary, "batch two") {
		t.Fatalf("expected both batch summaries retained, got %q", completed.Summary)
	}
}

func TestRunner_ReportsRejectedAndOutOfSetFindings(t *testing.T) {
	response := "```json\n{\"summary\":\"mixed\",\"findings\":[" +
		"{\"file\":\"a.go\",\"line\":2,\"severity\":\"major\",\"category\":\"c\",\"title\":\"Real\",\"body\":\"b\"}," +
		"{\"line\":3,\"severity\":\"major\",\"category\":\"c\",\"title\":\"No file\",\"body\":\"b\"}," +
		"{\"file\":\"never-changed.go\",\"line\":3,\"severity\":\"major\",\"category\":\"c\",\"title\":\"Outside\",\"body\":\"b\"}" +
		"]}\n```"
	h := newRunnerHarness(t,
		map[string]any{"a.go": fileEntry("a.go", "@@ -1 +1,2 @@\n x\n+y\n", "", "")},
		[]string{response},
	)

	if _, err := h.runner.Run(context.Background(), RunRequest{TaskID: "task-mixed"}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	published, ok := h.store.lastPublished()
	if !ok || len(published.Findings) != 1 {
		t.Fatalf("expected only the in-set finding stored, got %+v", published.Findings)
	}
	completed, _ := h.store.lastCompleted()
	if !strings.Contains(completed.Summary, "malformed") {
		t.Fatalf("summary should report the malformed entry, got %q", completed.Summary)
	}
	if !strings.Contains(completed.Summary, "outside the reviewed change set") {
		t.Fatalf("summary should report the out-of-set finding, got %q", completed.Summary)
	}
}

func TestRunner_MultiRepoAnchorsToTheRightRepository(t *testing.T) {
	files := map[string]any{
		"frontend\x00src/a.ts": fileEntry("src/a.ts", "@@ -1 +1,2 @@\n x\n+fe\n", "frontend", "repo-fe"),
		"backend\x00src/a.go":  fileEntry("src/a.go", "@@ -1 +1,2 @@\n x\n+be\n", "backend", "repo-be"),
	}
	response := "```json\n{\"summary\":\"\",\"findings\":[{\"repo\":\"backend\",\"file\":\"src/a.go\",\"line\":2," +
		"\"severity\":\"major\",\"category\":\"c\",\"title\":\"Backend bug\",\"body\":\"b\"}]}\n```"
	h := newRunnerHarness(t, files, []string{response})

	if _, err := h.runner.Run(context.Background(), RunRequest{TaskID: "task-multi"}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	published, ok := h.store.lastPublished()
	if !ok || len(published.Findings) != 1 {
		t.Fatalf("expected 1 finding, got %+v", published.Findings)
	}
	f := published.Findings[0]
	if f.RepositoryName != "backend" || f.RepositoryID != "repo-be" || f.FilePath != "src/a.go" {
		t.Fatalf("finding attributed to the wrong repository: %+v", f)
	}
	if f.AnchorText != "be" {
		t.Fatalf("anchor text should come from the backend diff, got %q", f.AnchorText)
	}
}

func TestRunner_AmbiguousUnprefixedPathIsDropped(t *testing.T) {
	// The same path exists in two repositories and the reviewer omitted `repo`.
	// Guessing would attribute the finding to the wrong repository, so it is
	// dropped and reported instead.
	files := map[string]any{
		"frontend\x00README.md": fileEntry("README.md", "@@ -1 +1,2 @@\n x\n+fe\n", "frontend", "repo-fe"),
		"backend\x00README.md":  fileEntry("README.md", "@@ -1 +1,2 @@\n x\n+be\n", "backend", "repo-be"),
	}
	response := "```json\n{\"summary\":\"\",\"findings\":[{\"file\":\"README.md\",\"line\":2," +
		"\"severity\":\"minor\",\"category\":\"c\",\"title\":\"Ambiguous\",\"body\":\"b\"}]}\n```"
	h := newRunnerHarness(t, files, []string{response})

	if _, err := h.runner.Run(context.Background(), RunRequest{TaskID: "task-ambig"}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if _, ok := h.store.lastPublished(); ok {
		t.Fatal("an ambiguous anchor must not be stored against a guessed repository")
	}
	completed, _ := h.store.lastCompleted()
	if !strings.Contains(completed.Summary, "outside the reviewed change set") {
		t.Fatalf("expected the drop reported in the summary, got %q", completed.Summary)
	}
}

func TestRunner_WorkflowStepTriggerIsRecorded(t *testing.T) {
	h := newRunnerHarness(t,
		map[string]any{"a.go": fileEntry("a.go", "@@ -1 +1,2 @@\n x\n+y\n", "", "")},
		[]string{okResponse("a.go", 2)},
	)

	run, err := h.runner.Run(context.Background(), RunRequest{
		TaskID:         "task-step",
		Trigger:        models.ReviewTriggerWorkflowStep,
		WorkflowStepID: "step-7",
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if run.Trigger != models.ReviewTriggerWorkflowStep || run.WorkflowStepID != "step-7" {
		t.Fatalf("expected the workflow-step trigger recorded, got %+v", run)
	}
}

func TestRunner_PublishErrorFailsTheRun(t *testing.T) {
	h := newRunnerHarness(t,
		map[string]any{"a.go": fileEntry("a.go", "@@ -1 +1,2 @@\n x\n+y\n", "", "")},
		[]string{okResponse("a.go", 2)},
	)
	h.store.publishErr = errors.New("db write failed")

	if _, err := h.runner.Run(context.Background(), RunRequest{TaskID: "task-pub"}); err != nil {
		t.Fatalf("Launch should succeed: %v", err)
	}
	if failure, ok := h.store.lastFailure(); !ok || failure.code != CodeExecutionFailed {
		t.Fatalf("expected the run marked failed, got %+v ok=%v", failure, ok)
	}
	if _, ok := h.store.lastCompleted(); ok {
		t.Fatal("a failed publish must not also complete the run")
	}
}

func TestRunner_TaskIDRequired(t *testing.T) {
	h := newRunnerHarness(t, map[string]any{}, []string{""})
	if _, err := h.runner.Launch(context.Background(), RunRequest{}); !errors.Is(err, taskservice.ErrTaskIDRequired) {
		t.Fatalf("expected ErrTaskIDRequired, got %v", err)
	}
}

func TestRunner_StopIsIdempotentAndDrains(t *testing.T) {
	h := newRunnerHarness(t,
		map[string]any{"a.go": fileEntry("a.go", "@@ -1 +1,2 @@\n x\n+y\n", "", "")},
		[]string{okResponse("a.go", 2)},
	)
	if _, err := h.runner.Launch(context.Background(), RunRequest{TaskID: "task-stop"}); err != nil {
		t.Fatalf("Launch: %v", err)
	}
	h.runner.Stop()
	h.runner.Stop()
	// Start after Stop must be usable again.
	h.runner.Start(context.Background())
}

func TestRunner_TaskContextErrorDoesNotFailTheRun(t *testing.T) {
	store := newFakeStore()
	runner := NewRunner(RunnerDeps{
		Store:       store,
		Resolver:    NewResolver(nil, &fakeUtility{found: true, enabled: true, agentID: "a", model: "m"}, nil),
		Changes:     &fakeChangeSource{uncommitted: map[string]any{"a.go": fileEntry("a.go", "@@ -1 +1,2 @@\n x\n+y\n", "", "")}},
		Inference:   &fakeInference{responses: []string{okResponse("a.go", 2)}},
		Prompts:     &fakePrompts{},
		TaskContext: &fakeTaskContext{err: errors.New("task gone")},
		Sessions:    &fakeSessions{sessionID: "sess-1"},
		Logger:      testLogger(t),
	})
	runner.Start(context.Background())
	t.Cleanup(runner.Stop)

	if _, err := runner.Run(context.Background(), RunRequest{TaskID: "task-ctx"}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if _, ok := store.lastCompleted(); !ok {
		t.Fatal("missing task metadata must not fail the review")
	}
}

// blockingInference holds a pass open until released, so a test can observe the
// window between "inference started" and "run completed".
type blockingInference struct {
	started  chan struct{}
	release  chan struct{}
	response string
	once     sync.Once
}

func (b *blockingInference) Run(ctx context.Context, identity ReviewerIdentity, _ string, _ string) (*PromptResult, error) {
	b.once.Do(func() { close(b.started) })
	select {
	case <-b.release:
		return &PromptResult{Response: b.response, Model: identity.Model}, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func newBlockingHarness(t *testing.T, response string) (*Runner, *fakeStore, *blockingInference) {
	t.Helper()
	store := newFakeStore()
	inference := &blockingInference{
		started:  make(chan struct{}),
		release:  make(chan struct{}),
		response: response,
	}
	runner := NewRunner(RunnerDeps{
		Store:     store,
		Resolver:  NewResolver(nil, &fakeUtility{found: true, enabled: true, agentID: "a", model: "m"}, nil),
		Changes:   &fakeChangeSource{uncommitted: map[string]any{"a.go": fileEntry("a.go", "@@ -1 +1,2 @@\n x\n+y\n", "", "")}},
		Inference: inference,
		Prompts:   &fakePrompts{},
		Sessions:  &fakeSessions{sessionID: "sess-1"},
		Logger:    testLogger(t),
	})
	runner.Start(context.Background())
	t.Cleanup(func() {
		close(inference.release)
		runner.Stop()
	})
	return runner, store, inference
}

// TestRunner_CancelStopsInferenceAndKeepsRunCancelled covers the bug where
// cancelling only marked the DB row: the goroutine finished anyway and its
// completion overwrote the cancelled status, publishing declined findings.
func TestRunner_CancelStopsInferenceAndKeepsRunCancelled(t *testing.T) {
	runner, store, inference := newBlockingHarness(t, okResponse("a.go", 2))

	run, err := runner.Launch(context.Background(), RunRequest{TaskID: "task-cancel"})
	if err != nil {
		t.Fatalf("Launch: %v", err)
	}
	<-inference.started

	if _, err := runner.Cancel(context.Background(), run.ID); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	// The pass must unwind on its own now that its context is cancelled.
	deadline := time.After(5 * time.Second)
	for store.statusOf(run.ID) != models.ReviewRunCancelled {
		select {
		case <-deadline:
			t.Fatalf("run did not settle as cancelled, got %q", store.statusOf(run.ID))
		case <-time.After(10 * time.Millisecond):
		}
	}
	runner.Stop()
	if got := store.statusOf(run.ID); got != models.ReviewRunCancelled {
		t.Fatalf("a cancelled run must stay cancelled after the pass unwinds, got %q", got)
	}
	if _, published := store.lastPublished(); published {
		t.Fatal("a cancelled review must not publish findings")
	}
}

// TestRunner_failSkipsCanceledCause reproduces CI flake where cancel raced
// CancelRun: inference errors wrapped with %v lost context.Canceled, so fail()
// called FailRun before CancelRun and left the row terminal-failed. fail must
// treat a cancelled run context as cancel even when the cause chain is broken.
func TestRunner_failSkipsCanceledCause(t *testing.T) {
	runner, store, _ := newBlockingHarness(t, okResponse("a.go", 2))
	runner.Stop() // no live pass; we call fail directly

	run, err := store.CreateRun(context.Background(), taskservice.CreateRunRequest{
		TaskID: "task-fail-cancel",
	})
	if err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	if _, err := store.MarkRunRunning(context.Background(), run.ID); err != nil {
		t.Fatalf("MarkRunRunning: %v", err)
	}

	// Broken wrapper: historical reviewBatches path used %v and dropped Canceled.
	broken := fmt.Errorf("%w: %v", ErrExecutionFailed, context.Canceled)
	if errors.Is(broken, context.Canceled) {
		t.Fatal("precondition: broken wrapper must not preserve context.Canceled")
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_ = runner.fail(ctx, run.ID, broken, time.Now())

	if got := store.statusOf(run.ID); got == models.ReviewRunFailed {
		t.Fatalf("fail must not mark a cancelled run failed, got %q", got)
	}
	if failure, ok := store.lastFailure(); ok {
		t.Fatalf("fail must not record FailRun on cancel, got %#v", failure)
	}
}

func singleBatchPlan() BatchPlan {
	return BatchPlan{Batches: [][]ChangedFile{{{Path: "a.go", Diff: "@@ -1 +1,2 @@\n x\n+y\n"}}}}
}

// reviewBatches must keep context.Canceled reachable through its %w wrapper so
// fail() can leave a user cancel as cancelled. Reverting the inference wrapper
// to %v would drop it and break this contract.
func TestRunner_reviewBatchesPreservesCanceled(t *testing.T) {
	runner := NewRunner(RunnerDeps{
		Store:     newFakeStore(),
		Resolver:  NewResolver(nil, &fakeUtility{found: true, enabled: true, agentID: "a", model: "m"}, nil),
		Changes:   &fakeChangeSource{},
		Inference: &fakeInference{err: fmt.Errorf("inference aborted: %w", context.Canceled)},
		Prompts:   &fakePrompts{},
		Sessions:  &fakeSessions{sessionID: "sess-1"},
		Logger:    testLogger(t),
	})

	_, err := runner.reviewBatches(context.Background(), singleBatchPlan(),
		ReviewerIdentity{Model: "m"}, "sess-1", PromptContext{})
	if !errors.Is(err, ErrExecutionFailed) {
		t.Fatalf("expected ErrExecutionFailed in the chain, got %v", err)
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("reviewBatches must keep context.Canceled reachable, got %v", err)
	}
}

// reviewBatches must keep the underlying parse error reachable through its %w
// wrapper on the unparseable-response path.
func TestRunner_reviewBatchesPreservesParseError(t *testing.T) {
	h := newRunnerHarness(t, map[string]any{}, []string{"no findings here at all"})

	_, err := h.runner.reviewBatches(context.Background(), singleBatchPlan(),
		ReviewerIdentity{Model: "m"}, "sess-1", PromptContext{})
	if !errors.Is(err, ErrUnparseableResponse) {
		t.Fatalf("expected ErrUnparseableResponse in the chain, got %v", err)
	}
}

// TestRunner_ConcurrentLaunchesCreateOneRun covers the race where the DB check
// and the in-memory claim were not atomic, leaving the loser's pending run row
// orphaned with no goroutine behind it.
func TestRunner_ConcurrentLaunchesCreateOneRun(t *testing.T) {
	runner, store, inference := newBlockingHarness(t, okResponse("a.go", 2))

	const launches = 8
	var wg sync.WaitGroup
	errs := make([]error, launches)
	for i := range launches {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			_, errs[idx] = runner.Launch(context.Background(), RunRequest{TaskID: "task-race"})
		}(i)
	}
	wg.Wait()
	<-inference.started

	if got := store.runCount(); got != 1 {
		t.Fatalf("concurrent launches must create exactly one run row, got %d", got)
	}
	// Losers either rejoin the winner's run or report the in-flight pass; none
	// may invent a second one.
	for i, err := range errs {
		if err != nil && !errors.Is(err, ErrExecutionFailed) {
			t.Fatalf("launch %d returned an unexpected error: %v", i, err)
		}
	}
}

// TestRunner_RunWaitsOnlyForItsOwnPass covers the suggestion that Run() blocked
// on the shared WaitGroup and therefore on every other task's review.
func TestRunner_RunWaitsOnlyForItsOwnPass(t *testing.T) {
	store := newFakeStore()
	slow := &blockingInference{started: make(chan struct{}), release: make(chan struct{})}
	changes := &fakeChangeSource{uncommitted: map[string]any{"a.go": fileEntry("a.go", "@@ -1 +1,2 @@\n x\n+y\n", "", "")}}
	resolver := NewResolver(nil, &fakeUtility{found: true, enabled: true, agentID: "a", model: "m"}, nil)

	slowRunner := NewRunner(RunnerDeps{
		Store: store, Resolver: resolver, Changes: changes, Inference: slow,
		Prompts: &fakePrompts{}, Sessions: &fakeSessions{sessionID: "s"}, Logger: testLogger(t),
	})
	slowRunner.Start(context.Background())
	defer func() { close(slow.release); slowRunner.Stop() }()

	if _, err := slowRunner.Launch(context.Background(), RunRequest{TaskID: "task-slow"}); err != nil {
		t.Fatalf("Launch slow: %v", err)
	}
	<-slow.started

	// A second task on the same runner must complete without waiting for the
	// still-blocked first pass.
	fastStore := newFakeStore()
	fastRunner := NewRunner(RunnerDeps{
		Store: fastStore, Resolver: resolver, Changes: changes,
		Inference: &fakeInference{responses: []string{okResponse("a.go", 2)}},
		Prompts:   &fakePrompts{}, Sessions: &fakeSessions{sessionID: "s"}, Logger: testLogger(t),
	})
	fastRunner.Start(context.Background())
	defer fastRunner.Stop()

	finished := make(chan struct{})
	go func() {
		defer close(finished)
		if _, err := fastRunner.Run(context.Background(), RunRequest{TaskID: "task-fast"}); err != nil {
			t.Errorf("Run fast: %v", err)
		}
	}()
	select {
	case <-finished:
	case <-time.After(10 * time.Second):
		t.Fatal("Run() blocked on an unrelated task's pass")
	}
	if _, ok := fastStore.lastCompleted(); !ok {
		t.Fatal("expected the fast pass to complete")
	}
}

func TestRunner_CancelUnknownRunSurfacesNotFound(t *testing.T) {
	h := newRunnerHarness(t, map[string]any{}, []string{""})
	if _, err := h.runner.Cancel(context.Background(), "nope"); !errors.Is(err, models.ErrTaskReviewRunNotFound) {
		t.Fatalf("expected ErrTaskReviewRunNotFound, got %v", err)
	}
}
