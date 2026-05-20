// Package service provides business logic for the office domain.
package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/office/configloader"
	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/repository/sqlite"
	"github.com/kandev/kandev/internal/office/shared"
	runsservice "github.com/kandev/kandev/internal/runs/service"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"

	"go.uber.org/zap"
)

func isValidPathComponent(s string) bool {
	return s != "" && !strings.Contains(s, "/") && !strings.Contains(s, "\\") && !strings.Contains(s, "..")
}

// TaskStarter launches agent sessions on behalf of the office scheduler.
// Implemented by the orchestrator service; the office package depends only
// on this interface to avoid a direct import of the orchestrator package.
type TaskStarter interface {
	// StartTask starts agent execution for a task, creating a new session.
	StartTask(ctx context.Context, taskID string, agentProfileID string, executorID string,
		executorProfileID string, priority string, prompt string, workflowStepID string,
		planMode bool, attachments []v1.MessageAttachment) error
}

// TaskStarterWithEnv optionally carries launch-scoped env vars to the agent runtime.
type TaskStarterWithEnv interface {
	StartTaskWithEnv(ctx context.Context, taskID string, agentProfileID string, executorID string,
		executorProfileID string, priority string, prompt string, workflowStepID string,
		planMode bool, attachments []v1.MessageAttachment, env map[string]string) error
}

// LaunchContext mirrors scheduler.LaunchContext so the office.service
// package can carry the Office-built launch context (prompt, env,
// workflow step, attachments, plan-mode, profile) into the routing
// dispatcher without importing the scheduler package directly.
//
// The scheduler.RoutingDispatcher implementation translates this to
// the scheduler-side LaunchContext when calling StartTaskWithRoute.
type LaunchContext struct {
	ExecutorID        string
	ExecutorProfileID string
	Priority          string
	Prompt            string
	WorkflowStepID    string
	PlanMode          bool
	Attachments       []v1.MessageAttachment
	Env               map[string]string
	ProfileID         string
}

// RoutingDispatcher is the seam the office scheduler integration uses to
// hand a run off to the provider-routing dispatcher. Implemented by the
// scheduler.SchedulerService when routing is wired. A nil dispatcher
// keeps the legacy concrete-profile path identical to today.
type RoutingDispatcher interface {
	// DispatchWithRouting attempts the configured providers in order.
	// Returns launched=true when a real launch succeeded; launched=false
	// with err=nil means routing is disabled (caller falls through to
	// the legacy launch path) OR the run was parked (caller treats as
	// handled — no failure escalation).
	DispatchWithRouting(ctx context.Context, run *models.Run,
		agent *models.AgentInstance, launch LaunchContext) (launched bool, parked bool, err error)
	// HandlePostStartFailure classifies a streaming/tool failure for a
	// run that was launched via routing. Returns handled=true when the
	// routing path requeued the run (caller should NOT escalate).
	HandlePostStartFailure(ctx context.Context, run *models.Run,
		agent *models.AgentInstance, errorMessage string) (handled bool, err error)
	// MarkRunSuccessHealth flips a successful run's resolved provider /
	// model / tier scopes back to healthy.
	MarkRunSuccessHealth(ctx context.Context, run *models.Run,
		agent *models.AgentInstance)
}

// AgentTokenMinter creates per-run agent API tokens for Office runtime calls.
type AgentTokenMinter interface {
	MintRuntimeJWT(agentInstanceID, taskID, workspaceID, runID, sessionID, capabilities string) (string, error)
}

// TaskCanceller stops active agent execution for a task.
type TaskCanceller interface {
	CancelTaskExecution(ctx context.Context, taskID string, reason string, force bool) error
}

// TaskWorkspaceService owns workspace/task rows outside the office schema.
type TaskWorkspaceService interface {
	GetWorkspace(ctx context.Context, id string) (*taskmodels.Workspace, error)
	ListWorkspaces(ctx context.Context) ([]*taskmodels.Workspace, error)
	DeleteWorkspace(ctx context.Context, id string) error
	ListTasksByWorkspace(ctx context.Context, workspaceID, workflowID, repositoryID, query string, page, pageSize int, includeArchived, includeEphemeral, onlyEphemeral, excludeConfig bool) ([]*taskmodels.Task, int, error)
	DeleteTask(ctx context.Context, id string) error
	GetLastAgentMessage(ctx context.Context, sessionID string) (string, error)
}

// TaskStarterFunc adapts a function to the TaskStarter interface.
// Useful for wrapping callers whose StartTask returns additional values.
type TaskStarterFunc func(ctx context.Context, taskID, agentProfileID, executorID,
	executorProfileID string, priority string, prompt, workflowStepID string,
	planMode bool, attachments []v1.MessageAttachment) error

// StartTask implements TaskStarter.
func (f TaskStarterFunc) StartTask(ctx context.Context, taskID, agentProfileID, executorID,
	executorProfileID string, priority string, prompt, workflowStepID string,
	planMode bool, attachments []v1.MessageAttachment) error {
	return f(ctx, taskID, agentProfileID, executorID, executorProfileID,
		priority, prompt, workflowStepID, planMode, attachments)
}

// TaskStarterWithEnvFunc adapts a function to TaskStarterWithEnv.
type TaskStarterWithEnvFunc func(ctx context.Context, taskID, agentProfileID, executorID,
	executorProfileID string, priority string, prompt, workflowStepID string,
	planMode bool, attachments []v1.MessageAttachment, env map[string]string) error

// StartTask implements TaskStarter.
func (f TaskStarterWithEnvFunc) StartTask(ctx context.Context, taskID, agentProfileID, executorID,
	executorProfileID string, priority string, prompt, workflowStepID string,
	planMode bool, attachments []v1.MessageAttachment) error {
	return f(ctx, taskID, agentProfileID, executorID, executorProfileID,
		priority, prompt, workflowStepID, planMode, attachments, nil)
}

// StartTaskWithEnv implements TaskStarterWithEnv.
func (f TaskStarterWithEnvFunc) StartTaskWithEnv(ctx context.Context, taskID, agentProfileID, executorID,
	executorProfileID string, priority string, prompt, workflowStepID string,
	planMode bool, attachments []v1.MessageAttachment, env map[string]string) error {
	return f(ctx, taskID, agentProfileID, executorID, executorProfileID,
		priority, prompt, workflowStepID, planMode, attachments, env)
}

// WorkspaceCreator creates a DB workspace row for kanban compatibility.
// Implemented by the task service or its repository.
type WorkspaceCreator interface {
	CreateWorkspace(ctx context.Context, name, description string) error
	// FindWorkspaceIDByName returns the kanban workspace UUID for a given name.
	// Returns empty string if not found.
	FindWorkspaceIDByName(ctx context.Context, name string) (string, error)
}

// TaskCreator creates a task in the kanban system.
// Implemented by the task service; the office package depends only on this
// interface to avoid a direct import of the task package.
type TaskCreator interface {
	CreateOfficeTask(ctx context.Context, workspaceID, projectID, assigneeAgentID, title, description string) (taskID string, err error)
}

// SubtaskCreator creates child tasks in the kanban system.
// Implemented by the production task adapter; optional in older tests.
type SubtaskCreator interface {
	CreateOfficeSubtask(ctx context.Context, parentTaskID, assigneeAgentID, title, description string) (taskID string, err error)
}

// TaskPRLink is the minimal projection of a github_task_prs row needed to
// build a per-child summary. The office package keeps this typed-but-tiny
// shape rather than importing internal/github so the dep graph stays
// acyclic.
type TaskPRLink struct {
	URL    string
	Title  string
	Number int
	State  string
}

// TaskPRLister returns the PR associations for a given task id, keyed by
// task. Implemented by the github store; the office package depends only
// on this narrow interface so it can stay github-package-free.
//
// When unset on the office service, child summaries omit PRLinks — the
// engine payload simply carries empty PRLinks slices, which the engine
// dispatch path handles cleanly.
type TaskPRLister interface {
	ListTaskPRsByTaskIDs(ctx context.Context, taskIDs []string) (map[string][]TaskPRLink, error)
}

// ServiceOptions holds all dependencies for the office Service constructor.
// Required fields: Repo and Logger. All other fields are optional and may be
// set to nil/zero to disable the corresponding feature.
type ServiceOptions struct {
	Repo                    *sqlite.Repository
	Logger                  *logger.Logger
	CfgLoader               *configloader.ConfigLoader
	CfgWriter               *configloader.FileWriter
	GitManager              *configloader.GitManager
	EventBus                bus.EventBus
	TaskStarter             TaskStarter
	TaskCanceller           TaskCanceller
	TaskWorkspace           TaskWorkspaceService
	TaskCreator             TaskCreator
	WorkspaceCreator        WorkspaceCreator
	AgentTypeResolver       AgentTypeResolver
	ProjectSkillDirResolver ProjectSkillDirResolver
	TaskPRs                 TaskPRLister
	APIBaseURL              string
	AgentctlBinaryPath      string
}

// Service provides office business logic.
type Service struct {
	repo                    *sqlite.Repository
	cfgLoader               *configloader.ConfigLoader
	cfgWriter               *configloader.FileWriter
	gitManager              *configloader.GitManager
	logger                  *logger.Logger
	eb                      bus.EventBus
	relay                   *ChannelRelay
	agentTypeResolver       AgentTypeResolver
	projectSkillDirResolver ProjectSkillDirResolver
	taskStarter             TaskStarter
	routingDispatcher       RoutingDispatcher
	taskCanceller           TaskCanceller
	taskWorkspace           TaskWorkspaceService
	taskCreator             TaskCreator
	workspaceCreator        WorkspaceCreator
	taskPRs                 TaskPRLister
	agentTokenMinter        AgentTokenMinter
	runsService             *runsservice.Service
	apiBaseURL              string
	agentctlBinaryPath      string
	syncHandlers            bool // when true, event handlers run synchronously (for tests)

	// Phase 4 (ADR-0004): engine-driven run queuing. When set, the four
	// event subscribers (comment_created, blockers_resolved,
	// children_completed, approval_resolved) route through the workflow
	// engine. There is no legacy fallback path — if the engine cannot
	// evaluate the trigger (no session yet) the trigger is dropped with
	// a debug log.
	engineDispatcher WorkflowEngineDispatcher

	// pricingLookup resolves per-model pricing for the cost subscriber's
	// Layer B fallback (models.dev). Optional — nil means Layer B always
	// misses and rows get cost_subcents=0 + estimated=true.
	pricingLookup shared.PricingLookup

	// sessionUsageWriter accumulates tokens / cost onto task_sessions
	// when a cost event lands. Optional in tests.
	sessionUsageWriter shared.SessionUsageWriter

	// budgetChecker evaluates budget policies. Wired to the
	// costs.CostService at startup; nil in tests that don't exercise
	// the budget pathways. Both CheckBudget and CheckPreExecutionBudget
	// delegate to it.
	budgetChecker BudgetEvaluator
}

// BudgetEvaluator is the surface the office service needs from the
// costs feature for budget evaluation. Implemented by
// *costs.CostService.EvaluateBudget — declared here so tests can supply
// fakes without pulling the costs package.
type BudgetEvaluator interface {
	CheckPreExecutionBudget(ctx context.Context, agentInstanceID, projectID, workspaceID string) (bool, string, error)
	// EvaluateBudget runs the post-event budget check (alerts, agent
	// pause). The office service discards the per-policy results; the
	// costs package is responsible for any side effects.
	EvaluateBudget(ctx context.Context, workspaceID, agentInstanceID, projectID string) error
}

// SetBudgetChecker wires the costs.CostService (or a test fake) as the
// budget evaluator. Without this wired, CheckBudget and
// CheckPreExecutionBudget treat all runs as unrestricted.
func (s *Service) SetBudgetChecker(b BudgetEvaluator) { s.budgetChecker = b }

// SetPricingLookup wires the models.dev pricing lookup.
func (s *Service) SetPricingLookup(p shared.PricingLookup) { s.pricingLookup = p }

// SetSessionUsageWriter wires the task-session usage incrementer.
func (s *Service) SetSessionUsageWriter(w shared.SessionUsageWriter) { s.sessionUsageWriter = w }

// SetAgentTokenMinter wires the runtime token minter after feature services are constructed.
func (s *Service) SetAgentTokenMinter(minter AgentTokenMinter) {
	s.agentTokenMinter = minter
}

// SetRoutingDispatcher wires the provider-routing dispatcher (the office
// scheduler.SchedulerService). When nil, the legacy concrete-profile
// launch path runs unchanged.
func (s *Service) SetRoutingDispatcher(rd RoutingDispatcher) {
	s.routingDispatcher = rd
}

// RoutingDispatcherHandle returns the wired routing dispatcher, if any.
// Exposed so the AgentFailed / AgentCompleted subscribers can call
// post-start hooks without a separate handle.
func (s *Service) RoutingDispatcherHandle() RoutingDispatcher {
	return s.routingDispatcher
}

// SetRunsService wires the new internal/runs/service so office.QueueRun
// can delegate the insert + publish + signal to the runs queue
// service. Optional: when nil, office.QueueRun falls back to the
// in-package implementation that writes through the office repo.
func (s *Service) SetRunsService(runs *runsservice.Service) {
	s.runsService = runs
}

// CancelTaskExecution delegates to the configured TaskCanceller (the
// orchestrator). Returns an error if no canceller is configured.
// Exposed on Service so callers (e.g. the dashboard reactivity pipeline)
// can hard-cancel tasks without holding a separate reference.
func (s *Service) CancelTaskExecution(ctx context.Context, taskID, reason string, force bool) error {
	if s.taskCanceller == nil {
		return fmt.Errorf("task canceller not configured")
	}
	return s.taskCanceller.CancelTaskExecution(ctx, taskID, reason, force)
}

// SetSyncHandlers makes event handlers run synchronously instead of in
// goroutines. Call before RegisterEventSubscribers in tests that assert
// handler effects immediately after Publish.
func (s *Service) SetSyncHandlers(sync bool) {
	s.syncHandlers = sync
}

// NewService creates a new office service. All dependencies are provided via
// ServiceOptions for compile-time completeness; only Repo and Logger are required.
func NewService(opts ServiceOptions) *Service {
	svc := &Service{
		repo:                    opts.Repo,
		logger:                  opts.Logger.WithFields(zap.String("component", "office-service")),
		cfgLoader:               opts.CfgLoader,
		cfgWriter:               opts.CfgWriter,
		gitManager:              opts.GitManager,
		eb:                      opts.EventBus,
		taskStarter:             opts.TaskStarter,
		taskCanceller:           opts.TaskCanceller,
		taskWorkspace:           opts.TaskWorkspace,
		taskCreator:             opts.TaskCreator,
		workspaceCreator:        opts.WorkspaceCreator,
		taskPRs:                 opts.TaskPRs,
		agentTypeResolver:       opts.AgentTypeResolver,
		projectSkillDirResolver: opts.ProjectSkillDirResolver,
		apiBaseURL:              opts.APIBaseURL,
		agentctlBinaryPath:      opts.AgentctlBinaryPath,
	}
	svc.relay = NewChannelRelay(svc)
	return svc
}

// SetAgentctlBinaryPath overrides the host path to the agentctl binary.
// Prefer passing AgentctlBinaryPath via ServiceOptions. This setter exists
// for cases where the path must be updated after construction (e.g. tests).
func (s *Service) SetAgentctlBinaryPath(path string) {
	s.agentctlBinaryPath = path
}

// GitManager returns the git manager (may be nil).
func (s *Service) GitManager() *configloader.GitManager {
	return s.gitManager
}

// ConfigLoader returns the filesystem config loader (may be nil).
func (s *Service) ConfigLoader() *configloader.ConfigLoader {
	return s.cfgLoader
}

// ConfigWriter returns the filesystem config writer (may be nil).
func (s *Service) ConfigWriter() *configloader.FileWriter {
	return s.cfgWriter
}

// defaultWorkspaceName is used for ConfigLoader lookups when we only have a
// DB workspace ID. Most single-user installs have one workspace named "default".
const defaultWorkspaceName = "default"

// Agent instance methods (CRUD + validation + status transitions) are in agents.go.

// -- Task creation --

// CreateOfficeTaskAsAgent checks can_create_tasks for the given caller before
// delegating to the TaskCreator. Passing callerAgentID="" skips the check
// (for internal/admin callers).
func (s *Service) CreateOfficeTaskAsAgent(
	ctx context.Context, callerAgentID, workspaceID, projectID, assigneeAgentID, title, description string,
) (string, error) {
	if err := s.requireTaskCreatePermission(ctx, callerAgentID); err != nil {
		return "", err
	}
	if s.taskCreator == nil {
		return "", fmt.Errorf("task creator not configured")
	}
	return s.taskCreator.CreateOfficeTask(ctx, workspaceID, projectID, assigneeAgentID, title, description)
}

// CreateOfficeSubtaskAsAgent checks can_create_tasks for the caller before
// creating a child task under parentTaskID.
func (s *Service) CreateOfficeSubtaskAsAgent(
	ctx context.Context, callerAgentID, parentTaskID, assigneeAgentID, title, description string,
) (string, error) {
	if err := s.requireTaskCreatePermission(ctx, callerAgentID); err != nil {
		return "", err
	}
	if s.taskCreator == nil {
		return "", fmt.Errorf("task creator not configured")
	}
	creator, ok := s.taskCreator.(SubtaskCreator)
	if !ok {
		return "", fmt.Errorf("subtask creator not configured")
	}
	return creator.CreateOfficeSubtask(ctx, parentTaskID, assigneeAgentID, title, description)
}

func (s *Service) requireTaskCreatePermission(ctx context.Context, callerAgentID string) error {
	if callerAgentID == "" {
		return nil
	}
	agent, err := s.repo.GetAgentInstance(ctx, callerAgentID)
	if err != nil {
		return fmt.Errorf("resolve caller: %w", err)
	}
	perms := shared.ResolvePermissions(shared.AgentRole(agent.Role), agent.Permissions)
	if !shared.HasPermission(perms, shared.PermCanCreateTasks) {
		return shared.ErrForbidden
	}
	return nil
}

// -- Skills --

// CreateSkill creates a new skill in the DB.
func (s *Service) CreateSkill(ctx context.Context, skill *models.Skill) error {
	if skill.SourceType == "" {
		skill.SourceType = SkillSourceTypeInline
	}
	if skill.FileInventory == "" {
		skill.FileInventory = "[]"
	}
	prepareServiceSkillPackageMetadata(skill)
	if err := s.repo.CreateSkill(ctx, skill); err != nil {
		return fmt.Errorf("create skill: %w", err)
	}
	return nil
}

// GetSkill returns a skill by ID.
func (s *Service) GetSkill(ctx context.Context, id string) (*models.Skill, error) {
	return s.GetSkillFromConfig(ctx, id)
}

// ListSkills returns all skills for a workspace.
func (s *Service) ListSkills(ctx context.Context, wsID string) ([]*models.Skill, error) {
	return s.ListSkillsFromConfig(ctx, wsID)
}

// UpdateSkill updates a skill in the DB.
func (s *Service) UpdateSkill(ctx context.Context, skill *models.Skill) error {
	prepareServiceSkillPackageMetadata(skill)
	if err := s.repo.UpdateSkill(ctx, skill); err != nil {
		return fmt.Errorf("update skill: %w", err)
	}
	return nil
}

func prepareServiceSkillPackageMetadata(skill *models.Skill) {
	if skill.Version == "" {
		skill.Version = "1"
	}
	if skill.ApprovalState == "" {
		skill.ApprovalState = "approved"
	}
	sum := sha256.Sum256([]byte(skill.Content + "\x00" + skill.FileInventory + "\x00" + skill.SourceLocator))
	skill.ContentHash = hex.EncodeToString(sum[:])
}

// DeleteSkill deletes a skill from the DB.
func (s *Service) DeleteSkill(ctx context.Context, id string) error {
	skill, err := s.GetSkillFromConfig(ctx, id)
	if err != nil {
		return err
	}
	if err := s.repo.DeleteSkill(ctx, skill.ID); err != nil {
		return fmt.Errorf("delete skill: %w", err)
	}
	return nil
}

// -- Projects --

// CreateProject validates and creates a new project in the DB.
func (s *Service) CreateProject(ctx context.Context, project *models.Project) error {
	if err := s.validateProject(project); err != nil {
		return err
	}
	if project.Status == "" {
		project.Status = models.ProjectStatusActive
	}
	if project.Repositories == "" {
		project.Repositories = "[]"
	}
	if project.ExecutorConfig == "" {
		project.ExecutorConfig = "{}"
	}
	if err := s.repo.CreateProject(ctx, project); err != nil {
		return fmt.Errorf("create project: %w", err)
	}
	s.logger.Info("project created",
		zap.String("project_id", project.ID),
		zap.String("name", project.Name))
	return nil
}

// GetProject returns a project by ID.
func (s *Service) GetProject(ctx context.Context, id string) (*models.Project, error) {
	return s.GetProjectFromConfig(ctx, id)
}

// ListProjects returns all projects for a workspace.
func (s *Service) ListProjects(ctx context.Context, wsID string) ([]*models.Project, error) {
	return s.ListProjectsFromConfig(ctx, wsID)
}

// ListProjectsWithCounts returns all projects with aggregated task counts.
func (s *Service) ListProjectsWithCounts(ctx context.Context, wsID string) ([]*models.ProjectWithCounts, error) {
	return s.ListProjectsWithCountsFromConfig(ctx, wsID)
}

// UpdateProject validates and updates a project in the DB.
func (s *Service) UpdateProject(ctx context.Context, project *models.Project) error {
	if err := s.validateProject(project); err != nil {
		return err
	}
	if err := s.repo.UpdateProject(ctx, project); err != nil {
		return fmt.Errorf("update project: %w", err)
	}
	s.logger.Info("project updated",
		zap.String("project_id", project.ID),
		zap.String("name", project.Name))
	return nil
}

// DeleteProject deletes a project from the DB.
func (s *Service) DeleteProject(ctx context.Context, id string) error {
	project, err := s.GetProjectFromConfig(ctx, id)
	if err != nil {
		return err
	}
	if err := s.repo.DeleteProject(ctx, project.ID); err != nil {
		return fmt.Errorf("delete project: %w", err)
	}
	return nil
}

func (s *Service) validateProject(project *models.Project) error {
	if project.Name == "" {
		return fmt.Errorf("project name is required")
	}
	if project.WorkspaceID == "" {
		return fmt.Errorf("workspace ID is required")
	}
	if project.Status != "" && !models.ValidProjectStatuses[project.Status] {
		return fmt.Errorf("invalid project status: %s", project.Status)
	}
	return validateRepositories(project.Repositories)
}

func validateRepositories(reposJSON string) error {
	if reposJSON == "" || reposJSON == "[]" {
		return nil
	}
	var repos []string
	if err := json.Unmarshal([]byte(reposJSON), &repos); err != nil {
		return fmt.Errorf("repositories must be a JSON array of strings: %w", err)
	}
	for _, repo := range repos {
		if strings.TrimSpace(repo) == "" {
			return fmt.Errorf("repository entry must not be empty")
		}
	}
	return nil
}

// -- Costs --

// ListCostEvents returns cost events for a workspace.
func (s *Service) ListCostEvents(ctx context.Context, wsID string) ([]*models.CostEvent, error) {
	return s.repo.ListCostEvents(ctx, wsID)
}

// GetCostsByAgent returns costs grouped by agent.
func (s *Service) GetCostsByAgent(ctx context.Context, wsID string) ([]*models.CostBreakdown, error) {
	return s.repo.GetCostsByAgent(ctx, wsID)
}

// GetCostsByProject returns costs grouped by project.
func (s *Service) GetCostsByProject(ctx context.Context, wsID string) ([]*models.CostBreakdown, error) {
	return s.repo.GetCostsByProject(ctx, wsID)
}

// GetCostsByModel returns costs grouped by model.
func (s *Service) GetCostsByModel(ctx context.Context, wsID string) ([]*models.CostBreakdown, error) {
	return s.repo.GetCostsByModel(ctx, wsID)
}

// -- Budgets --

// CheckPreExecutionBudget delegates to the wired BudgetEvaluator (the
// costs.CostService). Returns (true, "", nil) when no evaluator is
// configured so tests and feature-light deployments don't block runs.
func (s *Service) CheckPreExecutionBudget(
	ctx context.Context, agentInstanceID, projectID, workspaceID string,
) (bool, string, error) {
	if s.budgetChecker == nil {
		return true, "", nil
	}
	return s.budgetChecker.CheckPreExecutionBudget(ctx, agentInstanceID, projectID, workspaceID)
}

// CheckBudget delegates to the wired BudgetEvaluator. The post-event
// subscriber path uses this to fire alerts and pause agents when a
// policy is exceeded after a cost event lands. No-op when no evaluator
// is configured.
func (s *Service) CheckBudget(ctx context.Context, workspaceID, agentInstanceID, projectID string) error {
	if s.budgetChecker == nil {
		return nil
	}
	return s.budgetChecker.EvaluateBudget(ctx, workspaceID, agentInstanceID, projectID)
}

// CreateBudgetPolicy creates a new budget policy.
func (s *Service) CreateBudgetPolicy(ctx context.Context, policy *models.BudgetPolicy) error {
	return s.repo.CreateBudgetPolicy(ctx, policy)
}

// ListBudgetPolicies returns all budget policies for a workspace.
func (s *Service) ListBudgetPolicies(ctx context.Context, wsID string) ([]*models.BudgetPolicy, error) {
	return s.repo.ListBudgetPolicies(ctx, wsID)
}

// GetBudgetPolicy returns a budget policy by ID.
func (s *Service) GetBudgetPolicy(ctx context.Context, id string) (*models.BudgetPolicy, error) {
	return s.repo.GetBudgetPolicy(ctx, id)
}

// UpdateBudgetPolicy updates a budget policy.
func (s *Service) UpdateBudgetPolicy(ctx context.Context, policy *models.BudgetPolicy) error {
	return s.repo.UpdateBudgetPolicy(ctx, policy)
}

// DeleteBudgetPolicy deletes a budget policy.
func (s *Service) DeleteBudgetPolicy(ctx context.Context, id string) error {
	return s.repo.DeleteBudgetPolicy(ctx, id)
}

// -- Routines --

// CreateRoutine creates a new routine in the DB.
func (s *Service) CreateRoutine(ctx context.Context, routine *models.Routine) error {
	if err := s.repo.CreateRoutine(ctx, routine); err != nil {
		return fmt.Errorf("create routine: %w", err)
	}
	return nil
}

// GetRoutine returns a routine by ID.
func (s *Service) GetRoutine(ctx context.Context, id string) (*models.Routine, error) {
	return s.GetRoutineFromConfig(ctx, id)
}

// ListRoutines returns all routines for a workspace.
func (s *Service) ListRoutines(ctx context.Context, wsID string) ([]*models.Routine, error) {
	return s.ListRoutinesFromConfig(ctx, wsID)
}

// UpdateRoutine updates a routine in the DB.
func (s *Service) UpdateRoutine(ctx context.Context, routine *models.Routine) error {
	if err := s.repo.UpdateRoutine(ctx, routine); err != nil {
		return fmt.Errorf("update routine: %w", err)
	}
	return nil
}

// DeleteRoutine deletes a routine from the DB.
func (s *Service) DeleteRoutine(ctx context.Context, id string) error {
	routine, err := s.GetRoutineFromConfig(ctx, id)
	if err != nil {
		return err
	}
	if err := s.repo.DeleteRoutine(ctx, routine.ID); err != nil {
		return fmt.Errorf("delete routine: %w", err)
	}
	return nil
}

// -- Approvals --

// CreateApproval creates a new approval.
func (s *Service) CreateApproval(ctx context.Context, approval *models.Approval) error {
	return s.repo.CreateApproval(ctx, approval)
}

// ListApprovals returns all approvals for a workspace.
func (s *Service) ListApprovals(ctx context.Context, wsID string) ([]*models.Approval, error) {
	return s.repo.ListApprovals(ctx, wsID)
}

// UpdateApproval updates an approval (for deciding).
func (s *Service) UpdateApproval(ctx context.Context, approval *models.Approval) error {
	return s.repo.UpdateApproval(ctx, approval)
}

// GetApproval returns an approval by ID.
func (s *Service) GetApproval(ctx context.Context, id string) (*models.Approval, error) {
	return s.repo.GetApproval(ctx, id)
}

// -- Activity --

// ListActivity returns recent activity entries for a workspace.
func (s *Service) ListActivity(ctx context.Context, wsID string, limit int) ([]*models.ActivityEntry, error) {
	return s.repo.ListActivityEntries(ctx, wsID, limit)
}

// -- Memory --

// ListAgentMemory returns all memory entries for an agent.
func (s *Service) ListAgentMemory(ctx context.Context, agentID string) ([]*models.AgentMemory, error) {
	return s.repo.ListAgentMemory(ctx, agentID)
}

// UpsertAgentMemory creates or updates an agent memory entry.
func (s *Service) UpsertAgentMemory(ctx context.Context, mem *models.AgentMemory) error {
	return s.repo.UpsertAgentMemory(ctx, mem)
}

// DeleteAgentMemory deletes a memory entry.
func (s *Service) DeleteAgentMemory(ctx context.Context, id string) error {
	return s.repo.DeleteAgentMemory(ctx, id)
}

// -- Task Checkout --

// CheckoutTask atomically acquires an exclusive lock on a task for an agent.
func (s *Service) CheckoutTask(ctx context.Context, taskID, agentID string) (bool, error) {
	return s.repo.CheckoutTask(ctx, taskID, agentID)
}

// ReleaseTaskCheckout releases the exclusive lock on a task.
func (s *Service) ReleaseTaskCheckout(ctx context.Context, taskID string) error {
	return s.repo.ReleaseTaskCheckout(ctx, taskID)
}

// -- Runs --

// ListRuns returns run requests for a workspace.
func (s *Service) ListRuns(ctx context.Context, wsID string) ([]*models.Run, error) {
	return s.repo.ListRuns(ctx, wsID)
}

// -- Task Search --

// SearchTasks searches for tasks matching the query string in a workspace.
func (s *Service) SearchTasks(ctx context.Context, wsID, query string, limit int) ([]*sqlite.TaskSearchResult, error) {
	return s.repo.SearchTasks(ctx, wsID, query, limit)
}

// CreateOfficeWorkspace writes workspace config to the filesystem and,
// if a WorkspaceCreator is configured, also creates a DB workspace row for
// kanban board compatibility.
func (s *Service) CreateOfficeWorkspace(ctx context.Context, name, description string) error {
	if !isValidPathComponent(name) {
		return fmt.Errorf("invalid workspace name")
	}

	// 1. Write kandev.yml to filesystem.
	if s.cfgWriter != nil {
		settings := &configloader.WorkspaceSettings{
			Name:        name,
			Slug:        generateSlug(name),
			Description: description,
			TaskPrefix:  "KAN",
		}
		if err := s.writeWorkspaceConfig(name, settings); err != nil {
			return err
		}
	}

	// 2. Create DB row for kanban compatibility.
	if s.workspaceCreator != nil {
		if err := s.workspaceCreator.CreateWorkspace(ctx, name, description); err != nil {
			s.logger.Warn("dual workspace DB creation failed",
				zap.String("name", name), zap.Error(err))
		}
	}
	return nil
}

var (
	slugNonAlphanumRe = regexp.MustCompile(`[^a-z0-9-]`)
	slugMultiDashRe   = regexp.MustCompile(`-+`)
)

// generateSlug creates a URL-safe slug from a workspace name.
func generateSlug(name string) string {
	slug := strings.ToLower(name)
	slug = slugNonAlphanumRe.ReplaceAllString(slug, "-")
	slug = slugMultiDashRe.ReplaceAllString(slug, "-")
	slug = strings.Trim(slug, "-")
	if slug == "" {
		slug = "workspace"
	}
	if len(slug) > 50 {
		slug = slug[:50]
	}
	return slug
}

// writeWorkspaceConfig marshals settings and writes them to the workspace directory.
func (s *Service) writeWorkspaceConfig(name string, settings *configloader.WorkspaceSettings) error {
	if !isValidPathComponent(name) {
		return fmt.Errorf("invalid workspace name")
	}
	data, err := configloader.MarshalSettings(*settings)
	if err != nil {
		return fmt.Errorf("marshal settings: %w", err)
	}
	wsDir := filepath.Join(s.cfgLoader.BasePath(), "workspaces", name)
	if mkErr := os.MkdirAll(wsDir, 0o755); mkErr != nil {
		return fmt.Errorf("create dir: %w", mkErr)
	}
	settingsPath := filepath.Join(wsDir, "kandev.yml")
	if writeErr := os.WriteFile(settingsPath, data, 0o644); writeErr != nil {
		return fmt.Errorf("write settings: %w", writeErr)
	}
	if reloadErr := s.cfgLoader.Reload(name); reloadErr != nil {
		return fmt.Errorf("reload config: %w", reloadErr)
	}
	return nil
}
