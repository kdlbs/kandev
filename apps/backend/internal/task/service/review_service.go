package service

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/models"
)

// ErrInvalidReviewFinding is returned when a submitted finding fails structural
// validation. The MCP publish path surfaces it as a client error so the agent
// can correct and retry.
var ErrInvalidReviewFinding = errors.New("invalid review finding")

// ErrInvalidReviewFindingFilter is returned when a list_review_findings_kandev
// status or severity filter names a value outside the accepted set.
var ErrInvalidReviewFindingFilter = errors.New("invalid review finding filter")

// ErrReviewAccessDenied wraps an authorizer's denial for a review-finding read.
// It lets handlers distinguish "the caller cannot reach this task" from other
// failure modes (via errors.As) while surfacing the authorizer's own message
// verbatim, unlike PublishFindings which propagates the raw authorizer error.
type ErrReviewAccessDenied struct {
	Err error
}

func (e *ErrReviewAccessDenied) Error() string { return e.Err.Error() }
func (e *ErrReviewAccessDenied) Unwrap() error { return e.Err }

// ErrReviewRunNotFound / ErrReviewFindingNotFound re-export the model sentinels
// so handlers in this package can match without importing models.
var (
	ErrReviewRunNotFound     = models.ErrTaskReviewRunNotFound
	ErrReviewFindingNotFound = models.ErrTaskReviewFindingNotFound
)

// Event payload keys, hoisted to constants to satisfy goconst.
const (
	rvFieldTaskID     = "task_id"
	rvFieldRunID      = "run_id"
	rvFieldFinding    = "finding"
	rvFieldFindings   = "findings"
	rvFieldRun        = "run"
	rvFieldSuperseded = "superseded_ids"
)

// reviewRepo is the minimal repository surface ReviewService needs. The SQLite
// repository satisfies it; declared locally so the service does not depend on
// the full aggregate repository interface (same pattern as walkthroughRepo).
type reviewRepo interface {
	CreateTaskReviewRun(ctx context.Context, run *models.TaskReviewRun) error
	UpdateTaskReviewRun(ctx context.Context, run *models.TaskReviewRun) error
	GetTaskReviewRun(ctx context.Context, runID string) (*models.TaskReviewRun, error)
	ListTaskReviewRuns(ctx context.Context, taskID string, limit int) ([]*models.TaskReviewRun, error)
	ListActiveTaskReviewRuns(ctx context.Context, taskID string) ([]*models.TaskReviewRun, error)
	FindTaskReviewRunByEntryID(ctx context.Context, entryID string) (*models.TaskReviewRun, error)
	CreateTaskReviewFindings(ctx context.Context, findings []*models.TaskReviewFinding) error
	ListTaskReviewFindings(ctx context.Context, taskID string) ([]*models.TaskReviewFinding, error)
	GetTaskReviewFinding(ctx context.Context, findingID string) (*models.TaskReviewFinding, error)
	UpdateTaskReviewFindingStatus(ctx context.Context, findingID string, status models.ReviewFindingStatus, resolvedAt *time.Time) error
	DeleteSupersededTaskReviewFindings(ctx context.Context, taskID, runID string, keys []models.ReviewFindingKey) ([]string, error)
	DeleteTaskReviewByTask(ctx context.Context, taskID string) error
}

// ReviewService is the single write path for native code-review runs and
// findings. Every mutation persists first and then publishes on the event bus so
// the Review surface updates without polling.
type ReviewService struct {
	repo     reviewRepo
	eventBus bus.EventBus
	logger   *logger.Logger
	// authorizeTask gates cross-task finding writes by the task's workspace
	// ownership (opt-in auth). Nil = unscoped (internal callers such as the
	// built-in review runner / auth disabled). Mirrors PlanService and
	// WalkthroughService.
	authorizeTask func(ctx context.Context, taskID string) error
}

// NewReviewService creates a new code-review service.
func NewReviewService(repo reviewRepo, eventBus bus.EventBus, log *logger.Logger) *ReviewService {
	return &ReviewService{
		repo:     repo,
		eventBus: eventBus,
		logger:   log.WithFields(zap.String("component", "review-service")),
	}
}

// SetTaskAuthorizer wires the per-user task-access check (opt-in auth). The
// authorizer must return nil for contexts without a request identity. Mirrors
// PlanService / WalkthroughService so publish_review_findings_kandev honors an
// explicit cross-task task_id only within the caller's reach.
func (s *ReviewService) SetTaskAuthorizer(fn func(ctx context.Context, taskID string) error) {
	s.authorizeTask = fn
}

func (s *ReviewService) authorize(ctx context.Context, taskID string) error {
	if s.authorizeTask == nil {
		return nil
	}
	return s.authorizeTask(ctx, taskID)
}

// CreateRunRequest describes a new review pass.
type CreateRunRequest struct {
	TaskID         string
	SessionID      string
	Trigger        models.ReviewRunTrigger
	WorkflowStepID string
	AgentID        string
	Model          string
	// EntryID is the step-transition ledger row identifier of the step entry
	// that requested this run, when triggered by the run_code_review
	// step-entry action. Empty for manual/MCP-triggered runs.
	EntryID string
}

// CreateRun records a pending run and publishes it so the UI can show progress
// immediately, before any inference happens.
func (s *ReviewService) CreateRun(ctx context.Context, req CreateRunRequest) (*models.TaskReviewRun, error) {
	if req.TaskID == "" {
		return nil, ErrTaskIDRequired
	}
	run := &models.TaskReviewRun{
		TaskID:         req.TaskID,
		SessionID:      req.SessionID,
		Trigger:        req.Trigger,
		WorkflowStepID: req.WorkflowStepID,
		AgentID:        req.AgentID,
		Model:          req.Model,
		EntryID:        req.EntryID,
		Status:         models.ReviewRunPending,
	}
	if err := s.repo.CreateTaskReviewRun(ctx, run); err != nil {
		s.logger.Error("create review run", zap.String(rvFieldTaskID, req.TaskID), zap.Error(err))
		return nil, err
	}
	s.publishRun(ctx, run)
	return run, nil
}

// ActiveRun returns the task's newest pending/running run, or nil when the task
// has no pass in flight. Callers use it to rejoin an existing run rather than
// starting a duplicate.
func (s *ReviewService) ActiveRun(ctx context.Context, taskID string) (*models.TaskReviewRun, error) {
	if taskID == "" {
		return nil, ErrTaskIDRequired
	}
	runs, err := s.repo.ListActiveTaskReviewRuns(ctx, taskID)
	if err != nil {
		return nil, err
	}
	if len(runs) == 0 {
		return nil, nil
	}
	return runs[0], nil
}

// FindRunByEntryID returns the run created by the given step-entry ledger row,
// or nil when no run has been created for it yet. Satisfies review.Store so a
// redelivered run_code_review entry rejoins the run it already created instead
// of launching a duplicate (AC-OFFICE-STEP-ENTRY-001.10).
func (s *ReviewService) FindRunByEntryID(ctx context.Context, entryID string) (*models.TaskReviewRun, error) {
	if entryID == "" {
		return nil, nil
	}
	return s.repo.FindTaskReviewRunByEntryID(ctx, entryID)
}

// MarkRunRunning moves a run from pending to running.
func (s *ReviewService) MarkRunRunning(ctx context.Context, runID string) (*models.TaskReviewRun, error) {
	return s.mutateRun(ctx, runID, func(run *models.TaskReviewRun) {
		run.Status = models.ReviewRunRunning
	})
}

// CompleteRunRequest carries the accounting a finished run reports.
type CompleteRunRequest struct {
	RunID           string
	Summary         string
	FindingCount    int
	FileCount       int
	RepositoryCount int
	PromptTokens    int
	ResponseTokens  int
	DurationMs      int
}

// CompleteRun marks a run completed with its counts.
//
// A run the user already cancelled stays cancelled: without that guard a pass
// whose inference finished after the cancel would flip the status back to
// completed and publish findings the user declined.
func (s *ReviewService) CompleteRun(ctx context.Context, req CompleteRunRequest) (*models.TaskReviewRun, error) {
	now := time.Now().UTC()
	return s.mutateRunIfLive(ctx, req.RunID, func(run *models.TaskReviewRun) {
		run.Status = models.ReviewRunCompleted
		run.Summary = req.Summary
		run.FindingCount = req.FindingCount
		run.FileCount = req.FileCount
		run.RepositoryCount = req.RepositoryCount
		run.PromptTokens = req.PromptTokens
		run.ResponseTokens = req.ResponseTokens
		run.DurationMs = req.DurationMs
		run.ErrorCode = ""
		run.ErrorMessage = ""
		run.CompletedAt = &now
	})
}

// maxRunErrorMessage bounds the stored failure text. An unparseable reviewer
// reply is retained for debugging, but a multi-megabyte response must not land
// in the run row.
const maxRunErrorMessage = 2000

// FailRun marks a run failed with a client-facing code and a bounded message.
// Like CompleteRun, it leaves an already-terminal run alone.
func (s *ReviewService) FailRun(ctx context.Context, runID, code, message string, durationMs int) (*models.TaskReviewRun, error) {
	now := time.Now().UTC()
	trimmed := message
	if len(trimmed) > maxRunErrorMessage {
		trimmed = trimmed[:maxRunErrorMessage]
	}
	return s.mutateRunIfLive(ctx, runID, func(run *models.TaskReviewRun) {
		run.Status = models.ReviewRunFailed
		run.ErrorCode = code
		run.ErrorMessage = trimmed
		run.DurationMs = durationMs
		run.CompletedAt = &now
	})
}

// CancelRun marks a non-terminal run cancelled. Cancelling an already-terminal
// run is a no-op so the action is idempotent.
func (s *ReviewService) CancelRun(ctx context.Context, runID string) (*models.TaskReviewRun, error) {
	run, err := s.repo.GetTaskReviewRun(ctx, runID)
	if err != nil {
		return nil, err
	}
	if run.Status.IsTerminal() {
		return run, nil
	}
	now := time.Now().UTC()
	return s.mutateRun(ctx, runID, func(r *models.TaskReviewRun) {
		r.Status = models.ReviewRunCancelled
		r.CompletedAt = &now
	})
}

// mutateRunIfLive applies a transition only while the run is still non-terminal,
// so an external cancel wins over a late completion. Returns the untouched run
// (and publishes nothing) when it has already finished.
func (s *ReviewService) mutateRunIfLive(ctx context.Context, runID string, apply func(*models.TaskReviewRun)) (*models.TaskReviewRun, error) {
	if runID == "" {
		return nil, fmt.Errorf("%w: run id is required", ErrReviewRunNotFound)
	}
	existing, err := s.repo.GetTaskReviewRun(ctx, runID)
	if err != nil {
		return nil, err
	}
	if existing.Status.IsTerminal() {
		s.logger.Debug("skipping transition on a terminal review run",
			zap.String(rvFieldRunID, runID), zap.String("status", string(existing.Status)))
		return existing, nil
	}
	return s.mutateRun(ctx, runID, apply)
}

func (s *ReviewService) mutateRun(ctx context.Context, runID string, apply func(*models.TaskReviewRun)) (*models.TaskReviewRun, error) {
	if runID == "" {
		return nil, fmt.Errorf("%w: run id is required", ErrReviewRunNotFound)
	}
	run, err := s.repo.GetTaskReviewRun(ctx, runID)
	if err != nil {
		return nil, err
	}
	apply(run)
	if err := s.repo.UpdateTaskReviewRun(ctx, run); err != nil {
		s.logger.Error("update review run", zap.String(rvFieldRunID, runID), zap.Error(err))
		return nil, err
	}
	s.publishRun(ctx, run)
	return run, nil
}

// PublishFindingsRequest carries a batch of findings to store.
//
// RunID may be empty: the MCP path has no pre-created run, so the service
// creates a completed run with the given trigger and attributes the findings to
// it. That keeps every finding traceable to a run regardless of how it arrived.
type PublishFindingsRequest struct {
	TaskID    string
	RunID     string
	SessionID string
	Trigger   models.ReviewRunTrigger
	Summary   string
	Findings  []ReviewFindingInput
}

// ReviewFindingInput is one finding as submitted, before anchoring metadata is
// attached. Mirrors review.FindingInput and the MCP tool schema.
type ReviewFindingInput struct {
	RepositoryID   string
	RepositoryName string
	FilePath       string
	StartLine      int
	EndLine        int
	Side           string
	Severity       string
	Category       string
	Title          string
	Body           string
	Suggestion     string
	AnchorText     string
	FileDiffHash   string
}

// PublishFindings validates and stores a batch of findings.
//
// Validation is all-or-nothing: one malformed entry rejects the whole batch, so
// a caller never persists a partially-anchored review. Findings that repeat an
// earlier run's still-open anchor supersede it, keeping a re-review from listing
// the same issue twice while leaving human dispositions alone.
func (s *ReviewService) PublishFindings(ctx context.Context, req PublishFindingsRequest) (*models.TaskReviewRun, []*models.TaskReviewFinding, error) {
	if req.TaskID == "" {
		return nil, nil, ErrTaskIDRequired
	}
	if err := s.authorize(ctx, req.TaskID); err != nil {
		return nil, nil, err
	}
	if len(req.Findings) == 0 {
		return nil, nil, fmt.Errorf("%w: at least one finding is required", ErrInvalidReviewFinding)
	}

	run, err := s.resolvePublishRun(ctx, req)
	if err != nil {
		return nil, nil, err
	}

	findings := make([]*models.TaskReviewFinding, 0, len(req.Findings))
	for i, in := range req.Findings {
		finding, validateErr := buildReviewFinding(req.TaskID, run.ID, in)
		if validateErr != nil {
			return nil, nil, fmt.Errorf("%w: finding %d: %s", ErrInvalidReviewFinding, i+1, validateErr)
		}
		findings = append(findings, finding)
	}

	if err := s.repo.CreateTaskReviewFindings(ctx, findings); err != nil {
		s.logger.Error("create review findings", zap.String(rvFieldTaskID, req.TaskID), zap.Error(err))
		return nil, nil, err
	}
	superseded := s.supersedePriorFindings(ctx, req.TaskID, run.ID, findings)
	s.publishFindings(ctx, req.TaskID, run.ID, findings, superseded)
	return run, findings, nil
}

// resolvePublishRun returns the run the findings belong to, creating a completed
// one when the caller has none (the MCP path).
func (s *ReviewService) resolvePublishRun(ctx context.Context, req PublishFindingsRequest) (*models.TaskReviewRun, error) {
	if req.RunID != "" {
		return s.repo.GetTaskReviewRun(ctx, req.RunID)
	}
	now := time.Now().UTC()
	trigger := req.Trigger
	if trigger == "" {
		trigger = models.ReviewTriggerAgent
	}
	run := &models.TaskReviewRun{
		TaskID:       req.TaskID,
		SessionID:    req.SessionID,
		Trigger:      trigger,
		Status:       models.ReviewRunCompleted,
		Summary:      strings.TrimSpace(req.Summary),
		FindingCount: len(req.Findings),
		CompletedAt:  &now,
	}
	if err := s.repo.CreateTaskReviewRun(ctx, run); err != nil {
		return nil, err
	}
	s.publishRun(ctx, run)
	return run, nil
}

// supersedePriorFindings is best-effort: the new findings are already stored, so
// a failure to prune duplicates is logged rather than failing the publish.
// supersedePriorFindings prunes duplicate anchors and returns the ids removed so
// the publish event can tell connected clients which findings to drop.
// Best-effort: the new findings are already stored, so a prune failure is logged
// rather than failing the publish.
func (s *ReviewService) supersedePriorFindings(ctx context.Context, taskID, runID string, findings []*models.TaskReviewFinding) []string {
	keys := supersedeKeys(findings)
	deleted, err := s.repo.DeleteSupersededTaskReviewFindings(ctx, taskID, runID, keys)
	if err != nil {
		s.logger.Warn("supersede prior review findings",
			zap.String(rvFieldTaskID, taskID), zap.String(rvFieldRunID, runID), zap.Error(err))
		return nil
	}
	if len(deleted) > 0 {
		s.logger.Debug("superseded prior review findings",
			zap.String(rvFieldTaskID, taskID), zap.Int("deleted", len(deleted)))
	}
	return deleted
}

func supersedeKeys(findings []*models.TaskReviewFinding) []models.ReviewFindingKey {
	seen := make(map[models.ReviewFindingKey]struct{}, len(findings))
	keys := make([]models.ReviewFindingKey, 0, len(findings))
	for _, f := range findings {
		k := f.SupersedeKey()
		if _, ok := seen[k]; ok {
			continue
		}
		seen[k] = struct{}{}
		keys = append(keys, k)
	}
	return keys
}

// buildReviewFinding validates one submitted finding and returns the row to
// store. Validation duplicates the review package's rules deliberately: this is
// the persistence boundary and must hold even for a caller that skipped the
// parser (for example the MCP tool).
func buildReviewFinding(taskID, runID string, in ReviewFindingInput) (*models.TaskReviewFinding, error) {
	filePath := strings.TrimSpace(in.FilePath)
	title := strings.TrimSpace(in.Title)
	body := strings.TrimSpace(in.Body)
	severity := models.ReviewSeverity(strings.ToLower(strings.TrimSpace(in.Severity)))

	if filePath == "" {
		return nil, errors.New("file is required")
	}
	if in.StartLine <= 0 {
		return nil, fmt.Errorf("line must be positive, got %d", in.StartLine)
	}
	endLine := in.EndLine
	if endLine == 0 {
		endLine = in.StartLine
	}
	if endLine < in.StartLine {
		return nil, fmt.Errorf("line_end %d is before line %d", endLine, in.StartLine)
	}
	if !models.ValidReviewSeverity(severity) {
		return nil, fmt.Errorf("unknown severity %q", in.Severity)
	}
	if title == "" {
		return nil, errors.New("title is required")
	}
	if body == "" {
		return nil, errors.New("body is required")
	}

	side := strings.TrimSpace(in.Side)
	if side != models.ReviewSideDeletions {
		side = models.ReviewSideAdditions
	}

	return &models.TaskReviewFinding{
		RunID:          runID,
		TaskID:         taskID,
		RepositoryID:   strings.TrimSpace(in.RepositoryID),
		RepositoryName: strings.TrimSpace(in.RepositoryName),
		FilePath:       filePath,
		StartLine:      in.StartLine,
		EndLine:        endLine,
		Side:           side,
		Severity:       severity,
		Category:       strings.TrimSpace(in.Category),
		Title:          title,
		Body:           body,
		Suggestion:     strings.TrimSpace(in.Suggestion),
		AnchorText:     in.AnchorText,
		FileDiffHash:   strings.TrimSpace(in.FileDiffHash),
		Status:         models.ReviewFindingOpen,
	}, nil
}

// UpdateFindingStatus records the human's disposition of a finding. Moving to
// resolved stamps resolved_at; returning to open clears it. This is the UI
// action's path and is deliberately unauthorized at this layer (task access
// was already established to reach the finding through the UI).
func (s *ReviewService) UpdateFindingStatus(ctx context.Context, findingID string, status models.ReviewFindingStatus) (*models.TaskReviewFinding, error) {
	if findingID == "" {
		return nil, fmt.Errorf("%w: finding id is required", ErrReviewFindingNotFound)
	}
	if err := validateReviewFindingStatus(status); err != nil {
		return nil, err
	}
	existing, err := s.repo.GetTaskReviewFinding(ctx, findingID)
	if err != nil {
		return nil, err
	}
	return s.applyFindingStatus(ctx, existing, status)
}

// ResolveFindingRequest carries the resolve_review_finding_kandev tool args.
type ResolveFindingRequest struct {
	FindingID string
	Status    models.ReviewFindingStatus
}

// ResolveFinding is the MCP tool's path: it authorizes against the finding's
// own owning task (read from the row, never trusted from the caller) before
// applying the status change. An unknown finding_id and a finding on a task
// the caller cannot reach return the identical ErrReviewFindingNotFound, so
// the response never confirms that a foreign finding_id exists.
func (s *ReviewService) ResolveFinding(ctx context.Context, req ResolveFindingRequest) (*models.TaskReviewFinding, error) {
	if req.FindingID == "" {
		return nil, fmt.Errorf("%w: finding id is required", ErrReviewFindingNotFound)
	}
	if err := validateReviewFindingStatus(req.Status); err != nil {
		return nil, err
	}
	existing, err := s.repo.GetTaskReviewFinding(ctx, req.FindingID)
	if err != nil {
		if errors.Is(err, ErrReviewFindingNotFound) {
			return nil, ErrReviewFindingNotFound
		}
		return nil, err
	}
	if err := s.authorize(ctx, existing.TaskID); err != nil {
		return nil, ErrReviewFindingNotFound
	}
	return s.applyFindingStatus(ctx, existing, req.Status)
}

// validateReviewFindingStatus rejects anything outside open/resolved/dismissed,
// naming the accepted values, shared by UpdateFindingStatus and ResolveFinding.
func validateReviewFindingStatus(status models.ReviewFindingStatus) error {
	if !models.ValidReviewFindingStatus(status) {
		return fmt.Errorf("%w: status must be one of open, resolved, dismissed (got %q)", ErrInvalidReviewFinding, status)
	}
	return nil
}

// applyFindingStatus persists a finding's new status, implementing the shared
// idempotency rule: resubmitting the finding's current status leaves
// resolved_at unchanged rather than re-stamping it. Shared by both
// UpdateFindingStatus and ResolveFinding so the two entry points cannot drift.
func (s *ReviewService) applyFindingStatus(ctx context.Context, existing *models.TaskReviewFinding, status models.ReviewFindingStatus) (*models.TaskReviewFinding, error) {
	resolvedAt := existing.ResolvedAt
	if status != existing.Status {
		if status == models.ReviewFindingResolved || status == models.ReviewFindingDismissed {
			now := time.Now().UTC()
			resolvedAt = &now
		} else {
			resolvedAt = nil
		}
	}
	if err := s.repo.UpdateTaskReviewFindingStatus(ctx, existing.ID, status, resolvedAt); err != nil {
		return nil, err
	}
	finding, err := s.repo.GetTaskReviewFinding(ctx, existing.ID)
	if err != nil {
		return nil, err
	}
	s.publishEvent(ctx, events.TaskReviewFindingUpdated, map[string]any{
		rvFieldTaskID:  finding.TaskID,
		rvFieldFinding: finding,
	})
	return finding, nil
}

// maxListedReviewFindings bounds a single list_review_findings_kandev response
// (AC-TWS-003.8/003.9).
const maxListedReviewFindings = 100

// reviewFindingStatusAll is the status filter value that returns every status
// rather than defaulting to open (AC-TWS-003.5).
const reviewFindingStatusAll = "all"

// ListFindingsRequest carries the list_review_findings_kandev filter args.
type ListFindingsRequest struct {
	TaskID   string
	Status   string
	Severity string
}

// ListFindingsResult is the outcome of a findings list, including truncation
// bookkeeping so the caller can report how many findings matched in total.
type ListFindingsResult struct {
	Findings     []*models.TaskReviewFinding
	TotalMatched int
	Truncated    bool
}

// ListFindings returns a task's review findings, filtered by status (default
// "open") and optionally by severity, ordered by repository/file/line/id and
// truncated at maxListedReviewFindings (REQ-TWS-003).
func (s *ReviewService) ListFindings(ctx context.Context, req ListFindingsRequest) (*ListFindingsResult, error) {
	if req.TaskID == "" {
		return nil, ErrTaskIDRequired
	}
	if err := s.authorize(ctx, req.TaskID); err != nil {
		return nil, &ErrReviewAccessDenied{Err: err}
	}

	status, err := normalizeReviewFindingStatusFilter(req.Status)
	if err != nil {
		return nil, err
	}
	severity, err := normalizeReviewFindingSeverityFilter(req.Severity)
	if err != nil {
		return nil, err
	}

	all, err := s.repo.ListTaskReviewFindings(ctx, req.TaskID)
	if err != nil {
		s.logger.Error("list review findings", zap.String(rvFieldTaskID, req.TaskID), zap.Error(err))
		return nil, fmt.Errorf("list review findings: %w", err)
	}

	matched := filterReviewFindings(all, status, severity)
	sortReviewFindings(matched)

	result := &ListFindingsResult{TotalMatched: len(matched)}
	if len(matched) > maxListedReviewFindings {
		result.Findings = matched[:maxListedReviewFindings]
		result.Truncated = true
	} else {
		result.Findings = matched
	}
	if result.Findings == nil {
		result.Findings = []*models.TaskReviewFinding{}
	}
	return result, nil
}

// normalizeReviewFindingStatusFilter trims and lowercases a status filter,
// defaulting an empty value to "open" (AC-TWS-003.5) and rejecting anything
// outside open/resolved/dismissed/all.
func normalizeReviewFindingStatusFilter(raw string) (string, error) {
	trimmed := strings.ToLower(strings.TrimSpace(raw))
	if trimmed == "" {
		return string(models.ReviewFindingOpen), nil
	}
	switch trimmed {
	case string(models.ReviewFindingOpen), string(models.ReviewFindingResolved), string(models.ReviewFindingDismissed), reviewFindingStatusAll:
		return trimmed, nil
	default:
		return "", fmt.Errorf("%w: status must be one of open, resolved, dismissed, all (got %q)", ErrInvalidReviewFindingFilter, raw)
	}
}

// normalizeReviewFindingSeverityFilter trims and lowercases a severity filter.
// An empty value means "no restriction" (AC-TWS-003.6), distinct from status's
// empty-means-open default.
func normalizeReviewFindingSeverityFilter(raw string) (string, error) {
	trimmed := strings.ToLower(strings.TrimSpace(raw))
	if trimmed == "" {
		return "", nil
	}
	if !models.ValidReviewSeverity(models.ReviewSeverity(trimmed)) {
		return "", fmt.Errorf("%w: severity must be one of blocker, major, minor, nit (got %q)", ErrInvalidReviewFindingFilter, raw)
	}
	return trimmed, nil
}

func filterReviewFindings(findings []*models.TaskReviewFinding, status, severity string) []*models.TaskReviewFinding {
	out := make([]*models.TaskReviewFinding, 0, len(findings))
	for _, f := range findings {
		if status != reviewFindingStatusAll && string(f.Status) != status {
			continue
		}
		if severity != "" && string(f.Severity) != severity {
			continue
		}
		out = append(out, f)
	}
	return out
}

// sortReviewFindings orders by repository_name, file_path, start_line, id
// (AC-TWS-003.8) for a stable, reviewable listing.
func sortReviewFindings(findings []*models.TaskReviewFinding) {
	sort.Slice(findings, func(i, j int) bool {
		a, b := findings[i], findings[j]
		if a.RepositoryName != b.RepositoryName {
			return a.RepositoryName < b.RepositoryName
		}
		if a.FilePath != b.FilePath {
			return a.FilePath < b.FilePath
		}
		if a.StartLine != b.StartLine {
			return a.StartLine < b.StartLine
		}
		return a.ID < b.ID
	})
}

// TaskReview is the full review state for a task.
type TaskReview struct {
	Runs     []*models.TaskReviewRun     `json:"runs"`
	Findings []*models.TaskReviewFinding `json:"findings"`
}

// GetTaskReview returns a task's bounded run history and all of its findings.
func (s *ReviewService) GetTaskReview(ctx context.Context, taskID string) (*TaskReview, error) {
	if taskID == "" {
		return nil, ErrTaskIDRequired
	}
	runs, err := s.repo.ListTaskReviewRuns(ctx, taskID, 0)
	if err != nil {
		return nil, err
	}
	findings, err := s.repo.ListTaskReviewFindings(ctx, taskID)
	if err != nil {
		return nil, err
	}
	return &TaskReview{
		Runs:     nonNilRuns(runs),
		Findings: nonNilFindings(findings),
	}, nil
}

// ClearTaskReview removes every run and finding for a task.
func (s *ReviewService) ClearTaskReview(ctx context.Context, taskID string) error {
	if taskID == "" {
		return ErrTaskIDRequired
	}
	if err := s.repo.DeleteTaskReviewByTask(ctx, taskID); err != nil {
		return err
	}
	s.publishEvent(ctx, events.TaskReviewCleared, map[string]any{rvFieldTaskID: taskID})
	return nil
}

func (s *ReviewService) publishRun(ctx context.Context, run *models.TaskReviewRun) {
	s.publishEvent(ctx, events.TaskReviewRunUpdated, map[string]any{
		rvFieldTaskID: run.TaskID,
		rvFieldRun:    run,
	})
}

func (s *ReviewService) publishFindings(ctx context.Context, taskID, runID string, findings []*models.TaskReviewFinding, supersededIDs []string) {
	if supersededIDs == nil {
		supersededIDs = []string{}
	}
	s.publishEvent(ctx, events.TaskReviewFindingsPublished, map[string]any{
		rvFieldTaskID:     taskID,
		rvFieldRunID:      runID,
		rvFieldFindings:   findings,
		rvFieldSuperseded: supersededIDs,
	})
}

func (s *ReviewService) publishEvent(ctx context.Context, eventType string, payload map[string]any) {
	if s.eventBus == nil {
		return
	}
	if err := s.eventBus.Publish(ctx, eventType, bus.NewEvent(eventType, "review-service", payload)); err != nil {
		s.logger.Error("publish review event", zap.String("event_type", eventType), zap.Error(err))
	}
}

// nonNilRuns / nonNilFindings keep the wire payload as `[]` rather than `null`,
// so the client can treat the arrays as always present.
func nonNilRuns(runs []*models.TaskReviewRun) []*models.TaskReviewRun {
	if runs == nil {
		return []*models.TaskReviewRun{}
	}
	return runs
}

func nonNilFindings(findings []*models.TaskReviewFinding) []*models.TaskReviewFinding {
	if findings == nil {
		return []*models.TaskReviewFinding{}
	}
	return findings
}
