// Package service provides workflow business logic operations.
package service

import (
	"context"
	"fmt"
	"sync"

	"github.com/google/uuid"
	"go.uber.org/zap"

	workflowcfg "github.com/kandev/kandev/config/workflows"
	"github.com/kandev/kandev/internal/common/constants"
	"github.com/kandev/kandev/internal/common/logger"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/workflow/models"
	"github.com/kandev/kandev/internal/workflow/repository"
)

// WorkflowProvider gives the workflow service access to workflows owned by the task domain.
type WorkflowProvider interface {
	ListWorkflows(ctx context.Context, workspaceID string, includeHidden bool) ([]*taskmodels.Workflow, error)
	GetWorkflow(ctx context.Context, id string) (*taskmodels.Workflow, error)
	CreateWorkflow(ctx context.Context, workspaceID, name, description string) (*taskmodels.Workflow, error)
	UpdateWorkflow(ctx context.Context, workflow *taskmodels.Workflow) error
}

// Service provides workflow business logic
type Service struct {
	repo                 *repository.Repository
	logger               *logger.Logger
	workflowProvider     WorkflowProvider
	workspaceProvider    WorkspaceProvider
	resolveProfile       models.AgentProfileResolver
	matchProfile         models.AgentProfileMatcher
	syncOps              SyncWorkflowOps
	sessionAccessChecker func(context.Context, string) error
	// workflowAccessChecker / workspaceAccessChecker carry the task domain's
	// per-user visibility rule into this package (see access.go).
	workflowAccessChecker  func(context.Context, string) error
	workspaceAccessChecker func(context.Context, string) error
	taskAccessChecker      func(context.Context, string) error
	historyQueue           chan historyWrite
	historyStop            chan struct{}
	historyDone            chan struct{}
	historyCloseOnce       sync.Once
	historyMu              sync.RWMutex
	historyClosed          bool
}

type historyWrite struct {
	sessionID, fromStepID, toStepID string
	trigger                         models.StepTransitionTrigger
	actorID                         *string
	metadata                        map[string]interface{}
}

// SetSessionAccessChecker wires the task-domain authorization check. The
// workflow package owns the history read, but the task package owns session
// and workspace permissions.
func (s *Service) SetSessionAccessChecker(checker func(context.Context, string) error) {
	s.sessionAccessChecker = checker
}

// WorkspaceProvider resolves a workspace by ID so the read-only guard can tell
// whether a workflow lives in the dedicated Improve Kandev workspace.
type WorkspaceProvider interface {
	GetWorkspace(ctx context.Context, id string) (*taskmodels.Workspace, error)
}

// SetWorkflowProvider wires the workflow provider (set during service init to break circular deps).
func (s *Service) SetWorkflowProvider(wp WorkflowProvider) {
	s.workflowProvider = wp
}

// SetWorkspaceProvider wires the workspace provider used by the read-only guard
// (set during service init to break circular deps).
func (s *Service) SetWorkspaceProvider(wp WorkspaceProvider) {
	s.workspaceProvider = wp
}

// SetAgentProfileFuncs wires the agent profile resolver and matcher for export/import.
func (s *Service) SetAgentProfileFuncs(resolve models.AgentProfileResolver, match models.AgentProfileMatcher) {
	s.resolveProfile = resolve
	s.matchProfile = match
}

// NewService creates a new workflow service
func NewService(repo *repository.Repository, log *logger.Logger) *Service {
	s := &Service{
		repo:         repo,
		logger:       log.WithFields(zap.String("component", "workflow-service")),
		historyQueue: make(chan historyWrite, 256),
		historyStop:  make(chan struct{}),
		historyDone:  make(chan struct{}),
	}
	go s.runHistoryWriter()
	return s
}

func (s *Service) runHistoryWriter() {
	defer close(s.historyDone)
	for {
		select {
		case write := <-s.historyQueue:
			ctx, cancel := context.WithTimeout(context.Background(), constants.StepHistoryWriteTimeout)
			if err := s.CreateStepTransition(ctx, write.sessionID, write.fromStepID, write.toStepID, write.trigger, write.actorID, write.metadata); err != nil {
				s.logger.Warn("failed to write queued step transition", zap.Error(err))
			}
			cancel()
		case <-s.historyStop:
			s.drainHistoryQueue()
			return
		}
	}
}

// drainHistoryQueue flushes whatever is left in the queue at shutdown under a
// single bounded deadline shared by every remaining row, rather than a fresh
// StepHistoryWriteTimeout per row — a stalled database and a full 256-row
// queue would otherwise be able to block graceful shutdown for minutes. Rows
// that don't fit inside the deadline are dropped; this audit trail is
// best-effort and must never hold up shutdown.
func (s *Service) drainHistoryQueue() {
	ctx, cancel := context.WithTimeout(context.Background(), constants.StepHistoryWriteTimeout)
	defer cancel()
	for {
		select {
		case write := <-s.historyQueue:
			if err := s.CreateStepTransition(ctx, write.sessionID, write.fromStepID, write.toStepID, write.trigger, write.actorID, write.metadata); err != nil {
				s.logger.Warn("failed to write queued step transition during shutdown", zap.Error(err))
			}
		case <-ctx.Done():
			if dropped := len(s.historyQueue); dropped > 0 {
				s.logger.Warn("dropped queued step transitions at shutdown deadline", zap.Int("dropped", dropped))
			}
			return
		default:
			return
		}
	}
}

// EnqueueStepTransition records transition history outside the caller's
// event-reader path. The queue is bounded. A full queue, or an enqueue after
// Close has begun, is logged as a dropped best-effort telemetry row while the
// workflow mutation itself succeeds.
func (s *Service) EnqueueStepTransition(sessionID, fromStepID, toStepID string, trigger models.StepTransitionTrigger, actorID *string, metadata map[string]interface{}) {
	if sessionID == "" {
		return
	}
	s.historyMu.RLock()
	defer s.historyMu.RUnlock()
	if s.historyClosed {
		s.logger.Warn("dropped step transition enqueued after shutdown", zap.String("session_id", sessionID))
		return
	}
	select {
	case s.historyQueue <- historyWrite{sessionID: sessionID, fromStepID: fromStepID, toStepID: toStepID, trigger: trigger, actorID: actorID, metadata: metadata}:
	default:
		s.logger.Warn("step transition history queue is full", zap.String("session_id", sessionID))
	}
}

// Close stops accepting new history writes, drains the queue under a bounded
// deadline, and waits for the writer goroutine to exit. The closed flag is
// set under the same lock EnqueueStepTransition holds across its
// check-and-send, so no enqueue can land in the queue after a concurrent
// Close has started shutting it down.
func (s *Service) Close() error {
	if s.historyStop == nil {
		return nil
	}
	s.historyCloseOnce.Do(func() {
		s.historyMu.Lock()
		s.historyClosed = true
		s.historyMu.Unlock()
		close(s.historyStop)
	})
	<-s.historyDone
	return nil
}

// ============================================================================
// Template Operations
// ============================================================================

// ListTemplates returns user-pickable workflow templates. Templates marked
// `hidden: true` in their embedded YAML (e.g. improve-kandev) are excluded
// from the management UI and the create-workflow picker.
func (s *Service) ListTemplates(ctx context.Context) ([]*models.WorkflowTemplate, error) {
	templates, err := s.repo.ListTemplates(ctx)
	if err != nil {
		s.logger.Error("failed to list templates", zap.Error(err))
		return nil, err
	}
	hidden, err := workflowcfg.HiddenTemplateIDs()
	if err != nil {
		s.logger.Error("failed to load embedded template visibility", zap.Error(err))
		return nil, err
	}
	result := make([]*models.WorkflowTemplate, 0, len(templates))
	for _, t := range templates {
		if hidden[t.ID] {
			continue
		}
		result = append(result, t)
	}
	return result, nil
}

// GetTemplate retrieves a workflow template by ID.
func (s *Service) GetTemplate(ctx context.Context, id string) (*models.WorkflowTemplate, error) {
	template, err := s.repo.GetTemplate(ctx, id)
	if err != nil {
		s.logger.Error("failed to get template", zap.String("template_id", id), zap.Error(err))
		return nil, err
	}
	return template, nil
}

// GetSystemTemplates returns only system workflow templates.
func (s *Service) GetSystemTemplates(ctx context.Context) ([]*models.WorkflowTemplate, error) {
	templates, err := s.repo.GetSystemTemplates(ctx)
	if err != nil {
		s.logger.Error("failed to get system templates", zap.Error(err))
		return nil, err
	}
	return templates, nil
}

// ============================================================================
// Step Operations
// ============================================================================

// ListStepsByWorkflow returns all workflow steps for a workflow.
func (s *Service) ListStepsByWorkflow(ctx context.Context, workflowID string) ([]*models.WorkflowStep, error) {
	steps, err := s.repo.ListStepsByWorkflow(ctx, workflowID)
	if err != nil {
		s.logger.Error("failed to list steps by workflow", zap.String("workflow_id", workflowID), zap.Error(err))
		return nil, err
	}
	return steps, nil
}

// ListStepsByWorkspaceID returns all workflow steps for all workflows in a workspace.
func (s *Service) ListStepsByWorkspaceID(ctx context.Context, workspaceID string) ([]*models.WorkflowStep, error) {
	if err := s.AuthorizeWorkspace(ctx, workspaceID); err != nil {
		return nil, err
	}
	steps, err := s.repo.ListStepsByWorkspaceID(ctx, workspaceID)
	if err != nil {
		s.logger.Error("failed to list steps by workspace", zap.String("workspace_id", workspaceID), zap.Error(err))
		return nil, err
	}
	return steps, nil
}

// GetStep retrieves a workflow step by ID.
func (s *Service) GetStep(ctx context.Context, stepID string) (*models.WorkflowStep, error) {
	step, err := s.repo.GetStep(ctx, stepID)
	if err != nil {
		s.logger.Error("failed to get step", zap.String("step_id", stepID), zap.Error(err))
		return nil, err
	}
	return step, nil
}

// GetNextStepByPosition returns the next step after the given position for a workflow.
// Steps are ordered by position, so this finds the step with the next higher position.
// Returns nil if there is no next step (i.e., current step is the last one).
func (s *Service) GetNextStepByPosition(ctx context.Context, workflowID string, currentPosition int) (*models.WorkflowStep, error) {
	steps, err := s.repo.ListStepsByWorkflow(ctx, workflowID)
	if err != nil {
		s.logger.Error("failed to list steps for next step lookup",
			zap.String("workflow_id", workflowID),
			zap.Error(err))
		return nil, err
	}

	// Steps are already ordered by position from ListStepsByWorkflow
	for _, step := range steps {
		if step.Position > currentPosition {
			return step, nil
		}
	}

	return nil, nil // No next step found (current step is the last one)
}

// WorkflowMeta is the subset of workflow fields needed at step entry
// (agent profile default + optional workflow-level prompt).
type WorkflowMeta struct {
	AgentProfileID string
	Prompt         string
}

// GetWorkflowMeta returns agent profile id and prompt for a workflow in one
// provider read. Callers that previously stacked GetWorkflowAgentProfileID and
// GetWorkflowPrompt should use this instead.
func (s *Service) GetWorkflowMeta(ctx context.Context, workflowID string) (WorkflowMeta, error) {
	wf, err := s.workflowProvider.GetWorkflow(ctx, workflowID)
	if err != nil {
		return WorkflowMeta{}, err
	}
	return WorkflowMeta{
		AgentProfileID: wf.AgentProfileID,
		Prompt:         wf.Prompt,
	}, nil
}

// GetWorkflowAgentProfileID returns the default agent profile ID for a workflow.
func (s *Service) GetWorkflowAgentProfileID(ctx context.Context, workflowID string) (string, error) {
	meta, err := s.GetWorkflowMeta(ctx, workflowID)
	if err != nil {
		return "", err
	}
	return meta.AgentProfileID, nil
}

// GetWorkflowPrompt returns the optional workflow-level agent instructions.
// Empty string means the workflow has no prompt configured.
func (s *Service) GetWorkflowPrompt(ctx context.Context, workflowID string) (string, error) {
	meta, err := s.GetWorkflowMeta(ctx, workflowID)
	if err != nil {
		return "", err
	}
	return meta.Prompt, nil
}

// GetPreviousStepByPosition returns the previous step before the given position for a workflow.
// Returns nil if there is no previous step (i.e., current step is the first one).
func (s *Service) GetPreviousStepByPosition(ctx context.Context, workflowID string, currentPosition int) (*models.WorkflowStep, error) {
	steps, err := s.repo.ListStepsByWorkflow(ctx, workflowID)
	if err != nil {
		s.logger.Error("failed to list steps for previous step lookup",
			zap.String("workflow_id", workflowID),
			zap.Error(err))
		return nil, err
	}

	// Steps are ordered by position ascending. Walk backwards to find the step just before current.
	var prev *models.WorkflowStep
	for _, step := range steps {
		if step.Position >= currentPosition {
			break
		}
		prev = step
	}

	return prev, nil
}

// CreateStepsFromTemplate creates workflow steps for a workflow from a template.
func (s *Service) CreateStepsFromTemplate(ctx context.Context, workflowID, templateID string) error {
	template, err := s.repo.GetTemplate(ctx, templateID)
	if err != nil {
		s.logger.Error("failed to get template for step creation",
			zap.String("template_id", templateID),
			zap.Error(err))
		return fmt.Errorf("failed to get template: %w", err)
	}

	// Build mapping from template step ID to new UUID
	idMap := make(map[string]string, len(template.Steps))
	for _, stepDef := range template.Steps {
		idMap[stepDef.ID] = uuid.New().String()
	}

	steps := make([]*models.WorkflowStep, 0, len(template.Steps))
	// Build and validate every step before the first write. A malformed later
	// step must not leave a partially-created workflow behind.
	for _, stepDef := range template.Steps {
		events := models.RemapStepEvents(stepDef.Events, idMap)
		step := &models.WorkflowStep{
			ID:                         idMap[stepDef.ID],
			WorkflowID:                 workflowID,
			Name:                       stepDef.Name,
			Position:                   stepDef.Position,
			Color:                      stepDef.Color,
			Prompt:                     stepDef.Prompt,
			Events:                     events,
			AllowManualMove:            stepDef.AllowManualMove,
			IsStartStep:                stepDef.IsStartStep,
			ShowInCommandPanel:         stepDef.ShowInCommandPanel,
			AutoArchiveAfterHours:      stepDef.AutoArchiveAfterHours,
			AgentProfileID:             stepDef.AgentProfileID,
			AutoAdvanceRequiresSignal:  stepDef.AutoAdvanceRequiresSignal,
			CancelTriggersTurnComplete: stepDef.CancelTriggersTurnComplete,
			WIPLimit:                   stepDef.WIPLimit,
			PullFromStepID:             models.RemapStepID(stepDef.PullFromStepID, idMap),
			StageType:                  stepDef.StageType,
		}

		if err := models.ValidateWorkflowStep(step); err != nil {
			return fmt.Errorf("validate template step %q: %w", step.Name, err)
		}
		steps = append(steps, step)
	}

	for _, step := range steps {
		if err := s.repo.CreateStep(ctx, step); err != nil {
			s.logger.Error("failed to create step from template",
				zap.String("workflow_id", workflowID),
				zap.String("step_name", step.Name),
				zap.Error(err))
			return fmt.Errorf("failed to create step %s: %w", step.Name, err)
		}
	}

	s.logger.Info("created workflow steps from template",
		zap.String("workflow_id", workflowID),
		zap.String("template_id", templateID),
		zap.Int("step_count", len(template.Steps)))

	return nil
}

// ResolveStartStep resolves which step a task should start in for a workflow.
// Fallback chain: is_start_step=true → first step by position.
func (s *Service) ResolveStartStep(ctx context.Context, workflowID string) (*models.WorkflowStep, error) {
	startStep, err := s.repo.GetStartStep(ctx, workflowID)
	if err != nil {
		s.logger.Error("failed to get start step", zap.String("workflow_id", workflowID), zap.Error(err))
		return nil, err
	}
	if startStep != nil {
		return startStep, nil
	}

	// Fallback: first step by position
	return s.ResolveFirstStep(ctx, workflowID)
}

// ResolveAutoStartStep resolves which step a task should be created in when the
// creator asked to start an agent immediately: the first step by position whose
// on_enter carries auto_start_agent.
//
// `is_start_step` and `auto_start_agent` are independent settings — the first
// says where new tasks are parked, the second says which steps run agents — so
// a task that is starting now belongs in the first step that actually runs
// agents, not in the parking column. They coincide in every built-in template
// (In Progress is both), which is why routing an agent start through
// ResolveStartStep looked correct until someone moved the start step onto their
// backlog and watched the agent start there anyway.
//
// Fallback chain: first auto_start_agent step by position → ResolveStartStep
// (is_start_step → first step by position), so a workflow with no automated
// step behaves exactly as it did before.
func (s *Service) ResolveAutoStartStep(ctx context.Context, workflowID string) (*models.WorkflowStep, error) {
	steps, err := s.repo.ListStepsByWorkflow(ctx, workflowID)
	if err != nil {
		s.logger.Error("failed to list steps for auto-start step resolution", zap.String("workflow_id", workflowID), zap.Error(err))
		return nil, err
	}
	if step := models.SelectAutoStartStep(steps); step != nil {
		return step, nil
	}
	return s.ResolveStartStep(ctx, workflowID)
}

// ResolveFirstStep always returns the first step by position, ignoring is_start_step.
func (s *Service) ResolveFirstStep(ctx context.Context, workflowID string) (*models.WorkflowStep, error) {
	steps, err := s.repo.ListStepsByWorkflow(ctx, workflowID)
	if err != nil {
		s.logger.Error("failed to list steps for first step resolution", zap.String("workflow_id", workflowID), zap.Error(err))
		return nil, err
	}
	if len(steps) == 0 {
		return nil, fmt.Errorf("workflow %s has no steps", workflowID)
	}
	return steps[0], nil
}

// CreateStep creates a new workflow step.
func (s *Service) CreateStep(ctx context.Context, step *models.WorkflowStep) error {
	_, err := s.CreateStepWithStartStepUpdates(ctx, step)
	return err
}

// CreateStepWithStartStepUpdates creates a new workflow step and returns any
// other workflow steps whose start-step flag was cleared.
func (s *Service) CreateStepWithStartStepUpdates(ctx context.Context, step *models.WorkflowStep) ([]*models.WorkflowStep, error) {
	if err := models.ValidateWorkflowStep(step); err != nil {
		return nil, err
	}
	if step.ID == "" {
		step.ID = uuid.New().String()
	}
	demoted, err := s.repo.CreateStepWithDemotedStartSteps(ctx, step)
	if err != nil {
		s.logger.Error("failed to create step", zap.String("workflow_id", step.WorkflowID), zap.Error(err))
		return nil, err
	}
	s.logger.Info("created workflow step", zap.String("step_id", step.ID), zap.String("workflow_id", step.WorkflowID))
	return demoted, nil
}

// UpdateStep updates an existing workflow step.
func (s *Service) UpdateStep(ctx context.Context, step *models.WorkflowStep) error {
	_, err := s.UpdateStepWithStartStepUpdates(ctx, step)
	return err
}

// UpdateStepWithStartStepUpdates updates a workflow step and returns any other
// workflow steps whose start-step flag was cleared.
func (s *Service) UpdateStepWithStartStepUpdates(ctx context.Context, step *models.WorkflowStep) ([]*models.WorkflowStep, error) {
	if err := models.ValidateWorkflowStep(step); err != nil {
		return nil, err
	}
	demoted, err := s.repo.UpdateStepWithDemotedStartSteps(ctx, step)
	if err != nil {
		s.logger.Error("failed to update step", zap.String("step_id", step.ID), zap.Error(err))
		return nil, err
	}
	s.logger.Info("updated workflow step", zap.String("step_id", step.ID))
	return demoted, nil
}

// DeleteStep deletes a workflow step and clears any references to it from other steps.
// Tasks currently on the step are NOT auto-reassigned: doing so through the normal
// move path would run on_exit/on_enter (auto-start agents, session hand-offs) as a
// side effect of an administrative delete, and any failed move would be silently
// ignored while the step is deleted anyway (a partial, non-atomic cascade). Instead,
// affected tasks keep their now-dangling workflow_step_id and are surfaced via the
// Kanban/Pipeline "Needs Reassignment" fallback column for explicit manual
// reassignment by a user (which goes through the normal, fully-validated move path).
func (s *Service) DeleteStep(ctx context.Context, stepID string) error {
	// First, get the step to find its workflow ID
	step, err := s.repo.GetStep(ctx, stepID)
	if err != nil {
		s.logger.Error("failed to get step for deletion", zap.String("step_id", stepID), zap.Error(err))
		return err
	}

	// Clear any move_to_step references to this step
	if err := s.repo.ClearStepReferences(ctx, step.WorkflowID, stepID); err != nil {
		s.logger.Error("failed to clear step references",
			zap.String("step_id", stepID),
			zap.String("workflow_id", step.WorkflowID),
			zap.Error(err))
		return err
	}

	// Now delete the step
	if err := s.repo.DeleteStep(ctx, stepID); err != nil {
		s.logger.Error("failed to delete step", zap.String("step_id", stepID), zap.Error(err))
		return err
	}

	s.logger.Info("deleted workflow step and cleared references",
		zap.String("step_id", stepID),
		zap.String("workflow_id", step.WorkflowID))
	return nil
}

// ReorderSteps reorders workflow steps for a workflow.
func (s *Service) ReorderSteps(ctx context.Context, workflowID string, stepIDs []string) error {
	if err := s.AuthorizeWorkflow(ctx, workflowID); err != nil {
		return err
	}
	// Resolve and check every step before writing any of them. The step IDs
	// are caller-supplied and are not required by anything upstream to belong
	// to workflowID, so a reorder could set positions on another workflow's
	// steps — including a read-only one, whose guard only ever saw the
	// workflow named in the URL. A rejection found halfway through the write
	// loop would also leave a half-applied reorder behind.
	steps := make([]*models.WorkflowStep, 0, len(stepIDs))
	for _, stepID := range stepIDs {
		step, err := s.repo.GetStep(ctx, stepID)
		if err != nil {
			s.logger.Error("failed to get step for reorder", zap.String("step_id", stepID), zap.Error(err))
			return err
		}
		if step == nil || step.WorkflowID != workflowID {
			// Same answer as a step that does not exist: the caller named an
			// ID this workflow does not own, and whether it exists elsewhere
			// is not theirs to learn.
			s.logger.Warn("refused to reorder a step from another workflow",
				zap.String("step_id", stepID), zap.String("workflow_id", workflowID))
			return ErrNotVisible
		}
		steps = append(steps, step)
	}
	for i, step := range steps {
		step.Position = i
		if err := s.repo.UpdateStep(ctx, step); err != nil {
			s.logger.Error("failed to update step position", zap.String("step_id", step.ID), zap.Error(err))
			return err
		}
	}
	s.logger.Info("reordered workflow steps", zap.String("workflow_id", workflowID), zap.Int("count", len(stepIDs)))
	return nil
}

// ============================================================================
// History Operations
// ============================================================================

// CreateStepTransition creates a new step transition history entry. metadata
// is optional (nil for a plain move) and carries the ADR 0015 consumed-signal
// shape ({"signal_source", "signal_summary"}) when a completion signal drove
// the transition.
func (s *Service) CreateStepTransition(ctx context.Context, sessionID string, fromStepID, toStepID string, trigger models.StepTransitionTrigger, actorID *string, metadata map[string]interface{}) error {
	history := &models.SessionStepHistory{
		SessionID: sessionID,
		ToStepID:  toStepID,
		Trigger:   trigger,
		ActorID:   actorID,
		Metadata:  metadata,
	}

	if fromStepID != "" {
		history.FromStepID = &fromStepID
	}

	if err := s.repo.CreateHistory(ctx, history); err != nil {
		s.logger.Error("failed to create step transition",
			zap.String("session_id", sessionID),
			zap.String("to_step_id", toStepID),
			zap.Error(err))
		return err
	}

	s.logger.Info("step transition recorded",
		zap.String("session_id", sessionID),
		zap.Stringp("from_step_id", history.FromStepID),
		zap.String("to_step_id", toStepID),
		zap.String("trigger", string(trigger)))

	return nil
}

// ListHistoryBySession returns all step history entries for a session.
func (s *Service) ListHistoryBySession(ctx context.Context, sessionID string) ([]*models.SessionStepHistory, error) {
	if s.sessionAccessChecker != nil {
		if err := s.sessionAccessChecker(ctx, sessionID); err != nil {
			return nil, err
		}
	}
	history, err := s.repo.ListHistoryBySession(ctx, sessionID)
	if err != nil {
		s.logger.Error("failed to list history by session", zap.String("session_id", sessionID), zap.Error(err))
		return nil, err
	}
	return history, nil
}

// ============================================================================
// Export/Import Operations
// ============================================================================

// ImportResult holds the outcome of an import operation.
type ImportResult struct {
	Created []string `json:"created"`
	Skipped []string `json:"skipped"`
}

// ExportWorkflow exports a single workflow with its steps as portable JSON.
func (s *Service) ExportWorkflow(ctx context.Context, workflowID string) (*models.WorkflowExport, error) {
	if err := s.AuthorizeWorkflow(ctx, workflowID); err != nil {
		return nil, err
	}
	wf, err := s.workflowProvider.GetWorkflow(ctx, workflowID)
	if err != nil {
		return nil, fmt.Errorf("failed to get workflow: %w", err)
	}
	steps, err := s.repo.ListStepsByWorkflow(ctx, workflowID)
	if err != nil {
		return nil, fmt.Errorf("failed to list steps: %w", err)
	}
	stepMap := map[string][]*models.WorkflowStep{wf.ID: steps}
	return models.BuildWorkflowExport([]*taskmodels.Workflow{wf}, stepMap, s.resolveProfile), nil
}

// ExportWorkflows exports workflows for a workspace. When workflowIDs is nil,
// every (non-hidden) workflow is exported (back-compat). When workflowIDs is
// non-nil, only workflows whose ID is in the set are exported — an empty set
// exports nothing. Filtering is by ID membership only: the backend MUST NOT
// branch on a workflow's style (see internal/workflow/models/phase2.go), so the
// frontend decides which workflows (e.g. kanban-only) to include and passes
// their IDs.
//
// Hidden/system workflows (e.g. improve-kandev) are only listed when the caller
// passes explicit IDs; the back-compat "export everything" path leaves them out
// so a bare GET of the export endpoint can't leak system flows.
func (s *Service) ExportWorkflows(ctx context.Context, workspaceID string, workflowIDs []string) (*models.WorkflowExport, error) {
	if err := s.AuthorizeWorkspace(ctx, workspaceID); err != nil {
		return nil, err
	}
	includeHidden := workflowIDs != nil
	workflows, err := s.workflowProvider.ListWorkflows(ctx, workspaceID, includeHidden)
	if err != nil {
		return nil, fmt.Errorf("failed to list workflows: %w", err)
	}
	if workflowIDs != nil {
		workflows = filterWorkflowsByID(workflows, workflowIDs)
	}
	stepMap := make(map[string][]*models.WorkflowStep, len(workflows))
	for _, wf := range workflows {
		steps, stepsErr := s.repo.ListStepsByWorkflow(ctx, wf.ID)
		if stepsErr != nil {
			return nil, fmt.Errorf("failed to list steps for workflow %s: %w", wf.ID, stepsErr)
		}
		stepMap[wf.ID] = steps
	}
	return models.BuildWorkflowExport(workflows, stepMap, s.resolveProfile), nil
}

// filterWorkflowsByID returns the subset of workflows whose ID is in ids,
// preserving the input order.
func filterWorkflowsByID(workflows []*taskmodels.Workflow, ids []string) []*taskmodels.Workflow {
	want := make(map[string]bool, len(ids))
	for _, id := range ids {
		want[id] = true
	}
	filtered := make([]*taskmodels.Workflow, 0, len(workflows))
	for _, wf := range workflows {
		if want[wf.ID] {
			filtered = append(filtered, wf)
		}
	}
	return filtered
}

// ImportWorkflows imports workflows into a workspace. Deduplicates by name.
func (s *Service) ImportWorkflows(ctx context.Context, workspaceID string, export *models.WorkflowExport) (*ImportResult, error) {
	if err := s.AuthorizeWorkspace(ctx, workspaceID); err != nil {
		return nil, err
	}
	if err := export.Validate(); err != nil {
		return nil, fmt.Errorf("invalid export data: %w", err)
	}

	existing, err := s.workflowProvider.ListWorkflows(ctx, workspaceID, true)
	if err != nil {
		return nil, fmt.Errorf("failed to list existing workflows: %w", err)
	}
	existingNames := make(map[string]bool, len(existing))
	for _, wf := range existing {
		existingNames[wf.Name] = true
	}

	result := &ImportResult{}
	for _, pw := range export.Workflows {
		if existingNames[pw.Name] {
			result.Skipped = append(result.Skipped, pw.Name)
			continue
		}
		if _, err := s.importSingleWorkflow(ctx, workspaceID, pw); err != nil {
			return nil, fmt.Errorf("failed to import workflow %q: %w", pw.Name, err)
		}
		result.Created = append(result.Created, pw.Name)
	}

	s.logger.Info("imported workflows",
		zap.String("workspace_id", workspaceID),
		zap.Int("created", len(result.Created)),
		zap.Int("skipped", len(result.Skipped)))
	return result, nil
}

func (s *Service) importSingleWorkflow(ctx context.Context, workspaceID string, pw models.WorkflowPortable) (*taskmodels.Workflow, error) {
	// Generate UUIDs for all steps and build position→ID map.
	posToID := make(map[int]string, len(pw.Steps))
	for _, sp := range pw.Steps {
		posToID[sp.Position] = uuid.New().String()
	}

	// Build and validate every step before creating the workflow or persisting
	// any step. This keeps imports atomic with respect to validation failures.
	steps := make([]*models.WorkflowStep, 0, len(pw.Steps))
	for _, sp := range pw.Steps {
		step := s.stepFromPortable("pending-workflow", sp, posToID, "")
		if err := models.ValidateWorkflowStep(step); err != nil {
			return nil, fmt.Errorf("validate step %q: %w", sp.Name, err)
		}
		if err := s.validateImportedStepReferences(ctx, step, sp.Name); err != nil {
			return nil, err
		}
		steps = append(steps, step)
	}

	wf, err := s.workflowProvider.CreateWorkflow(ctx, workspaceID, pw.Name, pw.Description)
	if err != nil {
		return nil, fmt.Errorf("create workflow: %w", err)
	}

	needsUpdate := false
	// Match workflow-level agent profile if present.
	if pw.AgentProfile != nil && s.matchProfile != nil {
		if profileID := s.matchProfile(pw.AgentProfile.AgentName, pw.AgentProfile.Model, pw.AgentProfile.Mode, ""); profileID != "" {
			wf.AgentProfileID = profileID
			needsUpdate = true
		}
	}
	if pw.Prompt != "" {
		wf.Prompt = pw.Prompt
		needsUpdate = true
	}
	if needsUpdate {
		if err := s.workflowProvider.UpdateWorkflow(ctx, wf); err != nil {
			return nil, fmt.Errorf("set workflow fields: %w", err)
		}
	}

	for _, step := range steps {
		step.WorkflowID = wf.ID
		if err := s.repo.CreateStep(ctx, step); err != nil {
			return nil, fmt.Errorf("create step %q: %w", step.Name, err)
		}
	}
	return wf, nil
}

// validateImportedStepReferences authorizes the tasks an imported step would
// queue work on.
//
// Transition targets need no check here. An export names them by position:
// WorkflowExport.Validate rejects a raw `step_id` under on_turn_start and
// on_turn_complete and requires each position to exist in the document, and
// ConvertPositionToStepID then maps every one onto a step this import just
// created. The Phase 2 triggers carry no reference at all, because that
// converter drops those lists. If either of those stops being true, the
// same-workflow check for step targets belongs here.
//
// A queue_run task target has no positional form, and an on_enter action is
// copied through the conversion verbatim, so a hand-written document can name
// any task in the install — authorize it against the importing caller exactly
// as the step-write API does.
func (s *Service) validateImportedStepReferences(
	ctx context.Context, step *models.WorkflowStep, name string,
) error {
	for _, taskID := range models.CollectStepEventReferences(step.Events).TaskIDs {
		if err := s.AuthorizeTask(ctx, taskID); err != nil {
			return fmt.Errorf("step %q: %w", name, err)
		}
	}
	return nil
}

// stepFromPortable builds a WorkflowStep from its portable form, remapping
// position-based references to the step IDs in posToID and matching the
// step-level agent profile when a matcher is wired. existingProfileID is the
// profile already bound to this step (by name) before sync, or "" for a step
// with no prior binding (including at import time) - it lets the matcher
// preserve a disabled-but-still-matching binding instead of treating
// disablement as a rebind.
func (s *Service) stepFromPortable(workflowID string, sp models.StepPortable, posToID map[int]string, existingProfileID string) *models.WorkflowStep {
	return s.stepFromPortableWithMatcher(workflowID, sp, posToID, s.matchProfile, existingProfileID)
}

// stepFromPortableWithMatcher lets workflow sync use a matcher that preserves
// profile IDs embedded in existing review actions while imports use the normal
// matcher directly.
func (s *Service) stepFromPortableWithMatcher(workflowID string, sp models.StepPortable, posToID map[int]string, matchProfile models.AgentProfileMatcher, existingProfileID string) *models.WorkflowStep {
	step := &models.WorkflowStep{
		ID:                         posToID[sp.Position],
		WorkflowID:                 workflowID,
		Name:                       sp.Name,
		Position:                   sp.Position,
		Color:                      sp.Color,
		Prompt:                     sp.Prompt,
		Events:                     models.ConvertReviewProfileToID(models.ConvertPositionToStepID(sp.Events, posToID), matchProfile),
		IsStartStep:                sp.IsStartStep,
		ShowInCommandPanel:         sp.ShowInCommandPanel,
		AllowManualMove:            sp.AllowManualMove,
		AutoArchiveAfterHours:      sp.AutoArchiveAfterHours,
		AutoAdvanceRequiresSignal:  sp.AutoAdvanceRequiresSignal,
		CancelTriggersTurnComplete: sp.CancelTriggersTurnComplete,
		WIPLimit:                   sp.WIPLimit,
		PullFromStepID:             sp.PullFromStepID(posToID),
	}
	if sp.AgentProfile != nil && matchProfile != nil {
		step.AgentProfileID = matchProfile(sp.AgentProfile.AgentName, sp.AgentProfile.Model, sp.AgentProfile.Mode, existingProfileID)
	}
	return step
}
