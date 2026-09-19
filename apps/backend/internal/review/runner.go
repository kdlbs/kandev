package review

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/models"
	taskservice "github.com/kandev/kandev/internal/task/service"
)

// PromptContext carries the task-level facts the review prompt interpolates.
type PromptContext struct {
	TaskTitle       string
	TaskDescription string
	BranchName      string
	BaseBranch      string
}

// PromptResult is one reviewer reply plus its accounting.
type PromptResult struct {
	Response       string
	Model          string
	PromptTokens   int
	ResponseTokens int
	DurationMs     int
}

// Inference runs one reviewer prompt. Implementations wrap the session-bound
// agentctl inference path and the sessionless host-utility path; the runner does
// not care which, so both triggers share one execution substrate.
type Inference interface {
	Run(ctx context.Context, identity ReviewerIdentity, sessionID, prompt string) (*PromptResult, error)
}

// PromptBuilder renders the reviewer prompt for one batch of files. Backed by the
// `code-review` utility agent's stored (and user-editable) prompt template.
type PromptBuilder interface {
	Build(ctx context.Context, batch []ChangedFile, promptCtx PromptContext) (string, error)
}

// TaskContextLookup supplies the task-level prompt facts.
type TaskContextLookup interface {
	ReviewPromptContext(ctx context.Context, taskID, sessionID string) (PromptContext, error)
}

// SessionLookup resolves which session's workspace a run should read. A task can
// have several sessions; they share one workspace, so any live session will do.
type SessionLookup interface {
	ReviewSessionID(ctx context.Context, taskID string) (string, error)
}

// Store is the persistence surface the runner drives. Satisfied by
// *taskservice.ReviewService.
type Store interface {
	CreateRun(ctx context.Context, req taskservice.CreateRunRequest) (*models.TaskReviewRun, error)
	ActiveRun(ctx context.Context, taskID string) (*models.TaskReviewRun, error)
	MarkRunRunning(ctx context.Context, runID string) (*models.TaskReviewRun, error)
	CompleteRun(ctx context.Context, req taskservice.CompleteRunRequest) (*models.TaskReviewRun, error)
	FailRun(ctx context.Context, runID, code, message string, durationMs int) (*models.TaskReviewRun, error)
	CancelRun(ctx context.Context, runID string) (*models.TaskReviewRun, error)
	PublishFindings(ctx context.Context, req taskservice.PublishFindingsRequest) (*models.TaskReviewRun, []*models.TaskReviewFinding, error)
	// FindRunByEntryID returns the run already created for a step-transition
	// ledger entry, or nil when none exists yet. Backs the durable dedup for
	// AC-OFFICE-STEP-ENTRY-001.10: a redelivery of the same entry (after
	// completion, or after a restart) must rejoin the existing run instead of
	// launching a second one.
	FindRunByEntryID(ctx context.Context, entryID string) (*models.TaskReviewRun, error)
}

// RunRequest describes a review pass to start.
type RunRequest struct {
	TaskID         string
	SessionID      string
	RepositoryID   string
	AgentProfileID string
	Trigger        models.ReviewRunTrigger
	WorkflowStepID string
	// EntryID is the step-transition ledger row identifier of the step entry
	// that requested this run, when triggered by the run_code_review
	// step-entry action. Empty for manual/MCP-triggered runs.
	EntryID string
}

// Runner orchestrates review passes.
//
// Goroutine ownership: Start registers the runner's background context and
// Stop cancels it and waits for in-flight passes to drain, per the ownership
// rules in apps/backend/AGENTS.md. A pass detached by Launch is therefore always
// joined at shutdown rather than leaked.
type Runner struct {
	store       Store
	resolver    *Resolver
	changes     ChangeSource
	inference   Inference
	prompts     PromptBuilder
	taskContext TaskContextLookup
	sessions    SessionLookup
	logger      *logger.Logger

	// budgetBytes overrides PromptBudgetBytes; tests set it to force batching.
	budgetBytes int

	mu       sync.Mutex
	inFlight map[string]*inFlightRun
	ctx      context.Context
	cancel   context.CancelFunc
	wg       sync.WaitGroup
	started  bool
}

// inFlightRun is the live state of one review pass. The cancel handle is what
// makes a user-requested cancel actually stop inference: without it, cancelling
// only marks the DB row and the goroutine keeps going and overwrites it.
type inFlightRun struct {
	runID  string
	cancel context.CancelFunc
	done   chan struct{}
}

// RunnerDeps groups the runner's collaborators.
type RunnerDeps struct {
	Store       Store
	Resolver    *Resolver
	Changes     ChangeSource
	Inference   Inference
	Prompts     PromptBuilder
	TaskContext TaskContextLookup
	Sessions    SessionLookup
	Logger      *logger.Logger
	BudgetBytes int
}

// NewRunner builds a Runner.
func NewRunner(deps RunnerDeps) *Runner {
	return &Runner{
		store:       deps.Store,
		resolver:    deps.Resolver,
		changes:     deps.Changes,
		inference:   deps.Inference,
		prompts:     deps.Prompts,
		taskContext: deps.TaskContext,
		sessions:    deps.Sessions,
		logger:      deps.Logger.WithFields(zap.String("component", "review-runner")),
		budgetBytes: deps.BudgetBytes,
		inFlight:    make(map[string]*inFlightRun),
	}
}

// Start registers the runner's background context.
func (r *Runner) Start(ctx context.Context) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.started {
		return
	}
	r.ctx, r.cancel = context.WithCancel(ctx)
	r.started = true
}

// Stop cancels in-flight passes and waits for them to drain. Idempotent.
func (r *Runner) Stop() {
	r.mu.Lock()
	cancel := r.cancel
	r.started = false
	r.cancel = nil
	r.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	r.wg.Wait()
}

// runTimeout bounds a single review pass so a wedged provider cannot pin the
// in-flight slot for a task forever.
const runTimeout = 10 * time.Minute

// Launch starts a review pass in the background and returns the pending run, so
// the caller's request does not block on inference.
//
// A task that already has a pending or running pass returns that run untouched —
// re-clicking Review must not fan out into duplicate provider calls.
func (r *Runner) Launch(ctx context.Context, req RunRequest) (*models.TaskReviewRun, error) {
	run, _, err := r.launch(ctx, req)
	return run, err
}

// launch does the work of Launch and additionally returns a channel closed when
// the pass finishes, so a synchronous caller can wait for its own run instead of
// every run in the process.
func (r *Runner) launch(ctx context.Context, req RunRequest) (*models.TaskReviewRun, <-chan struct{}, error) {
	if req.TaskID == "" {
		return nil, nil, taskservice.ErrTaskIDRequired
	}

	// Claim the task's slot before touching the DB. Claiming after CreateRun
	// would let two concurrent launches both insert a run and leave the loser's
	// row pending forever with no goroutine behind it.
	if !r.claim(req.TaskID) {
		existing, err := r.store.ActiveRun(ctx, req.TaskID)
		if err != nil {
			return nil, nil, err
		}
		if existing == nil {
			// Claimed by a launch that has not created its row yet; report the
			// in-flight pass rather than starting a competing one.
			return nil, nil, fmt.Errorf("%w: a review is already starting for this task", ErrExecutionFailed)
		}
		return existing, nil, nil
	}
	// Every early return below must free the slot; only the launched goroutine
	// keeps it.
	launched := false
	defer func() {
		if !launched {
			r.release(req.TaskID)
		}
	}()

	// A pass that survived a restart has a DB row but no in-memory claim, so the
	// claim above succeeds. Rejoin it instead of starting a second one.
	if existing, err := r.store.ActiveRun(ctx, req.TaskID); err == nil && existing != nil {
		return existing, nil, nil
	}

	// A redelivered step entry must rejoin the run it already created — even a
	// completed one — rather than start a duplicate. The ActiveRun check above
	// only catches a still-pending/running pass, so a redelivery after
	// completion needs its own check keyed on entry identity.
	if req.EntryID != "" {
		existing, err := r.store.FindRunByEntryID(ctx, req.EntryID)
		if err != nil {
			return nil, nil, err
		}
		if existing != nil {
			return existing, nil, nil
		}
	}

	sessionID, err := r.resolveSessionID(ctx, req)
	if err != nil {
		return nil, nil, err
	}
	req.SessionID = sessionID

	// Resolve the reviewer and read the diff before creating a run row: both
	// "no capable agent" and "nothing to review" are conditions the user should
	// see immediately, and neither deserves a failed run in the history.
	identity, err := r.resolver.Resolve(ctx, req.AgentProfileID)
	if err != nil {
		return nil, nil, err
	}
	files, err := CollectChanges(ctx, r.changes, sessionID, req.RepositoryID)
	if err != nil {
		return nil, nil, err
	}
	if len(files) == 0 {
		return nil, nil, fmt.Errorf("%w: the task has no changed files to review", ErrNoChanges)
	}

	run, err := r.store.CreateRun(ctx, taskservice.CreateRunRequest{
		TaskID:         req.TaskID,
		SessionID:      sessionID,
		Trigger:        req.Trigger,
		WorkflowStepID: req.WorkflowStepID,
		AgentID:        identity.AgentID,
		Model:          identity.Model,
		EntryID:        req.EntryID,
	})
	if err != nil {
		return nil, nil, err
	}

	runCtx, cancel := context.WithTimeout(r.backgroundContext(), runTimeout)
	done := r.attach(req.TaskID, run.ID, cancel)
	launched = true

	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		defer r.release(req.TaskID)
		defer cancel()
		if err := r.execute(runCtx, req, run.ID, identity, files); err != nil {
			r.logger.Warn("review run failed",
				zap.String("task_id", req.TaskID), zap.String("run_id", run.ID), zap.Error(err))
		}
	}()
	return run, done, nil
}

// Cancel stops a review pass and records the cancellation.
//
// Cancelling the live context is the half that matters: marking only the DB row
// would let the goroutine finish inference and overwrite the cancelled status
// with a completed one, publishing findings the user already declined.
func (r *Runner) Cancel(ctx context.Context, runID string) (*models.TaskReviewRun, error) {
	r.cancelInFlight(runID)
	return r.store.CancelRun(ctx, runID)
}

// backgroundContext returns the runner's own context so a detached pass outlives
// the originating request but still stops on shutdown.
func (r *Runner) backgroundContext() context.Context {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.ctx != nil {
		return r.ctx
	}
	return context.Background()
}

// claim takes the task's single review slot before any DB row exists.
//
// Claiming first is what closes the duplicate-row race: if the slot were taken
// only after CreateRun, two concurrent launches could both create a run and the
// loser would return a pending row with no goroutine behind it, leaving a review
// that never completes.
func (r *Runner) claim(taskID string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, busy := r.inFlight[taskID]; busy {
		return false
	}
	r.inFlight[taskID] = &inFlightRun{done: make(chan struct{})}
	return true
}

// attach records the run id and cancel handle on an already-claimed slot and
// returns its done channel.
func (r *Runner) attach(taskID, runID string, cancel context.CancelFunc) <-chan struct{} {
	r.mu.Lock()
	defer r.mu.Unlock()
	entry := r.inFlight[taskID]
	if entry == nil {
		return nil
	}
	entry.runID = runID
	entry.cancel = cancel
	return entry.done
}

// release frees the task's slot and wakes anyone waiting on the pass.
func (r *Runner) release(taskID string) {
	r.mu.Lock()
	entry := r.inFlight[taskID]
	delete(r.inFlight, taskID)
	r.mu.Unlock()
	if entry != nil {
		close(entry.done)
	}
}

// cancelInFlight cancels the live pass for runID, if this process owns it.
// Returns true when a goroutine was actually signalled.
func (r *Runner) cancelInFlight(runID string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, entry := range r.inFlight {
		if entry.runID == runID && entry.cancel != nil {
			entry.cancel()
			return true
		}
	}
	return false
}

// resolveSessionID prefers the caller's session and otherwise asks the lookup.
func (r *Runner) resolveSessionID(ctx context.Context, req RunRequest) (string, error) {
	if req.SessionID != "" {
		return req.SessionID, nil
	}
	if r.sessions == nil {
		return "", fmt.Errorf("%w: no session is available for this task", ErrWorkspaceUnavailable)
	}
	sessionID, err := r.sessions.ReviewSessionID(ctx, req.TaskID)
	if err != nil {
		return "", fmt.Errorf("%w: resolve session: %v", ErrWorkspaceUnavailable, err)
	}
	if sessionID == "" {
		return "", fmt.Errorf("%w: the task has no session to read changes from", ErrWorkspaceUnavailable)
	}
	return sessionID, nil
}

// Run performs a review pass synchronously. Used by tests and by callers that
// want to await the result; Launch is the normal entry point.
func (r *Runner) Run(ctx context.Context, req RunRequest) (*models.TaskReviewRun, error) {
	run, done, err := r.launch(ctx, req)
	if err != nil {
		return nil, err
	}
	// Wait only for this task's pass. Waiting on the shared WaitGroup would
	// block on every other task's review too.
	if done != nil {
		<-done
	}
	return run, nil
}

// execute is the body of one pass. Every failure path records the outcome on the
// run row before returning, so the UI always has a reason to show.
func (r *Runner) execute(ctx context.Context, req RunRequest, runID string, identity ReviewerIdentity, files []ChangedFile) error {
	started := time.Now()
	if _, err := r.store.MarkRunRunning(ctx, runID); err != nil {
		return err
	}

	promptCtx, err := r.promptContext(ctx, req)
	if err != nil {
		return r.fail(ctx, runID, err, started)
	}

	plan := PlanBatches(files, r.budgetBytes)
	accumulated, err := r.reviewBatches(ctx, plan, identity, req.SessionID, promptCtx)
	if err != nil {
		return r.fail(ctx, runID, err, started)
	}

	// A cancel that landed while inference was running must win: publishing here
	// would store findings the user already declined, and CompleteRun below would
	// be a no-op against the cancelled row, leaving orphaned findings.
	if err := ctx.Err(); err != nil {
		r.logger.Debug("review run cancelled before publishing",
			zap.String("run_id", runID), zap.Error(err))
		return nil
	}

	index := FileByKey(files)
	inputs := anchorFindings(accumulated.findings, index)
	summary := buildRunSummary(accumulated, plan, len(inputs))

	if len(inputs) > 0 {
		if _, _, pubErr := r.store.PublishFindings(ctx, taskservice.PublishFindingsRequest{
			TaskID:    req.TaskID,
			RunID:     runID,
			SessionID: req.SessionID,
			Trigger:   req.Trigger,
			Summary:   summary,
			Findings:  inputs,
		}); pubErr != nil {
			return r.fail(ctx, runID, pubErr, started)
		}
	}

	_, err = r.store.CompleteRun(ctx, taskservice.CompleteRunRequest{
		RunID:           runID,
		Summary:         summary,
		FindingCount:    len(inputs),
		FileCount:       plan.FileCount(),
		RepositoryCount: RepositoryCount(files),
		PromptTokens:    accumulated.promptTokens,
		ResponseTokens:  accumulated.responseTokens,
		DurationMs:      int(time.Since(started).Milliseconds()),
	})
	return err
}

func (r *Runner) promptContext(ctx context.Context, req RunRequest) (PromptContext, error) {
	if r.taskContext == nil {
		return PromptContext{}, nil
	}
	promptCtx, err := r.taskContext.ReviewPromptContext(ctx, req.TaskID, req.SessionID)
	if err != nil {
		// Task metadata is nice-to-have context, not a reason to fail a review.
		r.logger.Warn("review prompt context unavailable",
			zap.String("task_id", req.TaskID), zap.Error(err))
		return PromptContext{}, nil
	}
	return promptCtx, nil
}

// accumulator collects results across prompt batches.
type accumulator struct {
	findings       []FindingInput
	summaries      []string
	rejected       int
	promptTokens   int
	responseTokens int
}

func (r *Runner) reviewBatches(ctx context.Context, plan BatchPlan, identity ReviewerIdentity, sessionID string, promptCtx PromptContext) (accumulator, error) {
	acc := accumulator{}
	for i, batch := range plan.Batches {
		if err := ctx.Err(); err != nil {
			// Keep context.Canceled / DeadlineExceeded in the chain so fail() can
			// leave a user cancel as cancelled instead of overwriting it failed.
			return acc, fmt.Errorf("%w: review cancelled after batch %d: %w", ErrExecutionFailed, i, err)
		}
		prompt, err := r.prompts.Build(ctx, batch, promptCtx)
		if err != nil {
			return acc, fmt.Errorf("%w: build prompt: %w", ErrExecutionFailed, err)
		}
		result, err := r.inference.Run(ctx, identity, sessionID, prompt)
		if err != nil {
			// Preserve the inference error chain (esp. context cancellation). Using
			// %v here used to drop context.Canceled, so fail() marked cancelled runs
			// as failed when CancelRun lost the race with FailRun.
			return acc, fmt.Errorf("%w: %w", ErrExecutionFailed, err)
		}
		if result == nil {
			return acc, fmt.Errorf("%w: reviewer returned no result", ErrExecutionFailed)
		}
		acc.promptTokens += result.PromptTokens
		acc.responseTokens += result.ResponseTokens

		parsed, err := ParseFindings(result.Response)
		if err != nil {
			// One unreadable batch fails the run: reporting a partial review as
			// complete would read as an all-clear for the files we could not parse.
			return acc, fmt.Errorf("%w: batch %d: %w", ErrUnparseableResponse, i+1, err)
		}
		acc.findings = append(acc.findings, parsed.Findings...)
		acc.rejected += parsed.Rejected
		if parsed.Summary != "" {
			acc.summaries = append(acc.summaries, parsed.Summary)
		}
	}
	return acc, nil
}

func (r *Runner) fail(ctx context.Context, runID string, cause error, started time.Time) error {
	// An explicit cancel already recorded the terminal state, so re-reporting it
	// as a failure would only churn the row (and would overwrite it on any store
	// without a terminal guard). A deadline is different: nobody recorded that.
	// Check both the wrapped cause and the run context: wrappers historically used
	// %v and dropped context.Canceled from the chain.
	if isReviewCanceled(cause) || isReviewCanceled(ctx.Err()) {
		r.logger.Debug("review run unwound after cancellation", zap.String("run_id", runID))
		if cause != nil {
			return cause
		}
		return ctx.Err()
	}
	code := CodeFor(cause)
	if errors.Is(cause, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
		code = CodeCancelled
	}
	// Persist the failure on a context detached from the (possibly cancelled)
	// run context, otherwise the run would stay stuck as "running" forever.
	failCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	if _, err := r.store.FailRun(failCtx, runID, code, cause.Error(), int(time.Since(started).Milliseconds())); err != nil {
		r.logger.Error("record review run failure", zap.String("run_id", runID), zap.Error(err))
	}
	return cause
}

// isReviewCanceled reports a user/cancel signal (context.Canceled), not a
// deadline. DeadlineExceeded also satisfies errors.Is(..., Canceled) in some
// chains, so it is excluded explicitly.
func isReviewCanceled(err error) bool {
	return err != nil && errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded)
}

// anchorFindings attaches the authoritative diff hash and anchor text from the
// change set to each reported finding, and drops findings naming a file the
// reviewer was never shown.
//
// The hash must come from the change set rather than the reviewer: it is what
// staleness is computed against, so it has to describe the diff that was
// actually reviewed.
func anchorFindings(reported []FindingInput, index map[string]ChangedFile) []taskservice.ReviewFindingInput {
	inputs := make([]taskservice.ReviewFindingInput, 0, len(reported))
	for _, f := range reported {
		file, ok := lookupReportedFile(f, index)
		if !ok {
			continue
		}
		inputs = append(inputs, taskservice.ReviewFindingInput{
			RepositoryID:   file.RepositoryID,
			RepositoryName: file.RepositoryName,
			FilePath:       file.Path,
			StartLine:      f.Line,
			EndLine:        f.LineEnd,
			Side:           f.Side,
			Severity:       f.Severity,
			Category:       f.Category,
			Title:          f.Title,
			Body:           f.Body,
			Suggestion:     f.Suggestion,
			AnchorText:     TruncateAnchorText(ExtractAnchorText(file.Diff, f.Line, f.LineEnd)),
			FileDiffHash:   file.DiffHash,
		})
	}
	return inputs
}

// lookupReportedFile matches a reported (repo, file) pair against the change set,
// tolerating a reviewer that omitted or guessed the repository prefix.
func lookupReportedFile(f FindingInput, index map[string]ChangedFile) (ChangedFile, bool) {
	if f.Repo != "" {
		if file, ok := index[f.Repo+fileKeySep+f.File]; ok {
			return file, true
		}
	}
	if file, ok := index[f.File]; ok {
		return file, true
	}
	// Unprefixed path in a multi-repo change set: accept it only when exactly one
	// repository contains that path, so a finding is never attributed to the
	// wrong repository.
	var match ChangedFile
	found := 0
	for _, file := range index {
		if file.Path == f.File {
			match = file
			found++
		}
	}
	if found == 1 {
		return match, true
	}
	return ChangedFile{}, false
}

// buildRunSummary combines the reviewer's prose with the mechanical facts the
// user needs to trust the result: what was skipped and what was rejected.
func buildRunSummary(acc accumulator, plan BatchPlan, stored int) string {
	parts := make([]string, 0, 4)
	if joined := strings.TrimSpace(strings.Join(acc.summaries, "\n\n")); joined != "" {
		parts = append(parts, joined)
	}
	if len(plan.Skipped) > 0 {
		names := make([]string, 0, len(plan.Skipped))
		for _, f := range plan.Skipped {
			names = append(names, f.Key())
		}
		parts = append(parts, fmt.Sprintf("Skipped %d file(s) whose diff was too large to review: %s.",
			len(plan.Skipped), strings.Join(names, ", ")))
	}
	if acc.rejected > 0 {
		parts = append(parts, fmt.Sprintf("Discarded %d malformed finding(s) returned by the reviewer.", acc.rejected))
	}
	if dropped := len(acc.findings) - stored; dropped > 0 {
		parts = append(parts, fmt.Sprintf("Discarded %d finding(s) anchored to files outside the reviewed change set.", dropped))
	}
	return strings.Join(parts, " ")
}
