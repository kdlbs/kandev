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
	PublishFindings(ctx context.Context, req taskservice.PublishFindingsRequest) (*models.TaskReviewRun, []*models.TaskReviewFinding, error)
}

// RunRequest describes a review pass to start.
type RunRequest struct {
	TaskID         string
	SessionID      string
	RepositoryID   string
	AgentProfileID string
	Trigger        models.ReviewRunTrigger
	WorkflowStepID string
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
	inFlight map[string]string // taskID -> runID
	ctx      context.Context
	cancel   context.CancelFunc
	wg       sync.WaitGroup
	started  bool
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
		inFlight:    make(map[string]string),
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
	if req.TaskID == "" {
		return nil, taskservice.ErrTaskIDRequired
	}
	if existing, err := r.store.ActiveRun(ctx, req.TaskID); err == nil && existing != nil {
		return existing, nil
	}

	sessionID, err := r.resolveSessionID(ctx, req)
	if err != nil {
		return nil, err
	}
	req.SessionID = sessionID

	// Resolve the reviewer and read the diff before creating a run row: both
	// "no capable agent" and "nothing to review" are conditions the user should
	// see immediately, and neither deserves a failed run in the history.
	identity, err := r.resolver.Resolve(ctx, req.AgentProfileID)
	if err != nil {
		return nil, err
	}
	files, err := CollectChanges(ctx, r.changes, sessionID, req.RepositoryID)
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("%w: the task has no changed files to review", ErrNoChanges)
	}

	run, err := r.store.CreateRun(ctx, taskservice.CreateRunRequest{
		TaskID:         req.TaskID,
		SessionID:      sessionID,
		Trigger:        req.Trigger,
		WorkflowStepID: req.WorkflowStepID,
		AgentID:        identity.AgentID,
		Model:          identity.Model,
	})
	if err != nil {
		return nil, err
	}
	if !r.reserve(req.TaskID, run.ID) {
		return run, nil
	}

	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		defer r.release(req.TaskID)
		runCtx, cancel := context.WithTimeout(r.backgroundContext(), runTimeout)
		defer cancel()
		if err := r.execute(runCtx, req, run.ID, identity, files); err != nil {
			r.logger.Warn("review run failed",
				zap.String("task_id", req.TaskID), zap.String("run_id", run.ID), zap.Error(err))
		}
	}()
	return run, nil
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

func (r *Runner) reserve(taskID, runID string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, busy := r.inFlight[taskID]; busy {
		return false
	}
	r.inFlight[taskID] = runID
	return true
}

func (r *Runner) release(taskID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.inFlight, taskID)
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
	run, err := r.Launch(ctx, req)
	if err != nil {
		return nil, err
	}
	r.wg.Wait()
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
			return acc, fmt.Errorf("%w: review cancelled after batch %d", ErrExecutionFailed, i)
		}
		prompt, err := r.prompts.Build(ctx, batch, promptCtx)
		if err != nil {
			return acc, fmt.Errorf("%w: build prompt: %v", ErrExecutionFailed, err)
		}
		result, err := r.inference.Run(ctx, identity, sessionID, prompt)
		if err != nil {
			return acc, fmt.Errorf("%w: %v", ErrExecutionFailed, err)
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
			return acc, fmt.Errorf("%w: batch %d: %v", ErrUnparseableResponse, i+1, err)
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
	code := CodeFor(cause)
	if errors.Is(cause, context.Canceled) || errors.Is(cause, context.DeadlineExceeded) {
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
