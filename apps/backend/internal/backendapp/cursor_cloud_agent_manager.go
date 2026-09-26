package backendapp

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/runtime"
	cursorcloudruntime "github.com/kandev/kandev/internal/agent/runtime/cursorcloud"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	agentruntime "github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/cursorcloud"
	"github.com/kandev/kandev/internal/github"
	"github.com/kandev/kandev/internal/mcp/managed"
	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/secrets"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	taskservice "github.com/kandev/kandev/internal/task/service"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

type cursorCloudAgentManager struct {
	*lifecycleAdapter
	repo                            *sqliterepo.Repository
	taskService                     *taskservice.Service
	github                          *github.Service
	secrets                         secrets.SecretStore
	runtime                         *runtime.Router
	managedRuntime                  *cursorcloudruntime.Runtime
	enabled                         func() bool
	turnMu                          sync.Mutex
	pendingTurns                    map[string]string
	observerMu                      sync.Mutex
	observerCtx                     context.Context
	observerCancel                  context.CancelFunc
	observerCancels                 map[string]context.CancelFunc
	observerStopping                bool
	observerWG                      sync.WaitGroup
	localPromptWithDispatchCallback func(context.Context, string, string, []v1.MessageAttachment, bool, func()) (*executor.PromptResult, error)
}

const (
	cursorCloudMCPModeTask             = "task"
	cursorCloudMCPModeTaskTitlePending = "task-title-pending"
)

func newCursorCloudAgentManager(
	base *lifecycleAdapter,
	repo *sqliterepo.Repository,
	taskSvc *taskservice.Service,
	githubSvc *github.Service,
	secretStore secrets.SecretStore,
	enabled func() bool,
) (*cursorCloudAgentManager, error) {
	if base == nil || repo == nil || taskSvc == nil || secretStore == nil || enabled == nil {
		return nil, errors.New("cursor cloud agent manager dependencies are incomplete")
	}
	managedRuntime, err := newManagedCursorCloudRuntime(
		repo, secretStore, enabled, base.mgr.PublishAgentStreamEventPayload,
	)
	if err != nil {
		return nil, err
	}
	router, err := runtime.NewRouter(runtime.New(base.mgr), managedRuntimeRepository{Repository: repo},
		map[string]runtime.Runtime{string(agentruntime.RuntimeCursorCloud): managedRuntime})
	if err != nil {
		return nil, err
	}
	return &cursorCloudAgentManager{
		lifecycleAdapter: base, repo: repo, taskService: taskSvc, github: githubSvc,
		secrets: secretStore, runtime: router, managedRuntime: managedRuntime, enabled: enabled,
		localPromptWithDispatchCallback: base.PromptAgentWithDispatchCallback,
		pendingTurns:                    make(map[string]string),
	}, nil
}

func newManagedCursorCloudRuntime(
	repo *sqliterepo.Repository,
	secretStore secrets.SecretStore,
	enabled func() bool,
	publishStream func(*runtime.AgentStreamEventPayload),
) (*cursorcloudruntime.Runtime, error) {
	if publishStream == nil {
		return nil, errors.New("cursor cloud stream publisher is required")
	}
	managedRepository := managedRuntimeRepository{Repository: repo}
	return cursorcloudruntime.New(cursorcloudruntime.Config{
		Repository: managedRepository,
		Enabled:    enabled,
		ClientFactory: func(ctx context.Context, binding *models.ManagedAgentBinding) (cursorcloudruntime.Provider, error) {
			if err := secrets.ValidateGlobalReference(ctx, secretStore, binding.CredentialRef); err != nil {
				return nil, fmt.Errorf("cursor cloud credential reference is unavailable")
			}
			apiKey, err := secretStore.Reveal(ctx, binding.CredentialRef)
			if err != nil || strings.TrimSpace(apiKey) == "" {
				return nil, fmt.Errorf("cursor cloud credential is unavailable")
			}
			return cursorcloud.NewRuntimeClient(apiKey, enabled())
		},
		GrantIssuer: cursorcloudruntime.GrantIssuerFunc(func(ctx context.Context, binding *models.ManagedAgentBinding, operation *models.ManagedAgentOperation) (cursorcloudruntime.MCPGrant, error) {
			capabilities := []mcpprofile.Capability{mcpprofile.CapabilityUserQuestion, mcpprofile.CapabilityTaskTitle}
			if operation.RequestSnapshot.MCPMode == cursorCloudMCPModeTaskTitlePending {
				capabilities = []mcpprofile.Capability{mcpprofile.CapabilityTaskTitle}
			}
			toolProfile := mcpprofile.New(mcpprofile.SurfaceManagedTask, capabilities, nil)
			issued, err := managed.IssueGrant(ctx, repo, binding, operation, toolProfile, time.Now().UTC(), managed.MaxGrantLifetime)
			if err != nil {
				return cursorcloudruntime.MCPGrant{}, err
			}
			return cursorcloudruntime.MCPGrant{URL: issued.URL, Token: issued.Token}, nil
		}),
		ProjectStream: projectCursorCloudStream,
		PublishStream: func(_ context.Context, payload *runtime.AgentStreamEventPayload) {
			publishStream(payload)
		},
	})
}

// managedRuntimeRepository converts durable repository absence into the
// runtime facade's single not-found sentinel for local-runtime fallback.
type managedRuntimeRepository struct{ *sqliterepo.Repository }

func (r managedRuntimeRepository) RuntimeForExecution(ctx context.Context, executionID string) (string, error) {
	binding, err := r.GetManagedAgentBindingByExecution(ctx, executionID)
	if errors.Is(err, repository.ErrManagedAgentBindingNotFound) {
		return "", runtime.ErrNotFound
	}
	if err != nil {
		return "", err
	}
	return binding.ProviderKind, nil
}

func (m *cursorCloudAgentManager) LaunchAgent(ctx context.Context, req *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
	if req == nil || req.ExecutorType != string(models.ExecutorTypeCursorCloud) {
		return m.lifecycleAdapter.LaunchAgent(ctx, req)
	}
	if !m.enabled() {
		return nil, errors.New("cursor cloud is disabled for new launches")
	}
	input, err := m.prepareCloudLaunch(ctx, req)
	if err != nil {
		return nil, err
	}
	ref, err := m.runtime.Launch(ctx, runtime.LaunchSpec{
		AgentProfileID: req.AgentProfileID, ExecutorID: req.ExecutorType,
		RuntimeName: string(agentruntime.RuntimeCursorCloud), Prompt: req.TaskDescription,
		Metadata: map[string]any{cursorcloudruntime.LaunchMetadataKey: input},
	})
	if err != nil {
		return nil, err
	}
	row := &models.ExecutorRunning{
		ID: uuid.NewString(), SessionID: ref.SessionID, TaskID: input.Binding.TaskID,
		ExecutionProfileID: req.AgentProfileID, ExecutorID: input.Binding.ExecutorID,
		Runtime: agentruntime.RuntimeCursorCloud, Status: models.ExecutorRunningStatusStarting,
		Resumable: true, AgentExecutionID: ref.ID,
		Metadata: map[string]interface{}{"provider_kind": "cursor_cloud"},
	}
	if existing, getErr := m.repo.GetExecutorRunningBySessionID(ctx, ref.SessionID); getErr == nil && existing != nil {
		row.ID = existing.ID
	}
	if err := m.repo.UpsertExecutorRunning(ctx, row); err != nil {
		return nil, fmt.Errorf("persist Cursor Cloud execution inventory: %w", err)
	}
	return &executor.LaunchAgentResponse{
		AgentExecutionID: ref.ID, Status: v1.AgentStatusRunning,
		Metadata: map[string]interface{}{"runtime": string(agentruntime.RuntimeCursorCloud)},
	}, nil
}

func (m *cursorCloudAgentManager) StartAgentProcess(ctx context.Context, executionID string) error {
	binding, err := m.repo.GetManagedAgentBindingByExecution(ctx, executionID)
	if errors.Is(err, repository.ErrManagedAgentBindingNotFound) {
		return m.lifecycleAdapter.StartAgentProcess(ctx, executionID)
	}
	if err != nil {
		return err
	}
	if !m.enabled() {
		return errors.New("cursor cloud is disabled for new dispatches")
	}
	if err := m.runtime.StartExecution(ctx, binding.ExecutionID); err != nil {
		return err
	}
	m.startManagedObserver(binding.ExecutionID, "disconnect")
	return nil
}

func (m *cursorCloudAgentManager) IsAgentCommandConfigured(executionID string) bool {
	binding, err := m.repo.GetManagedAgentBindingByExecution(context.Background(), executionID)
	if err == nil {
		return binding != nil && binding.Launch.Model != "" && binding.RemoteAgentID != ""
	}
	return m.lifecycleAdapter.IsAgentCommandConfigured(executionID)
}

func (m *cursorCloudAgentManager) PromptAgent(ctx context.Context, executionID, prompt string, attachments []v1.MessageAttachment, dispatchOnly bool) (*executor.PromptResult, error) {
	return m.promptCloud(ctx, executionID, prompt, attachments, dispatchOnly, nil)
}

func (m *cursorCloudAgentManager) PromptAgentWithDispatchCallback(ctx context.Context, executionID, prompt string, attachments []v1.MessageAttachment, dispatchOnly bool, onDispatched func()) (*executor.PromptResult, error) {
	return m.promptCloud(ctx, executionID, prompt, attachments, dispatchOnly, onDispatched)
}

func (m *cursorCloudAgentManager) promptCloud(ctx context.Context, executionID, prompt string, attachments []v1.MessageAttachment, dispatchOnly bool, onDispatched func()) (*executor.PromptResult, error) {
	binding, err := m.repo.GetManagedAgentBindingByExecution(ctx, executionID)
	if errors.Is(err, repository.ErrManagedAgentBindingNotFound) {
		if m.localPromptWithDispatchCallback != nil {
			return m.localPromptWithDispatchCallback(ctx, executionID, prompt, attachments, dispatchOnly, onDispatched)
		}
		return m.lifecycleAdapter.PromptAgentWithDispatchCallback(ctx, executionID, prompt, attachments, dispatchOnly, onDispatched)
	}
	if err != nil {
		return nil, err
	}
	if !m.enabled() {
		return nil, errors.New("cursor cloud is disabled for new dispatches")
	}
	if len(attachments) != 0 {
		return nil, errors.New("cursor cloud does not support prompt attachments")
	}
	task, err := m.repo.GetTask(ctx, binding.TaskID)
	if err != nil || task == nil {
		return nil, errors.New("cursor cloud task is unavailable")
	}
	if task.ArchivedAt != nil {
		return nil, errors.New("cursor cloud cannot accept a message for an archived task")
	}
	if binding.Lifecycle == models.ManagedAgentBindingArchived || binding.Lifecycle == models.ManagedAgentBindingTerminationPending {
		if err := m.repo.ResolveManagedAgentTermination(ctx, binding.ID, models.ManagedAgentBindingReady, time.Now().UTC()); err != nil {
			return nil, fmt.Errorf("resolve Cursor Cloud termination before follow-up: %w", err)
		}
	}
	turnID := m.takePromptTurnID(executionID)
	if err := m.runtime.ResumeWithTurnID(ctx, binding.ExecutionID, turnID, prompt); err != nil {
		return nil, err
	}
	m.startManagedObserver(binding.ExecutionID, "disconnect")
	if onDispatched != nil {
		onDispatched()
	}
	return &executor.PromptResult{StopReason: "accepted"}, nil
}

func (m *cursorCloudAgentManager) SetPromptTurnID(ctx context.Context, executionID, turnID string) error {
	binding, err := m.repo.GetManagedAgentBindingByExecution(ctx, executionID)
	if errors.Is(err, repository.ErrManagedAgentBindingNotFound) {
		if setter, ok := any(m.lifecycleAdapter).(executor.PromptTurnIDSetter); ok {
			return setter.SetPromptTurnID(ctx, executionID, turnID)
		}
		return nil
	}
	if err != nil {
		return err
	}
	if turnID == "" {
		return errors.New("cursor cloud prompt turn ID is required")
	}
	m.turnMu.Lock()
	m.pendingTurns[executionID] = turnID
	m.turnMu.Unlock()
	_ = binding
	return nil
}

func (m *cursorCloudAgentManager) takePromptTurnID(executionID string) string {
	m.turnMu.Lock()
	defer m.turnMu.Unlock()
	turnID := m.pendingTurns[executionID]
	delete(m.pendingTurns, executionID)
	if turnID == "" {
		turnID = uuid.NewString()
	}
	return turnID
}

func (m *cursorCloudAgentManager) CancelAgent(ctx context.Context, sessionID string) error {
	binding, err := m.repo.GetManagedAgentBindingBySession(ctx, sessionID)
	if errors.Is(err, repository.ErrManagedAgentBindingNotFound) {
		return m.lifecycleAdapter.CancelAgent(ctx, sessionID)
	}
	if err != nil {
		return err
	}
	return m.runtime.CancelActive(ctx, binding.ExecutionID)
}

func (m *cursorCloudAgentManager) StopAgent(ctx context.Context, executionID string, force bool) error {
	return m.stopCloud(ctx, executionID, "stopped", force)
}

func (m *cursorCloudAgentManager) StopAgentWithReason(ctx context.Context, executionID, reason string, force bool) error {
	return m.stopCloud(ctx, executionID, reason, force)
}

func (m *cursorCloudAgentManager) stopCloud(ctx context.Context, executionID, reason string, _ bool) error {
	binding, err := m.repo.GetManagedAgentBindingByExecution(ctx, executionID)
	if errors.Is(err, repository.ErrManagedAgentBindingNotFound) {
		return m.lifecycleAdapter.StopAgentWithReason(ctx, executionID, reason, false)
	}
	if err != nil {
		return err
	}
	if err := m.runtime.Stop(ctx, binding.ExecutionID, reason); err != nil {
		return err
	}
	task, err := m.repo.GetTask(ctx, binding.TaskID)
	if err != nil || task == nil {
		return errors.New("cursor cloud task is unavailable while finalizing stop")
	}
	lifecycle := models.ManagedAgentBindingReady
	if task.ArchivedAt != nil {
		lifecycle = models.ManagedAgentBindingArchived
	}
	if err := m.repo.ResolveManagedAgentTermination(ctx, binding.ID, lifecycle, time.Now().UTC()); err != nil {
		return fmt.Errorf("finalize Cursor Cloud termination: %w", err)
	}
	if err := m.repo.DeleteExecutorRunningBySessionID(ctx, binding.SessionID); err != nil &&
		!errors.Is(err, models.ErrExecutorRunningNotFound) {
		return fmt.Errorf("detach confirmed Cursor Cloud execution: %w", err)
	}
	return nil
}

func (m *cursorCloudAgentManager) RespondToPermissionBySessionID(ctx context.Context, sessionID, pendingID, optionID string, cancelled bool) error {
	if m.isCloudSession(ctx, sessionID) {
		return runtime.ErrUnsupported
	}
	return m.lifecycleAdapter.RespondToPermissionBySessionID(ctx, sessionID, pendingID, optionID, cancelled)
}

func (m *cursorCloudAgentManager) ListPendingPermissionsBySessionID(ctx context.Context, sessionID string) ([]streams.PendingAgentPermission, error) {
	if m.isCloudSession(ctx, sessionID) {
		return []streams.PendingAgentPermission{}, nil
	}
	return m.lifecycleAdapter.ListPendingPermissionsBySessionID(ctx, sessionID)
}

func (m *cursorCloudAgentManager) ResolvePermissionBySessionID(ctx context.Context, sessionID, requestID, pendingID, optionID string) (*streams.PermissionResolveResponse, error) {
	if m.isCloudSession(ctx, sessionID) {
		return nil, runtime.ErrUnsupported
	}
	return m.lifecycleAdapter.ResolvePermissionBySessionID(ctx, sessionID, requestID, pendingID, optionID)
}

func (m *cursorCloudAgentManager) CancelPermissionBySessionID(ctx context.Context, sessionID, requestID, pendingID string) (*streams.PermissionCancelResponse, error) {
	if m.isCloudSession(ctx, sessionID) {
		return nil, runtime.ErrUnsupported
	}
	return m.lifecycleAdapter.CancelPermissionBySessionID(ctx, sessionID, requestID, pendingID)
}

func (m *cursorCloudAgentManager) ProbeBackgroundWorkloads(ctx context.Context, sessionID string) (runtime.ProbeResult, error) {
	if m.isCloudSession(ctx, sessionID) {
		return runtime.ProbeResultUnknown, nil
	}
	return m.lifecycleAdapter.ProbeBackgroundWorkloads(ctx, sessionID)
}

func (m *cursorCloudAgentManager) IsAgentRunningForSession(ctx context.Context, sessionID string) bool {
	running, err := m.ProbeAgentRunningForSession(ctx, sessionID)
	return err != nil || running
}

func (m *cursorCloudAgentManager) ProbeAgentRunningForSession(ctx context.Context, sessionID string) (bool, error) {
	binding, err := m.repo.GetManagedAgentBindingBySession(ctx, sessionID)
	if errors.Is(err, repository.ErrManagedAgentBindingNotFound) {
		return m.lifecycleAdapter.ProbeAgentRunningForSession(ctx, sessionID)
	}
	if err != nil {
		return true, err
	}
	operation, err := m.repo.GetManagedAgentLatestOperation(ctx, binding.ID)
	if err != nil {
		return true, err
	}
	switch operation.State {
	case models.ManagedAgentSubmissionSucceeded, models.ManagedAgentSubmissionFailed,
		models.ManagedAgentSubmissionCancelled, models.ManagedAgentSubmissionRejected:
		return false, nil
	default:
		return true, nil
	}
}

func (m *cursorCloudAgentManager) IsAgentReadyForPrompt(ctx context.Context, sessionID string) bool {
	binding, err := m.repo.GetManagedAgentBindingBySession(ctx, sessionID)
	if errors.Is(err, repository.ErrManagedAgentBindingNotFound) {
		return m.lifecycleAdapter.IsAgentReadyForPrompt(ctx, sessionID)
	}
	if err != nil {
		return false
	}
	operation, err := m.repo.GetManagedAgentLatestOperation(ctx, binding.ID)
	return err == nil && (operation.State == models.ManagedAgentSubmissionSucceeded || operation.State == models.ManagedAgentSubmissionFailed ||
		operation.State == models.ManagedAgentSubmissionCancelled || operation.State == models.ManagedAgentSubmissionRejected)
}

func (m *cursorCloudAgentManager) SetExecutionDescription(ctx context.Context, executionID, description string) error {
	if _, err := m.repo.GetManagedAgentBindingByExecution(ctx, executionID); err == nil {
		return errors.New("cursor cloud prompt content is immutable after dispatch")
	} else if !errors.Is(err, repository.ErrManagedAgentBindingNotFound) {
		return err
	}
	return m.lifecycleAdapter.SetExecutionDescription(ctx, executionID, description)
}

func (m *cursorCloudAgentManager) SetExecutionEnv(ctx context.Context, executionID string, env map[string]string) error {
	if _, err := m.repo.GetManagedAgentBindingByExecution(ctx, executionID); err == nil {
		if len(env) == 0 {
			return nil
		}
		return errors.New("cursor cloud does not support environment forwarding")
	} else if !errors.Is(err, repository.ErrManagedAgentBindingNotFound) {
		return err
	}
	return m.lifecycleAdapter.SetExecutionEnv(ctx, executionID, env)
}

func (m *cursorCloudAgentManager) SetMcpMode(ctx context.Context, executionID, mode string) error {
	if _, err := m.repo.GetManagedAgentBindingByExecution(ctx, executionID); err == nil {
		return m.runtime.SetMcpMode(ctx, executionID, mode)
	} else if !errors.Is(err, repository.ErrManagedAgentBindingNotFound) {
		return err
	}
	return m.lifecycleAdapter.SetMcpMode(ctx, executionID, mode)
}

func (m *cursorCloudAgentManager) RestartAgentProcess(ctx context.Context, executionID string) error {
	if _, err := m.repo.GetManagedAgentBindingByExecution(ctx, executionID); err == nil {
		return runtime.ErrUnsupported
	} else if !errors.Is(err, repository.ErrManagedAgentBindingNotFound) {
		return err
	}
	return m.lifecycleAdapter.RestartAgentProcess(ctx, executionID)
}

func (m *cursorCloudAgentManager) ResetAgentContext(ctx context.Context, executionID string) error {
	if _, err := m.repo.GetManagedAgentBindingByExecution(ctx, executionID); err == nil {
		return runtime.ErrUnsupported
	} else if !errors.Is(err, repository.ErrManagedAgentBindingNotFound) {
		return err
	}
	return m.lifecycleAdapter.ResetAgentContext(ctx, executionID)
}

func (m *cursorCloudAgentManager) SetSessionModelBySessionID(ctx context.Context, sessionID, modelID string) error {
	if m.isCloudSession(ctx, sessionID) {
		return runtime.ErrUnsupported
	}
	return m.lifecycleAdapter.SetSessionModelBySessionID(ctx, sessionID, modelID)
}

func (m *cursorCloudAgentManager) SetSessionModeBySessionID(ctx context.Context, sessionID, modeID string) error {
	if m.isCloudSession(ctx, sessionID) {
		return runtime.ErrUnsupported
	}
	return m.lifecycleAdapter.SetSessionModeBySessionID(ctx, sessionID, modeID)
}

func (m *cursorCloudAgentManager) WasSessionInitialized(executionID string) bool {
	binding, err := m.repo.GetManagedAgentBindingByExecution(context.Background(), executionID)
	if errors.Is(err, repository.ErrManagedAgentBindingNotFound) {
		return m.lifecycleAdapter.WasSessionInitialized(executionID)
	}
	if err != nil {
		return false
	}
	operation, err := m.repo.GetManagedAgentOperationByPromptTurnID(context.Background(), cursorcloudruntime.InitialPromptTurnID(binding.SessionID))
	return err == nil && operation.RemoteRunID != "" && (operation.State == models.ManagedAgentSubmissionAccepted || operation.State == models.ManagedAgentSubmissionSucceeded)
}

func (m *cursorCloudAgentManager) GetSessionAuthMethods(sessionID string) []streams.AuthMethodInfo {
	if m.isCloudSession(context.Background(), sessionID) {
		return []streams.AuthMethodInfo{}
	}
	return m.lifecycleAdapter.GetSessionAuthMethods(sessionID)
}

func (m *cursorCloudAgentManager) IsPassthroughSession(ctx context.Context, sessionID string) bool {
	if m.isCloudSession(ctx, sessionID) {
		return false
	}
	return m.lifecycleAdapter.IsPassthroughSession(ctx, sessionID)
}

func (m *cursorCloudAgentManager) WritePassthroughStdin(ctx context.Context, sessionID, data string) error {
	if m.isCloudSession(ctx, sessionID) {
		return runtime.ErrUnsupported
	}
	return m.lifecycleAdapter.WritePassthroughStdin(ctx, sessionID, data)
}

func (m *cursorCloudAgentManager) ResolvePassthroughConfig(ctx context.Context, sessionID string) (agents.PassthroughConfig, error) {
	if m.isCloudSession(ctx, sessionID) {
		return agents.PassthroughConfig{}, runtime.ErrUnsupported
	}
	return m.lifecycleAdapter.ResolvePassthroughConfig(ctx, sessionID)
}

func (m *cursorCloudAgentManager) MarkPassthroughRunning(sessionID string) error {
	if m.isCloudSession(context.Background(), sessionID) {
		return runtime.ErrUnsupported
	}
	return m.lifecycleAdapter.MarkPassthroughRunning(sessionID)
}

func (m *cursorCloudAgentManager) GetRemoteRuntimeStatusBySession(ctx context.Context, sessionID string) (*executor.RemoteRuntimeStatus, error) {
	binding, err := m.repo.GetManagedAgentBindingBySession(ctx, sessionID)
	if errors.Is(err, repository.ErrManagedAgentBindingNotFound) {
		return m.lifecycleAdapter.GetRemoteRuntimeStatusBySession(ctx, sessionID)
	}
	if err != nil {
		return nil, err
	}
	operation, err := m.repo.GetManagedAgentLatestOperation(ctx, binding.ID)
	if err != nil {
		return nil, err
	}
	historyGap := false
	if operation.RemoteRunID != "" {
		checkpoint, checkpointErr := m.repo.GetManagedAgentStreamCheckpoint(ctx, binding.ID, operation.RemoteRunID)
		if checkpointErr != nil && !errors.Is(checkpointErr, repository.ErrManagedAgentStreamNotFound) {
			return nil, checkpointErr
		}
		historyGap = checkpoint != nil && checkpoint.HistoryGap
	}
	return &executor.RemoteRuntimeStatus{
		RuntimeName: agentruntime.RuntimeCursorCloud, RemoteName: "Cursor Cloud",
		State: string(operation.State), LastCheckedAt: operation.UpdatedAt, ErrorMessage: operation.SanitizedError,
		RepositoryID: operation.ResultSnapshot.RepositoryID, Branch: operation.ResultSnapshot.Branch,
		PullRequestURL: operation.ResultSnapshot.PullRequestURL, AgentURL: operation.ResultSnapshot.AgentURL,
		HistoryGap: historyGap,
	}, nil
}

func (m *cursorCloudAgentManager) ManagedAgentCompletionPending(ctx context.Context, operationID string) (bool, error) {
	operation, err := m.repo.GetManagedAgentOperation(ctx, operationID)
	if err != nil {
		return false, err
	}
	return operation.CompletionPending, nil
}

func (m *cursorCloudAgentManager) AcknowledgeManagedAgentCompletion(ctx context.Context, operationID string) error {
	return m.repo.AcknowledgeManagedAgentCompletion(ctx, operationID)
}

func (m *cursorCloudAgentManager) PollRemoteStatusForRecords(ctx context.Context, records []executor.RemoteStatusPollRequest) {
	filtered := make([]executor.RemoteStatusPollRequest, 0, len(records))
	for _, record := range records {
		if record.Runtime != agentruntime.RuntimeCursorCloud {
			filtered = append(filtered, record)
		}
	}
	m.lifecycleAdapter.PollRemoteStatusForRecords(ctx, filtered)
}

func (m *cursorCloudAgentManager) ProbeManagedRuntimeLiveness(ctx context.Context, executionID string) (executor.ManagedRuntimeLiveness, error) {
	state, err := m.managedRuntime.ProbeRemoteLiveness(ctx, executionID)
	return executor.ManagedRuntimeLiveness(state), err
}

func (m *cursorCloudAgentManager) CleanupStaleExecutionBySessionID(ctx context.Context, sessionID string) error {
	if m.isCloudSession(ctx, sessionID) {
		return nil
	}
	return m.lifecycleAdapter.CleanupStaleExecutionBySessionID(ctx, sessionID)
}

func (m *cursorCloudAgentManager) EnsureWorkspaceExecutionForSession(ctx context.Context, taskID, sessionID string) error {
	if m.isCloudSession(ctx, sessionID) {
		return runtime.ErrUnsupported
	}
	return m.lifecycleAdapter.EnsureWorkspaceExecutionForSession(ctx, taskID, sessionID)
}

func (m *cursorCloudAgentManager) GetExecutionIDForSession(ctx context.Context, sessionID string) (string, error) {
	binding, err := m.repo.GetManagedAgentBindingBySession(ctx, sessionID)
	if errors.Is(err, repository.ErrManagedAgentBindingNotFound) {
		return m.lifecycleAdapter.GetExecutionIDForSession(ctx, sessionID)
	}
	if err != nil {
		return "", err
	}
	return binding.ExecutionID, nil
}

func (m *cursorCloudAgentManager) ListExecutionsForTask(taskID string) []runtime.ExecutionReference {
	refs := m.lifecycleAdapter.ListExecutionsForTask(taskID)
	bySession := make(map[string]struct{}, len(refs))
	for _, ref := range refs {
		bySession[ref.SessionID] = struct{}{}
	}
	sessions, err := m.repo.ListTaskSessions(context.Background(), taskID)
	if err == nil {
		for _, session := range sessions {
			if session == nil {
				continue
			}
			binding, bindingErr := m.repo.GetManagedAgentBindingBySession(context.Background(), session.ID)
			if bindingErr == nil {
				if _, ok := bySession[session.ID]; !ok {
					refs = append(refs, runtime.ExecutionReference{SessionID: session.ID, ExecutionID: binding.ExecutionID})
					bySession[session.ID] = struct{}{}
				}
			}
		}
	}
	sort.Slice(refs, func(i, j int) bool { return refs[i].SessionID < refs[j].SessionID })
	return refs
}

// LiveSessionIDsForTask includes remote executions while they are active or
// uncertain. Task reconciliation must never treat a missing local process as
// proof that a managed Cursor run stopped.
func (m *cursorCloudAgentManager) LiveSessionIDsForTask(taskID string) []string {
	ids := make(map[string]struct{})
	if m.lifecycleAdapter != nil && m.mgr != nil {
		for _, sessionID := range m.lifecycleAdapter.LiveSessionIDsForTask(taskID) {
			if sessionID != "" {
				ids[sessionID] = struct{}{}
			}
		}
	}
	sessions, err := m.repo.ListTaskSessions(context.Background(), taskID)
	if err != nil {
		return sortedIDs(ids)
	}
	for _, session := range sessions {
		if session == nil || session.ID == "" {
			continue
		}
		binding, bindingErr := m.repo.GetManagedAgentBindingBySession(context.Background(), session.ID)
		if errors.Is(bindingErr, repository.ErrManagedAgentBindingNotFound) {
			continue
		}
		if bindingErr != nil {
			ids[session.ID] = struct{}{}
			continue
		}
		operation, operationErr := m.repo.GetManagedAgentLatestOperation(context.Background(), binding.ID)
		if operationErr != nil || operation == nil || !isTerminalManagedSubmission(operation.State) {
			ids[session.ID] = struct{}{}
		}
	}
	return sortedIDs(ids)
}

func isTerminalManagedSubmission(state models.ManagedAgentSubmissionState) bool {
	switch state {
	case models.ManagedAgentSubmissionSucceeded, models.ManagedAgentSubmissionFailed,
		models.ManagedAgentSubmissionCancelled, models.ManagedAgentSubmissionRejected:
		return true
	default:
		return false
	}
}

func sortedIDs(values map[string]struct{}) []string {
	ids := make([]string, 0, len(values))
	for id := range values {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func (m *cursorCloudAgentManager) GetGitLog(ctx context.Context, sessionID, baseCommit string, limit int, targetBranch string) (*runtime.GitLogResult, error) {
	if m.isCloudSession(ctx, sessionID) {
		return nil, runtime.ErrUnsupported
	}
	return m.lifecycleAdapter.GetGitLog(ctx, sessionID, baseCommit, limit, targetBranch)
}

func (m *cursorCloudAgentManager) GetCumulativeDiff(ctx context.Context, sessionID, baseCommit string) (*runtime.CumulativeDiffResult, error) {
	if m.isCloudSession(ctx, sessionID) {
		return nil, runtime.ErrUnsupported
	}
	return m.lifecycleAdapter.GetCumulativeDiff(ctx, sessionID, baseCommit)
}

func (m *cursorCloudAgentManager) GetGitStatus(ctx context.Context, sessionID string) (*runtime.GitStatusResult, error) {
	if m.isCloudSession(ctx, sessionID) {
		return nil, runtime.ErrUnsupported
	}
	return m.lifecycleAdapter.GetGitStatus(ctx, sessionID)
}

func (m *cursorCloudAgentManager) GetGitStatusFresh(ctx context.Context, sessionID string) (*runtime.GitStatusResult, error) {
	if m.isCloudSession(ctx, sessionID) {
		return nil, runtime.ErrUnsupported
	}
	return m.lifecycleAdapter.GetGitStatusFresh(ctx, sessionID)
}

func (m *cursorCloudAgentManager) WaitForAgentctlReady(ctx context.Context, sessionID string) error {
	if m.isCloudSession(ctx, sessionID) {
		return runtime.ErrUnsupported
	}
	return m.lifecycleAdapter.WaitForAgentctlReady(ctx, sessionID)
}

func (m *cursorCloudAgentManager) isCloudSession(ctx context.Context, sessionID string) bool {
	if sessionID == "" {
		return false
	}
	binding, err := m.repo.GetManagedAgentBindingBySession(ctx, sessionID)
	return err == nil && binding != nil
}

var _ executor.AgentManagerClient = (*cursorCloudAgentManager)(nil)
var _ executor.PromptTurnIDSetter = (*cursorCloudAgentManager)(nil)
var _ taskservice.SessionExecutionRegistry = (*cursorCloudAgentManager)(nil)
var _ = cursorCloudRequestDigest
