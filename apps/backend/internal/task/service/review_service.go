package service

import (
	"context"
	"errors"
	"fmt"
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
// resolved stamps resolved_at; returning to open clears it.
func (s *ReviewService) UpdateFindingStatus(ctx context.Context, findingID string, status models.ReviewFindingStatus) (*models.TaskReviewFinding, error) {
	if findingID == "" {
		return nil, fmt.Errorf("%w: finding id is required", ErrReviewFindingNotFound)
	}
	if !models.ValidReviewFindingStatus(status) {
		return nil, fmt.Errorf("%w: unknown status %q", ErrInvalidReviewFinding, status)
	}
	var resolvedAt *time.Time
	if status == models.ReviewFindingResolved || status == models.ReviewFindingDismissed {
		now := time.Now().UTC()
		resolvedAt = &now
	}
	if err := s.repo.UpdateTaskReviewFindingStatus(ctx, findingID, status, resolvedAt); err != nil {
		return nil, err
	}
	finding, err := s.repo.GetTaskReviewFinding(ctx, findingID)
	if err != nil {
		return nil, err
	}
	s.publishEvent(ctx, events.TaskReviewFindingUpdated, map[string]any{
		rvFieldTaskID:  finding.TaskID,
		rvFieldFinding: finding,
	})
	return finding, nil
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
