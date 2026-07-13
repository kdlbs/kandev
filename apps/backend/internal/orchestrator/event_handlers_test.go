package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/events"
	eventbus "github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/orchestrator/queue"
	"github.com/kandev/kandev/internal/orchestrator/scheduler"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
)

// --- Mocks ---

// mockStepGetter implements WorkflowStepGetter for testing.
type mockStepGetter struct {
	steps                  map[string]*wfmodels.WorkflowStep // stepID -> step
	workflowAgentProfileID string                            // returned by GetWorkflowAgentProfileID
}

func newMockStepGetter() *mockStepGetter {
	return &mockStepGetter{steps: make(map[string]*wfmodels.WorkflowStep)}
}

func (m *mockStepGetter) GetStep(_ context.Context, stepID string) (*wfmodels.WorkflowStep, error) {
	if s, ok := m.steps[stepID]; ok {
		return s, nil
	}
	return nil, nil
}

func (m *mockStepGetter) GetNextStepByPosition(_ context.Context, workflowID string, currentPosition int) (*wfmodels.WorkflowStep, error) {
	var best *wfmodels.WorkflowStep
	for _, s := range m.steps {
		if s.WorkflowID == workflowID && s.Position > currentPosition {
			if best == nil || s.Position < best.Position {
				best = s
			}
		}
	}
	return best, nil
}

func (m *mockStepGetter) GetPreviousStepByPosition(_ context.Context, workflowID string, currentPosition int) (*wfmodels.WorkflowStep, error) {
	var best *wfmodels.WorkflowStep
	for _, s := range m.steps {
		if s.WorkflowID == workflowID && s.Position < currentPosition {
			if best == nil || s.Position > best.Position {
				best = s
			}
		}
	}
	return best, nil
}

func (m *mockStepGetter) GetWorkflowAgentProfileID(_ context.Context, workflowID string) (string, error) {
	if m.workflowAgentProfileID != "" {
		return m.workflowAgentProfileID, nil
	}
	return "", nil
}

// mockTaskRepo implements scheduler.TaskRepository for testing.
type mockTaskRepo struct {
	mu            sync.Mutex
	tasks         map[string]*v1.Task
	updatedStates map[string]v1.TaskState
	stateWrites   map[string]int // per-task UpdateTaskState call count for dedup tests
	// stateHistory records every state actually written per task, in order.
	// updatedStates only keeps the latest value, which hides a transient
	// write (e.g. a REVIEW write later overwritten by an async prompt
	// dispatch's IN_PROGRESS write) — tests asserting a state was NEVER
	// written at any point must check stateHistory, not updatedStates.
	stateHistory map[string][]v1.TaskState
	// unconditionalWrites counts UpdateTaskState (the non-CAS write) calls per
	// task, tracked separately from stateWrites (which both UpdateTaskState
	// and UpdateTaskStateIfCurrentIn increment). Startup/crash reconciliation
	// callers must route guarded REVIEW writes through the CAS method so an
	// archive can't race a late write; tests assert this stays 0 for those
	// paths instead of just checking the resulting state.
	unconditionalWrites  map[string]int
	getTaskErr           error // if set, GetTask returns this error
	updateIfSessionState func(
		context.Context,
		string,
		string,
		models.TaskSessionState,
		v1.TaskState,
	) (bool, error)
}

func newMockTaskRepo() *mockTaskRepo {
	return &mockTaskRepo{
		tasks:               make(map[string]*v1.Task),
		updatedStates:       make(map[string]v1.TaskState),
		stateWrites:         make(map[string]int),
		stateHistory:        make(map[string][]v1.TaskState),
		unconditionalWrites: make(map[string]int),
	}
}

func seedMockTaskState(repo *mockTaskRepo, taskID string, state v1.TaskState) {
	repo.tasks[taskID] = &v1.Task{ID: taskID, State: state}
}

func (m *mockTaskRepo) GetTask(_ context.Context, taskID string) (*v1.Task, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.getTaskErr != nil {
		return nil, m.getTaskErr
	}
	if t, ok := m.tasks[taskID]; ok {
		return t, nil
	}
	return nil, nil
}

func (m *mockTaskRepo) UpdateTaskState(_ context.Context, taskID string, state v1.TaskState) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.updatedStates[taskID] = state
	m.stateWrites[taskID]++
	m.stateHistory[taskID] = append(m.stateHistory[taskID], state)
	m.unconditionalWrites[taskID]++
	if t, ok := m.tasks[taskID]; ok {
		t.State = state
	}
	return nil
}

func (m *mockTaskRepo) UpdateTaskStateIfCurrentIn(
	_ context.Context, taskID string, state v1.TaskState, allowed []v1.TaskState,
) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	t, ok := m.tasks[taskID]
	if !ok {
		return false, fmt.Errorf("%w: %s", sqliterepo.ErrTaskNotFound, taskID)
	}
	for _, candidate := range allowed {
		if t.State != candidate {
			continue
		}
		m.updatedStates[taskID] = state
		m.stateWrites[taskID]++
		m.stateHistory[taskID] = append(m.stateHistory[taskID], state)
		t.State = state
		return true, nil
	}
	return false, nil
}

// UpdateTaskStateIfNotArchived has no "allowed" precondition to check, so
// (unlike UpdateTaskStateIfCurrentIn above) this mock cannot model the
// archived_at race itself — v1.Task carries no ArchivedAt field for it to
// consult. It behaves like UpdateTaskState above: unconditional and
// always tracked, mutating the seeded task's State only when present —
// existing callers seed via seedMockTaskState only when they need to read
// the state back, not merely to assert a write happened.
// Real archived-freeze coverage for this CAS lives at the sqlite layer
// (task_state_cas_test.go) and the executor-layer mock (models.Task, which
// does carry ArchivedAt).
func (m *mockTaskRepo) UpdateTaskStateIfNotArchived(
	_ context.Context, taskID string, state v1.TaskState,
) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.updatedStates[taskID] = state
	m.stateWrites[taskID]++
	m.stateHistory[taskID] = append(m.stateHistory[taskID], state)
	if t, ok := m.tasks[taskID]; ok {
		t.State = state
	}
	return true, nil
}

func (m *mockTaskRepo) UpdateTaskStateIfSessionState(
	ctx context.Context,
	taskID, sessionID string,
	expectedSessionState models.TaskSessionState,
	state v1.TaskState,
) (bool, error) {
	if m.updateIfSessionState != nil {
		return m.updateIfSessionState(ctx, taskID, sessionID, expectedSessionState, state)
	}
	return m.UpdateTaskStateIfNotArchived(ctx, taskID, state)
}

// mockAgentManager is a minimal mock of executor.AgentManagerClient for testing.
type mockAgentManager struct {
	isPassthrough  bool
	isAgentRunning bool
	// isAgentRunningFn, when non-nil, overrides isAgentRunning for
	// IsAgentRunningForSession. Lets tests model state changes mid-sequence
	// (e.g. stream disconnect between PromptAgent call and queue write).
	isAgentRunningFn func(context.Context, string) bool
	isAgentReadyFn   func(context.Context, string) bool
	// rowLivenessFn, when non-nil, makes the mock satisfy the orchestrator's
	// optional rowLivenessProber so reconciliation tests can drive runtime-aware
	// liveness per row. Nil → the mock is not a prober and reconciliation treats
	// every row as Unknown.
	rowLivenessFn       func(*models.ExecutorRunning) models.ProcessLiveness
	resolveProfileInfo  *executor.AgentProfileInfo
	resolveProfileErr   error
	restartProcessCalls []string // tracks execution IDs passed to RestartAgentProcess
	restartProcessErr   error
	promptErr           error
	promptResult        *executor.PromptResult
	promptAgentFunc     func(context.Context, string, string, []v1.MessageAttachment, bool) (*executor.PromptResult, error)
	launchAgentFunc     func(context.Context, *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error)

	mu                      sync.Mutex
	stopAgentWithReasonArgs []stopAgentCall // tracks StopAgentWithReason calls
	stopAgentWithReasonErr  error           // optional error to return from StopAgentWithReason
	stopAgentWithReasonFunc func(context.Context, string, string, bool) error
	stopAgentArgs           []stopAgentCall // tracks StopAgent calls (no reason)
	stopAgentErr            error           // optional error to return from StopAgent

	// Prompt tracking — capturedPrompts records prompts only (legacy, several
	// tests assert on it directly). capturedPromptCalls records the same with
	// the execution ID so callers can filter by the agent that received it.
	capturedPrompts     []string
	capturedPromptCalls []promptCall
	// Optional: closed once on the first PromptAgent call so tests can wait
	// deterministically without polling. Tests opt in by initializing the channel.
	promptDone chan struct{}

	// Passthrough stdin tracking
	passthroughStdinCalls []passthroughStdinCall
	passthroughStdinErr   error
	markPassthroughCalls  []string // session IDs
	markPassthroughErr    error

	// Passthrough config resolution. When zero-valued and isPassthrough is true,
	// the mock returns a default config with SubmitSequence == "\r".
	passthroughConfig    agents.PassthroughConfig
	passthroughConfigSet bool
	passthroughConfigErr error

	// Optional override for GetExecutionIDForSession. When unset, the default
	// implementation reads from repoForExecutionLookup if provided so tests that
	// seed an executors_running row don't also have to mock this function.
	getExecutionIDForSessionFunc func(context.Context, string) (string, error)
	// Optional repo used as a fallback by GetExecutionIDForSession when no
	// override is provided — keeps tests that seed executors_running automatically
	// resolvable without per-test boilerplate.
	repoForExecutionLookup interface {
		GetExecutorRunningBySessionID(ctx context.Context, sessionID string) (*models.ExecutorRunning, error)
	}

	// CancelAgent tracking. cancelAgentCalls counts every invocation. If
	// cancelAgentBlock is non-nil, CancelAgent blocks on it before returning;
	// if cancelAgentEntered is non-nil, CancelAgent does a non-blocking send
	// on it on entry. Together they let tests stage concurrent cancel calls
	// without sleep-based polling.
	cancelAgentCalls   atomic.Int32
	cancelAgentBlock   chan struct{}
	cancelAgentEntered chan struct{}
	// cancelAgentErr, when set, is returned by CancelAgent instead of nil —
	// lets tests exercise callers that must react to a genuine cancel
	// failure (as opposed to the tolerated ErrNoExecutionForSession /
	// ErrCancelEscalated sentinels handled inside cancelAgentSilent).
	cancelAgentErr error

	currentPromptGeneration  atomic.Uint64
	currentPromptExecutionID string

	// set_session_mode tracking (issue #1183). Records (sessionID, modeID) for
	// every SetSessionModeBySessionID call. setSessionModeErr, when set, is
	// returned to simulate "no running agent".
	setSessionModeCalls      []sessionModeCall
	setSessionModeErr        error
	mcpModeCalls             []sessionModeCall
	setSessionModelCalls     []sessionModelCall
	setSessionModelSupported bool
	setSessionModelErr       error
}

type sessionModelCall struct {
	SessionID string
	ModelID   string
}

type sessionModeCall struct {
	SessionID string
	ModeID    string
}

type stopAgentCall struct {
	ExecutionID string
	Reason      string
	Force       bool
}

// promptCall records one PromptAgent invocation with its target execution ID.
type promptCall struct {
	ExecutionID  string
	Prompt       string
	DispatchOnly bool
}

type passthroughStdinCall struct {
	SessionID string
	Data      string
}

func (m *mockAgentManager) LaunchAgent(ctx context.Context, req *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
	if m.launchAgentFunc != nil {
		return m.launchAgentFunc(ctx, req)
	}
	return nil, nil
}
func (m *mockAgentManager) StartAgentProcess(_ context.Context, _ string) error { return nil }
func (m *mockAgentManager) IsAgentCommandConfigured(_ string) bool              { return true }
func (m *mockAgentManager) StopAgent(_ context.Context, agentExecutionID string, force bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stopAgentArgs = append(m.stopAgentArgs, stopAgentCall{ExecutionID: agentExecutionID, Force: force})
	return m.stopAgentErr
}
func (m *mockAgentManager) StopAgentWithReason(ctx context.Context, agentExecutionID, reason string, force bool) error {
	m.mu.Lock()
	m.stopAgentWithReasonArgs = append(m.stopAgentWithReasonArgs, stopAgentCall{
		ExecutionID: agentExecutionID,
		Reason:      reason,
		Force:       force,
	})
	hook := m.stopAgentWithReasonFunc
	err := m.stopAgentWithReasonErr
	m.mu.Unlock()
	if hook != nil {
		return hook(ctx, agentExecutionID, reason, force)
	}
	return err
}
func (m *mockAgentManager) PromptAgent(ctx context.Context, executionID string, prompt string, attachments []v1.MessageAttachment, dispatchOnly bool) (*executor.PromptResult, error) {
	m.mu.Lock()
	first := len(m.capturedPrompts) == 0
	m.capturedPrompts = append(m.capturedPrompts, prompt)
	m.capturedPromptCalls = append(m.capturedPromptCalls, promptCall{ExecutionID: executionID, Prompt: prompt, DispatchOnly: dispatchOnly})
	promptAgentFunc := m.promptAgentFunc
	promptErr := m.promptErr
	promptResult := m.promptResult
	doneCh := m.promptDone
	m.mu.Unlock()
	if first && doneCh != nil {
		close(doneCh)
	}
	if promptAgentFunc != nil {
		return promptAgentFunc(ctx, executionID, prompt, attachments, dispatchOnly)
	}
	if promptErr != nil {
		return nil, promptErr
	}
	if promptResult != nil {
		return promptResult, nil
	}
	return &executor.PromptResult{}, nil
}

func (m *mockAgentManager) PromptAgentWithDispatchCallback(ctx context.Context, executionID string, prompt string, attachments []v1.MessageAttachment, dispatchOnly bool, onDispatched func()) (*executor.PromptResult, error) {
	if onDispatched != nil {
		onDispatched()
	}
	return m.PromptAgent(ctx, executionID, prompt, attachments, dispatchOnly)
}
func (m *mockAgentManager) CancelAgent(_ context.Context, _ string) error {
	m.cancelAgentCalls.Add(1)
	if m.cancelAgentEntered != nil {
		select {
		case m.cancelAgentEntered <- struct{}{}:
		default:
		}
	}
	if m.cancelAgentBlock != nil {
		<-m.cancelAgentBlock
	}
	return m.cancelAgentErr
}
func (m *mockAgentManager) RespondToPermissionBySessionID(_ context.Context, _, _, _ string, _ bool) error {
	return nil
}
func (m *mockAgentManager) IsAgentRunningForSession(ctx context.Context, sessionID string) bool {
	if m.isAgentRunningFn != nil {
		return m.isAgentRunningFn(ctx, sessionID)
	}
	return m.isAgentRunning
}
func (m *mockAgentManager) IsAgentReadyForPrompt(ctx context.Context, sessionID string) bool {
	if m.isAgentReadyFn != nil {
		return m.isAgentReadyFn(ctx, sessionID)
	}
	return m.IsAgentRunningForSession(ctx, sessionID)
}

func (m *mockAgentManager) OwnsPromptGeneration(_ string, executionID string, generation uint64) bool {
	return executionID == m.currentPromptExecutionID && generation == m.currentPromptGeneration.Load()
}

// RowLiveness makes the mock satisfy the orchestrator's optional
// rowLivenessProber. It delegates to rowLivenessFn when set, else reports Unknown
// so tests that don't care about liveness see the safe default.
func (m *mockAgentManager) RowLiveness(row *models.ExecutorRunning) models.ProcessLiveness {
	if m.rowLivenessFn != nil {
		return m.rowLivenessFn(row)
	}
	return models.ProcessLivenessUnknown
}
func (m *mockAgentManager) ResolveAgentProfile(_ context.Context, _ string) (*executor.AgentProfileInfo, error) {
	if m.resolveProfileErr != nil {
		return nil, m.resolveProfileErr
	}
	if m.resolveProfileInfo != nil {
		return m.resolveProfileInfo, nil
	}
	return &executor.AgentProfileInfo{
		SupportsMCP:    true,
		CLIPassthrough: m.isPassthrough,
	}, nil
}
func (m *mockAgentManager) RestartAgentProcess(_ context.Context, agentExecutionID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.restartProcessCalls = append(m.restartProcessCalls, agentExecutionID)
	return m.restartProcessErr
}
func (m *mockAgentManager) ResetAgentContext(ctx context.Context, agentExecutionID string) error {
	return m.RestartAgentProcess(ctx, agentExecutionID)
}
func (m *mockAgentManager) SetExecutionDescription(_ context.Context, _, _ string) error {
	return nil
}
func (m *mockAgentManager) SetExecutionEnv(_ context.Context, _ string, _ map[string]string) error {
	return nil
}
func (m *mockAgentManager) SetSessionModelBySessionID(_ context.Context, sessionID, modelID string) error {
	if !m.setSessionModelSupported {
		return fmt.Errorf("not supported")
	}
	m.mu.Lock()
	m.setSessionModelCalls = append(m.setSessionModelCalls, sessionModelCall{SessionID: sessionID, ModelID: modelID})
	m.mu.Unlock()
	if m.setSessionModelErr != nil {
		return m.setSessionModelErr
	}
	return nil
}
func (m *mockAgentManager) SetSessionModeBySessionID(_ context.Context, sessionID, modeID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.setSessionModeCalls = append(m.setSessionModeCalls, sessionModeCall{SessionID: sessionID, ModeID: modeID})
	return m.setSessionModeErr
}

func (m *mockAgentManager) SetMcpMode(_ context.Context, executionID, mode string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.mcpModeCalls = append(m.mcpModeCalls, sessionModeCall{SessionID: executionID, ModeID: mode})
	return nil
}
func (m *mockAgentManager) WasSessionInitialized(_ string) bool { return false }
func (m *mockAgentManager) GetSessionAuthMethods(_ string) []streams.AuthMethodInfo {
	return nil
}
func (m *mockAgentManager) IsPassthroughSession(_ context.Context, _ string) bool {
	return m.isPassthrough
}
func (m *mockAgentManager) WritePassthroughStdin(_ context.Context, sessionID string, data string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.passthroughStdinCalls = append(m.passthroughStdinCalls, passthroughStdinCall{SessionID: sessionID, Data: data})
	return m.passthroughStdinErr
}
func (m *mockAgentManager) ResolvePassthroughConfig(_ context.Context, _ string) (agents.PassthroughConfig, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.passthroughConfigErr != nil {
		return agents.PassthroughConfig{}, m.passthroughConfigErr
	}
	if m.passthroughConfigSet {
		return m.passthroughConfig, nil
	}
	if !m.isPassthrough {
		return agents.PassthroughConfig{Supported: false}, nil
	}
	return agents.PassthroughConfig{Supported: true, SubmitSequence: "\r"}, nil
}
func (m *mockAgentManager) MarkPassthroughRunning(sessionID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.markPassthroughCalls = append(m.markPassthroughCalls, sessionID)
	return m.markPassthroughErr
}
func (m *mockAgentManager) GetRemoteRuntimeStatusBySession(_ context.Context, _ string) (*executor.RemoteRuntimeStatus, error) {
	return nil, nil
}
func (m *mockAgentManager) PollRemoteStatusForRecords(_ context.Context, _ []executor.RemoteStatusPollRequest) {
}
func (m *mockAgentManager) CleanupStaleExecutionBySessionID(_ context.Context, _ string) error {
	return nil
}
func (m *mockAgentManager) EnsureWorkspaceExecutionForSession(_ context.Context, _, _ string) error {
	return nil
}
func (m *mockAgentManager) GetExecutionIDForSession(ctx context.Context, sessionID string) (string, error) {
	if m.getExecutionIDForSessionFunc != nil {
		return m.getExecutionIDForSessionFunc(ctx, sessionID)
	}
	// Smart default: resolve from a seeded executors_running row when a repo
	// is provided. Removes per-test boilerplate when tests use seedExecutorRunning.
	if m.repoForExecutionLookup != nil {
		running, err := m.repoForExecutionLookup.GetExecutorRunningBySessionID(ctx, sessionID)
		if err == nil && running != nil && running.AgentExecutionID != "" {
			return running.AgentExecutionID, nil
		}
	}
	return "", fmt.Errorf("no execution found")
}
func (m *mockAgentManager) GetGitLog(_ context.Context, _, _ string, _ int, _ string) (*client.GitLogResult, error) {
	return nil, nil
}
func (m *mockAgentManager) GetCumulativeDiff(_ context.Context, _, _ string) (*client.CumulativeDiffResult, error) {
	return nil, nil
}
func (m *mockAgentManager) GetGitStatus(_ context.Context, _ string) (*client.GitStatusResult, error) {
	return &client.GitStatusResult{
		Success:    true,
		Branch:     "main",
		HeadCommit: "mock-commit",
	}, nil
}
func (m *mockAgentManager) GetGitStatusFresh(_ context.Context, _ string) (*client.GitStatusResult, error) {
	return nil, nil
}
func (m *mockAgentManager) WaitForAgentctlReady(_ context.Context, _ string) error {
	return nil
}

// --- Helpers ---

func testLogger() *logger.Logger {
	log, _ := logger.NewLogger(logger.LoggingConfig{
		Level:  "error",
		Format: "console",
	})
	return log
}

func strPtr(s string) *string { return &s }

// setupTestRepo creates a real in-memory SQLite repository for testing.
func setupTestRepo(t *testing.T) *sqliterepo.Repository {
	t.Helper()
	tmpDir := t.TempDir()
	dbConn, err := db.OpenSQLite(filepath.Join(tmpDir, "test.db"))
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}
	sqlxDB := sqlx.NewDb(dbConn, "sqlite3")
	t.Cleanup(func() { _ = sqlxDB.Close() })

	repo, cleanup, err := repository.Provide(sqlxDB, sqlxDB, nil)
	if err != nil {
		t.Fatalf("failed to create test repository: %v", err)
	}
	t.Cleanup(func() { _ = cleanup() })

	return repo
}

// seedSession creates a task, workspace, workflow and session in the repo for testing.
func seedSession(t *testing.T, repo *sqliterepo.Repository, taskID, sessionID, workflowStepID string) {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC()

	// Create workspace
	ws := &models.Workspace{ID: "ws1", Name: "Test", CreatedAt: now, UpdatedAt: now}
	if err := repo.CreateWorkspace(ctx, ws); err != nil {
		t.Fatalf("failed to create workspace: %v", err)
	}

	// Create workflow
	wf := &models.Workflow{ID: "wf1", WorkspaceID: "ws1", Name: "Test Workflow", CreatedAt: now, UpdatedAt: now}
	if err := repo.CreateWorkflow(ctx, wf); err != nil {
		// Might already exist
		_ = err
	}

	// Create task
	task := &models.Task{
		ID:             taskID,
		WorkflowID:     "wf1",
		WorkflowStepID: workflowStepID,
		Title:          "Test Task",
		Description:    "Test",
		State:          v1.TaskStateInProgress,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := repo.CreateTask(ctx, task); err != nil {
		t.Fatalf("failed to create task: %v", err)
	}

	// Create session
	session := &models.TaskSession{
		ID:        sessionID,
		TaskID:    taskID,
		State:     models.TaskSessionStateRunning,
		StartedAt: now,
		UpdatedAt: now,
	}
	if err := repo.CreateTaskSession(ctx, session); err != nil {
		t.Fatalf("failed to create task session: %v", err)
	}
}

type ownershipOverrideRepo struct {
	sessionExecutorStore
	tasks map[string]*models.Task
}

func (r ownershipOverrideRepo) GetTask(ctx context.Context, id string) (*models.Task, error) {
	if task, ok := r.tasks[id]; ok {
		return task, nil
	}
	return r.sessionExecutorStore.GetTask(ctx, id)
}

// seedExecutorRunning attaches an executors_running row to a session so the
// post-refactor "session has been launched" / GetExecutionIDForSession lookups
// resolve. Pre-refactor tests set session.AgentExecutionID directly; that field
// no longer drives runtime decisions, so any test that exercises the launched-
// session code paths must seed this row instead.
func seedExecutorRunning(t *testing.T, repo *sqliterepo.Repository, sessionID, taskID, executionID string) {
	t.Helper()
	if err := repo.UpsertExecutorRunning(context.Background(), &models.ExecutorRunning{
		ID:               sessionID,
		SessionID:        sessionID,
		TaskID:           taskID,
		AgentExecutionID: executionID,
		Status:           "ready",
	}); err != nil {
		t.Fatalf("seed executors_running: %v", err)
	}
}

// createTestService creates a Service with minimal dependencies for event handler testing.
func createTestService(repo *sqliterepo.Repository, stepGetter *mockStepGetter, taskRepo *mockTaskRepo) *Service {
	return createTestServiceWithAgent(repo, stepGetter, taskRepo, &mockAgentManager{})
}

func createTestServiceWithAgent(repo *sqliterepo.Repository, stepGetter *mockStepGetter, taskRepo *mockTaskRepo, agentMgr executor.AgentManagerClient) *Service {
	log := testLogger()
	// Wire the repo into the mockAgentManager's smart default for
	// GetExecutionIDForSession so tests that seed executors_running don't have
	// to also override the function. Skipped when the agent manager isn't a
	// mockAgentManager (custom implementations).
	if mock, ok := agentMgr.(*mockAgentManager); ok && mock.repoForExecutionLookup == nil {
		mock.repoForExecutionLookup = repo
	}
	return &Service{
		logger:             log,
		repo:               repo,
		workflowStepGetter: stepGetter,
		taskRepo:           taskRepo,
		agentManager:       agentMgr,
		messageQueue:       messagequeue.NewServiceMemory(log),
	}
}

// --- Tests ---

func TestWasResumeAttempt(t *testing.T) {
	ctx := context.Background()

	t.Run("returns true when resume token exists", func(t *testing.T) {
		repo := setupTestRepo(t)
		now := time.Now().UTC()
		ws := &models.Workspace{ID: "ws1", Name: "Test", CreatedAt: now, UpdatedAt: now}
		_ = repo.CreateWorkspace(ctx, ws)
		wf := &models.Workflow{ID: "wf1", WorkspaceID: "ws1", Name: "WF", CreatedAt: now, UpdatedAt: now}
		_ = repo.CreateWorkflow(ctx, wf)
		task := &models.Task{ID: "t1", WorkflowID: "wf1", Title: "T", State: v1.TaskStateInProgress, CreatedAt: now, UpdatedAt: now}
		_ = repo.CreateTask(ctx, task)
		session := &models.TaskSession{ID: "s1", TaskID: "t1", State: models.TaskSessionStateRunning, StartedAt: now, UpdatedAt: now}
		_ = repo.CreateTaskSession(ctx, session)
		_ = repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
			ID: "er1", SessionID: "s1", TaskID: "t1", ResumeToken: "acp-session-123",
			CreatedAt: now, UpdatedAt: now,
		})

		svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
		if !svc.wasResumeAttempt(ctx, "s1") {
			t.Error("expected wasResumeAttempt to return true when resume token exists")
		}
	})

	t.Run("returns false when no resume token", func(t *testing.T) {
		repo := setupTestRepo(t)
		now := time.Now().UTC()
		ws := &models.Workspace{ID: "ws1", Name: "Test", CreatedAt: now, UpdatedAt: now}
		_ = repo.CreateWorkspace(ctx, ws)
		wf := &models.Workflow{ID: "wf1", WorkspaceID: "ws1", Name: "WF", CreatedAt: now, UpdatedAt: now}
		_ = repo.CreateWorkflow(ctx, wf)
		task := &models.Task{ID: "t1", WorkflowID: "wf1", Title: "T", State: v1.TaskStateInProgress, CreatedAt: now, UpdatedAt: now}
		_ = repo.CreateTask(ctx, task)
		session := &models.TaskSession{ID: "s1", TaskID: "t1", State: models.TaskSessionStateRunning, StartedAt: now, UpdatedAt: now}
		_ = repo.CreateTaskSession(ctx, session)
		_ = repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
			ID: "er1", SessionID: "s1", TaskID: "t1", ResumeToken: "",
			CreatedAt: now, UpdatedAt: now,
		})

		svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
		if svc.wasResumeAttempt(ctx, "s1") {
			t.Error("expected wasResumeAttempt to return false when no resume token")
		}
	})

	t.Run("returns false when no executor running record", func(t *testing.T) {
		repo := setupTestRepo(t)
		svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
		if svc.wasResumeAttempt(ctx, "nonexistent-session") {
			t.Error("expected wasResumeAttempt to return false when no executor running record")
		}
	})
}

func TestHandleCompleteStreamEvent(t *testing.T) {
	ctx := context.Background()

	t.Run("does not force waiting when session is still running", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1")

		taskRepo := newMockTaskRepo()
		svc := createTestService(repo, newMockStepGetter(), taskRepo)

		payload := &lifecycle.AgentStreamEventPayload{
			TaskID:    "t1",
			SessionID: "s1",
			Data: &lifecycle.AgentStreamEventData{
				Type: agentEventComplete,
			},
		}

		svc.handleCompleteStreamEvent(ctx, payload)

		session, err := repo.GetTaskSession(ctx, "s1")
		if err != nil {
			t.Fatalf("failed to load session: %v", err)
		}
		if session.State != models.TaskSessionStateRunning {
			t.Fatalf("expected session to stay %q, got %q", models.TaskSessionStateRunning, session.State)
		}
		if _, ok := taskRepo.updatedStates["t1"]; ok {
			t.Fatalf("expected task state to remain unchanged, got update %q", taskRepo.updatedStates["t1"])
		}
	})
}

func TestHandleAgentReadyGuards(t *testing.T) {
	ctx := context.Background()

	t.Run("ignores ready when session is not running", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1")

		stepGetter := newMockStepGetter()
		stepGetter.steps["step1"] = &wfmodels.WorkflowStep{
			ID: "step1", WorkflowID: "wf1", Name: "Step 1", Position: 0,
			Events: wfmodels.StepEvents{
				OnTurnComplete: []wfmodels.OnTurnCompleteAction{
					{Type: wfmodels.OnTurnCompleteMoveToNext},
				},
			},
		}
		stepGetter.steps["step2"] = &wfmodels.WorkflowStep{
			ID: "step2", WorkflowID: "wf1", Name: "Step 2", Position: 1,
		}

		svc := createTestService(repo, stepGetter, newMockTaskRepo())
		session, _ := repo.GetTaskSession(ctx, "s1")
		session.State = models.TaskSessionStateWaitingForInput
		_ = repo.UpdateTaskSession(ctx, session)

		if _, err := svc.messageQueue.QueueMessage(ctx, "s1", "t1", "queued", "", "test", false, nil); err != nil {
			t.Fatalf("failed to queue message: %v", err)
		}

		svc.handleAgentReady(ctx, watcher.AgentEventData{TaskID: "t1", SessionID: "s1"})

		updatedTask, _ := repo.GetTask(ctx, "t1")
		if updatedTask.WorkflowStepID != "step1" {
			t.Fatalf("expected workflow step to remain step1, got %q", updatedTask.WorkflowStepID)
		}
		status := svc.messageQueue.GetStatus(ctx, "s1")
		if status.Count == 0 {
			t.Fatalf("expected queued message to remain queued")
		}
	})

	t.Run("waits for an in-flight cancel/interrupt then proceeds once it clears", func(t *testing.T) {
		// This is the "no transition" side of the parent-interrupt vs.
		// agent.ready race (see the reviewed PR #1653 discussion): once
		// handleAgentReady acquires the guard *before* any bookkeeping, a
		// concurrent holder no longer causes it to skip forever — it waits,
		// then (since nothing about this turn actually changed while it
		// waited) proceeds exactly as it would have uncontended, draining
		// the queued message. TestHandleAgentReadyGuards_ConcurrentInterruptRaces
		// below covers the cases where the guard holder *does* change
		// something (a redispatched successor turn) that handleAgentReady
		// must detect and back off from instead.
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1")

		stepGetter := newMockStepGetter()
		stepGetter.steps["step1"] = &wfmodels.WorkflowStep{
			ID: "step1", WorkflowID: "wf1", Name: "Step 1", Position: 0,
		}
		agentMgr := &mockAgentManager{isAgentRunning: true, repoForExecutionLookup: repo, promptDone: make(chan struct{})}
		svc := createTestServiceWithAgent(repo, stepGetter, newMockTaskRepo(), agentMgr)
		svc.executor = executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{})
		seedExecutorRunning(t, repo, "s1", "t1", "exec-1")

		if _, err := svc.messageQueue.QueueMessage(ctx, "s1", "t1", "queued", "", messagequeue.QueuedByUser, false, nil); err != nil {
			t.Fatalf("failed to queue message: %v", err)
		}

		lock, release := svc.acquireCancelInFlightGuard("s1")
		lock.Lock()

		readyDone := make(chan struct{})
		go func() {
			svc.handleAgentReady(ctx, watcher.AgentEventData{TaskID: "t1", SessionID: "s1"})
			close(readyDone)
		}()

		select {
		case <-readyDone:
			t.Fatal("handleAgentReady returned before the guard was released — it must block, not skip")
		case <-time.After(100 * time.Millisecond):
		}

		status := svc.messageQueue.GetStatus(ctx, "s1")
		if status.Count != 1 {
			t.Fatalf("expected queued message to remain parked while the guard is held, count=%d entries=%+v", status.Count, status.Entries)
		}

		lock.Unlock()
		release()

		select {
		case <-readyDone:
		case <-time.After(2 * time.Second):
			t.Fatal("timed out waiting for handleAgentReady to finish once the guard was released")
		}

		final := svc.messageQueue.GetStatus(ctx, "s1")
		if final.Count != 0 {
			t.Fatalf("expected handleAgentReady to drain the queued message once unblocked, count=%d entries=%+v", final.Count, final.Entries)
		}
		select {
		case <-agentMgr.promptDone:
		case <-time.After(2 * time.Second):
			t.Fatal("timed out waiting for the queued message to be dispatched")
		}
	})

	t.Run("moves STARTING session to waiting for input", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1")
		taskRepo := newMockTaskRepo()
		seedMockTaskState(taskRepo, "t1", v1.TaskStateInProgress)

		// Register the workflow step so processOnTurnComplete can resolve it.
		stepGetter := newMockStepGetter()
		stepGetter.steps["step1"] = &wfmodels.WorkflowStep{
			ID: "step1", WorkflowID: "wf1", Name: "Step 1", Position: 0,
		}
		svc := createTestService(repo, stepGetter, taskRepo)

		session, _ := repo.GetTaskSession(ctx, "s1")
		session.State = models.TaskSessionStateStarting
		_ = repo.UpdateTaskSession(ctx, session)

		svc.handleAgentReady(ctx, watcher.AgentEventData{TaskID: "t1", SessionID: "s1"})

		updated, _ := repo.GetTaskSession(ctx, "s1")
		if updated.State != models.TaskSessionStateWaitingForInput {
			t.Fatalf("expected session state %q, got %q", models.TaskSessionStateWaitingForInput, updated.State)
		}
		if state, ok := taskRepo.updatedStates["t1"]; !ok || state != v1.TaskStateReview {
			t.Fatalf("expected task state %q, got %q (ok=%v)", v1.TaskStateReview, state, ok)
		}
	})

	t.Run("ignores ready while reset is in progress", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1")

		stepGetter := newMockStepGetter()
		stepGetter.steps["step1"] = &wfmodels.WorkflowStep{
			ID: "step1", WorkflowID: "wf1", Name: "Step 1", Position: 0,
			Events: wfmodels.StepEvents{
				OnTurnComplete: []wfmodels.OnTurnCompleteAction{
					{Type: wfmodels.OnTurnCompleteMoveToNext},
				},
			},
		}
		stepGetter.steps["step2"] = &wfmodels.WorkflowStep{
			ID: "step2", WorkflowID: "wf1", Name: "Step 2", Position: 1,
		}

		svc := createTestService(repo, stepGetter, newMockTaskRepo())
		svc.setSessionResetInProgress("s1", true)
		defer svc.setSessionResetInProgress("s1", false)

		svc.handleAgentReady(ctx, watcher.AgentEventData{TaskID: "t1", SessionID: "s1"})

		updatedTask, _ := repo.GetTask(ctx, "t1")
		if updatedTask.WorkflowStepID != "step1" {
			t.Fatalf("expected workflow step to remain step1, got %q", updatedTask.WorkflowStepID)
		}
	})

	// "ignores stale ready from old execution" was removed: the early-drop branch
	// in handleAgentReady that compared session.AgentExecutionID with the event's
	// AgentExecutionID is gone. With executors_running as the single source of
	// truth and lifecycle-owned writes, a live event implies the emitting
	// execution is the active one for the session — there is no "old execution"
	// to filter out at this layer. See event_handlers_agent.go for the comment
	// explaining why the drop was removed and the lifecycle store invariant
	// that makes it unnecessary.
}

// turnSnapshotSyncTurnService wraps repoTurnService so tests can
// deterministically wait for handleAgentReady's pre-lock
// peekActiveTurnID(sessionID) call to have actually captured the
// *original* active turn before the test mutates it. Without this, the
// goroutine in raceGuardAgainstTurnReplacement below is scheduler-
// dependent: nothing guarantees handleAgentReady's first GetActiveTurn
// call has actually run (vs. merely been scheduled) before the test
// replaces the turn — a descheduled goroutine that captures the
// *replacement* turn as its own baseline would make the "must detect
// staleness" assertion pass for the wrong reason (no real race exercised)
// instead of failing when the guard's revalidation regresses.
// snapshotTaken closes once, on the first GetActiveTurn(sessionID) call
// (handleAgentReady's pre-lock snapshot is always its first turn lookup).
type turnSnapshotSyncTurnService struct {
	*repoTurnService
	sessionID     string
	snapshotTaken chan struct{}
	once          sync.Once
}

func (s *turnSnapshotSyncTurnService) GetActiveTurn(ctx context.Context, sessionID string) (*models.Turn, error) {
	turn, err := s.repoTurnService.GetActiveTurn(ctx, sessionID)
	if sessionID == s.sessionID {
		s.once.Do(func() { close(s.snapshotTaken) })
	}
	return turn, err
}

// TestHandleAgentReadyGuards_ConcurrentInterruptRaces extends the
// no-transition guard coverage above ("waits for an in-flight
// cancel/interrupt then proceeds once it clears") with the two races
// carlosflorencio's review explicitly called out as missing on PR #1653:
// an actual on_turn_complete transition, and a pending move, each racing a
// concurrent parent interrupt that cancels-and-redispatches the session
// *before* handleAgentReady's blocked guard acquisition returns.
//
// Each subtest manually replaces the active turn (completeTurnForSession +
// startTurnForSession) while holding the guard, mirroring exactly what a
// real QueueAndInterruptForPeerMessage call does internally (cancel the
// old turn, dispatch a new one) without needing to drive the full
// mockAgentManager cancel/prompt round trip — the property under test is
// specifically handleAgentReady's own post-guard revalidation, not the
// interrupt's own dispatch mechanics (covered separately by the
// TestQueueAndInterruptForPeerMessage_* suite in task_operations_test.go,
// including TestQueueAndInterruptForPeerMessage_DoesNotCancelUnrelatedSuccessorTurn
// which drives the same race through the public interrupt API instead).
func TestHandleAgentReadyGuards_ConcurrentInterruptRaces(t *testing.T) {
	ctx := context.Background()

	// raceGuardAgainstTurnReplacement claims sessionID's cancelInFlight
	// guard first (simulating an interrupt already in flight), starts
	// handleAgentReady in a goroutine, deterministically waits for it to
	// have snapshotted the *original* active turn (via
	// turnSnapshotSyncTurnService, not a sleep) before replacing the
	// active turn with a *different* one while still holding the guard
	// (simulating the interrupt's own cancel-and-redispatch), then
	// releases and waits for handleAgentReady to finish. Returns the turn
	// that replaced the original. svc.turnService must already be a
	// *turnSnapshotSyncTurnService for sessionID.
	raceGuardAgainstTurnReplacement := func(t *testing.T, svc *Service, sessionID string) *models.Turn {
		t.Helper()
		turnSync, ok := svc.turnService.(*turnSnapshotSyncTurnService)
		if !ok || turnSync.sessionID != sessionID {
			t.Fatalf("test setup bug: svc.turnService must be a *turnSnapshotSyncTurnService for %q", sessionID)
		}

		lock, release := svc.acquireCancelInFlightGuard(sessionID)
		lock.Lock()

		readyDone := make(chan struct{})
		go func() {
			svc.handleAgentReady(ctx, watcher.AgentEventData{TaskID: "t1", SessionID: sessionID})
			close(readyDone)
		}()

		select {
		case <-turnSync.snapshotTaken:
		case <-time.After(2 * time.Second):
			t.Fatal("timed out waiting for handleAgentReady to snapshot the original active turn")
		}
		select {
		case <-readyDone:
			t.Fatal("handleAgentReady returned before the guard was released — it must block on a concurrent interrupt")
		default:
		}

		svc.completeTurnForSession(ctx, sessionID)
		turnB, err := svc.turnService.StartTurn(ctx, sessionID)
		if err != nil {
			t.Fatalf("start replacement turn: %v", err)
		}

		lock.Unlock()
		release()

		select {
		case <-readyDone:
		case <-time.After(2 * time.Second):
			t.Fatal("timed out waiting for handleAgentReady to finish once the guard was released")
		}
		return turnB
	}

	t.Run("actual on_turn_complete transition: stale ready must not apply it", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1")

		stepGetter := newMockStepGetter()
		stepGetter.steps["step1"] = &wfmodels.WorkflowStep{
			ID: "step1", WorkflowID: "wf1", Name: "Step 1", Position: 0,
			Events: wfmodels.StepEvents{
				OnTurnComplete: []wfmodels.OnTurnCompleteAction{{Type: wfmodels.OnTurnCompleteMoveToNext}},
			},
		}
		stepGetter.steps["step2"] = &wfmodels.WorkflowStep{ID: "step2", WorkflowID: "wf1", Name: "Step 2", Position: 1}
		taskRepo := newMockTaskRepo()
		seedMockTaskState(taskRepo, "t1", v1.TaskStateInProgress)
		svc := createTestService(repo, stepGetter, taskRepo)
		svc.turnService = &turnSnapshotSyncTurnService{
			repoTurnService: &repoTurnService{repo: repo},
			sessionID:       "s1",
			snapshotTaken:   make(chan struct{}),
		}

		if _, err := svc.turnService.StartTurn(ctx, "s1"); err != nil {
			t.Fatalf("seed original turn: %v", err)
		}

		turnB := raceGuardAgainstTurnReplacement(t, svc, "s1")

		active, err := svc.turnService.GetActiveTurn(ctx, "s1")
		if err != nil {
			t.Fatalf("get active turn: %v", err)
		}
		if active == nil || active.ID != turnB.ID {
			t.Fatalf("expected the replacement turn %q to still be open, got %+v", turnB.ID, active)
		}

		updatedTask, err := repo.GetTask(ctx, "t1")
		if err != nil {
			t.Fatalf("get task: %v", err)
		}
		if updatedTask.WorkflowStepID != "step1" {
			t.Fatalf("a stale ready event must not apply the on_turn_complete transition for a turn it no longer owns; workflow step moved to %q", updatedTask.WorkflowStepID)
		}
	})

	t.Run("pending move: stale ready must leave it for the replacement turn", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1")

		stepGetter := newMockStepGetter()
		stepGetter.steps["step1"] = &wfmodels.WorkflowStep{ID: "step1", WorkflowID: "wf1", Name: "Step 1", Position: 0}
		stepGetter.steps["step2"] = &wfmodels.WorkflowStep{ID: "step2", WorkflowID: "wf1", Name: "Step 2", Position: 1}
		taskRepo := newMockTaskRepo()
		seedMockTaskState(taskRepo, "t1", v1.TaskStateInProgress)
		svc := createTestService(repo, stepGetter, taskRepo)
		svc.turnService = &turnSnapshotSyncTurnService{
			repoTurnService: &repoTurnService{repo: repo},
			sessionID:       "s1",
			snapshotTaken:   make(chan struct{}),
		}

		if _, err := svc.turnService.StartTurn(ctx, "s1"); err != nil {
			t.Fatalf("seed original turn: %v", err)
		}
		svc.messageQueue.SetPendingMove(ctx, "s1", &messagequeue.PendingMove{
			TaskID: "t1", WorkflowID: "wf1", WorkflowStepID: "step2",
		})

		turnB := raceGuardAgainstTurnReplacement(t, svc, "s1")

		active, err := svc.turnService.GetActiveTurn(ctx, "s1")
		if err != nil {
			t.Fatalf("get active turn: %v", err)
		}
		if active == nil || active.ID != turnB.ID {
			t.Fatalf("expected the replacement turn %q to still be open, got %+v", turnB.ID, active)
		}

		move, exists := svc.messageQueue.GetPendingMove(ctx, "s1")
		if !exists {
			t.Fatal("a stale ready event must not consume the pending move meant for the replacement turn")
		}
		if move.WorkflowStepID != "step2" {
			t.Fatalf("expected the pending move to still target step2, got %q", move.WorkflowStepID)
		}
	})
}

func TestHandleAgentReady_DelayedOldGenerationDoesNotCompleteReplacementTurn(t *testing.T) {
	ctx := context.Background()

	runScenario := func(t *testing.T, withPendingMove bool) {
		t.Helper()
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1")

		stepGetter := newMockStepGetter()
		stepGetter.steps["step1"] = &wfmodels.WorkflowStep{
			ID: "step1", WorkflowID: "wf1", Name: "Step 1", Position: 0,
		}
		taskRepo := newMockTaskRepo()
		seedMockTaskState(taskRepo, "t1", v1.TaskStateInProgress)
		agentMgr := &mockAgentManager{isAgentRunning: true}
		agentMgr.currentPromptExecutionID = "exec-1"
		agentMgr.currentPromptGeneration.Store(2)
		svc := createTestServiceWithAgent(repo, stepGetter, taskRepo, agentMgr)
		svc.turnService = &repoTurnService{repo: repo}

		oldTurn, err := svc.turnService.StartTurn(ctx, "s1")
		require.NoError(t, err)

		memoryBus := eventbus.NewMemoryEventBus(testLogger())
		t.Cleanup(memoryBus.Close)
		eventWatcher := watcher.NewWatcher(memoryBus, watcher.EventHandlers{
			OnAgentReady: svc.handleAgentReady,
		}, "stale-ready-regression", testLogger())
		require.NoError(t, eventWatcher.Start(ctx))
		t.Cleanup(func() { require.NoError(t, eventWatcher.Stop()) })

		oldReady := eventbus.NewEvent(events.AgentReady, "agent-manager", map[string]interface{}{
			"task_id":            "t1",
			"session_id":         "s1",
			"agent_execution_id": "exec-1",
			"prompt_generation":  1,
		})
		publishWaiting := make(chan struct{})
		releaseOldReady := make(chan struct{})
		publishDone := make(chan error, 1)
		go func() {
			close(publishWaiting)
			<-releaseOldReady
			publishDone <- memoryBus.Publish(ctx, events.AgentReady, oldReady)
		}()
		<-publishWaiting

		svc.completeTurnForSession(ctx, "s1")
		replacementTurn, err := svc.turnService.StartTurn(ctx, "s1")
		require.NoError(t, err)
		require.NotEqual(t, oldTurn.ID, replacementTurn.ID)
		if withPendingMove {
			svc.messageQueue.SetPendingMove(ctx, "s1", &messagequeue.PendingMove{
				TaskID: "t1", WorkflowID: "wf1", WorkflowStepID: "missing-step",
			})
		}

		close(releaseOldReady)
		select {
		case err := <-publishDone:
			require.NoError(t, err)
		case <-time.After(2 * time.Second):
			t.Fatal("timed out releasing the delayed old-generation ready event")
		}

		active, err := svc.turnService.GetActiveTurn(ctx, "s1")
		require.NoError(t, err)
		require.NotNil(t, active, "replacement turn must remain open")
		require.Equal(t, replacementTurn.ID, active.ID)

		updatedTask, err := repo.GetTask(ctx, "t1")
		require.NoError(t, err)
		require.Equal(t, "step1", updatedTask.WorkflowStepID,
			"old ready must not evaluate on_turn_complete for the replacement turn")
		updatedSession, err := repo.GetTaskSession(ctx, "s1")
		require.NoError(t, err)
		require.Equal(t, models.TaskSessionStateRunning, updatedSession.State,
			"old ready must not run replacement-turn completion bookkeeping")
		if withPendingMove {
			move, exists := svc.messageQueue.GetPendingMove(ctx, "s1")
			require.True(t, exists, "old ready must not consume the replacement turn's pending move")
			require.Equal(t, "missing-step", move.WorkflowStepID)
		}
	}

	t.Run("on_turn_complete remains unevaluated", func(t *testing.T) {
		runScenario(t, false)
	})
	t.Run("pending move remains untouched", func(t *testing.T) {
		runScenario(t, true)
	})
}

func TestExecuteQueuedMessage_RequeuesTransientPromptFailure(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")

	session, err := repo.GetTaskSession(ctx, "s1")
	if err != nil {
		t.Fatalf("failed to get session: %v", err)
	}
	session.State = models.TaskSessionStateWaitingForInput
	session.AgentExecutionID = "exec-1"
	seedExecutorRunning(t, repo, session.ID, session.TaskID, "exec-1")
	if err := repo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatalf("failed to update session: %v", err)
	}

	taskRepo := newMockTaskRepo()
	agentMgr := &mockAgentManager{
		isAgentRunning: true,
		promptErr:      errors.New("agent stream disconnected: read tcp [::1]:56463->[::1]:10002: use of closed network connection"),
	}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), taskRepo, agentMgr)
	svc.executor = executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{})

	queuedMsg := &messagequeue.QueuedMessage{
		ID:        "q1",
		SessionID: "s1",
		TaskID:    "t1",
		Content:   "hello",
		QueuedBy:  "test",
	}

	svc.markQueuedDispatchInFlight("s1", queuedMsg.ID)
	svc.executeQueuedMessage("s1", queuedMsg)

	status := svc.messageQueue.GetStatus(ctx, "s1")
	if status.Count != 1 {
		t.Fatalf("expected queued message to be requeued after transient failure, count=%d", status.Count)
	}
	if status.Entries[0].Content != "hello" {
		t.Fatalf("expected queued content to be preserved, got %q", status.Entries[0].Content)
	}
}

func TestExecuteQueuedMessage_RequeuesCoalescedMessageWithOriginalSender(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")

	session, err := repo.GetTaskSession(ctx, "s1")
	if err != nil {
		t.Fatalf("failed to get session: %v", err)
	}
	session.State = models.TaskSessionStateWaitingForInput
	session.AgentExecutionID = "exec-1"
	seedExecutorRunning(t, repo, session.ID, session.TaskID, "exec-1")
	if err := repo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatalf("failed to update session: %v", err)
	}

	taskRepo := newMockTaskRepo()
	agentMgr := &mockAgentManager{
		isAgentRunning: true,
		promptErr:      errors.New("agent stream disconnected: read tcp [::1]:56463->[::1]:10002: use of closed network connection"),
	}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), taskRepo, agentMgr)
	svc.executor = executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{})

	queuedMsg := &messagequeue.QueuedMessage{
		ID:        "q1",
		SessionID: "s1",
		TaskID:    "t1",
		Content:   "hello",
		Metadata:  map[string]interface{}{messagequeue.MetadataCoalesceKey: "ci-key"},
		QueuedBy:  messagequeue.QueuedByWorkflow,
	}
	if _, _, err := svc.messageQueue.QueueMessageWithCoalesceKey(ctx, "s1", "t1", "newer ci feedback", "", messagequeue.QueuedByWorkflow, false, nil, map[string]interface{}{messagequeue.MetadataCoalesceKey: "ci-key"}, "ci-key", true); err != nil {
		t.Fatalf("seed newer coalesced message: %v", err)
	}

	svc.markQueuedDispatchInFlight("s1", queuedMsg.ID)
	svc.executeQueuedMessage("s1", queuedMsg)

	status := svc.messageQueue.GetStatus(ctx, "s1")
	if status.Count != 1 {
		t.Fatalf("expected coalesced retry to replace existing pending entry, count=%d", status.Count)
	}
	if status.Entries[0].Content != "hello" {
		t.Fatalf("expected transient retry to remain coalesced, got %q", status.Entries[0].Content)
	}
	if status.Entries[0].QueuedBy != messagequeue.QueuedByWorkflow {
		t.Fatalf("expected original queued_by to be preserved, got %q", status.Entries[0].QueuedBy)
	}
	if status.Entries[0].Metadata[messagequeue.MetadataCoalesceKey] != "ci-key" {
		t.Fatalf("expected coalesce metadata to be preserved, got %+v", status.Entries[0].Metadata)
	}
}

func TestExecuteQueuedMessage_FiresOnTurnStart(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")

	session, err := repo.GetTaskSession(ctx, "s1")
	if err != nil {
		t.Fatalf("failed to get session: %v", err)
	}
	session.State = models.TaskSessionStateWaitingForInput
	session.AgentExecutionID = "exec-1"
	seedExecutorRunning(t, repo, session.ID, session.TaskID, "exec-1")
	if err := repo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatalf("failed to update session: %v", err)
	}

	stepGetter := newMockStepGetter()
	stepGetter.steps["step1"] = &wfmodels.WorkflowStep{
		ID: "step1", WorkflowID: "wf1", Name: "Step 1", Position: 0,
		Events: wfmodels.StepEvents{
			OnTurnStart: []wfmodels.OnTurnStartAction{
				{Type: wfmodels.OnTurnStartMoveToNext},
			},
		},
	}
	stepGetter.steps["step2"] = &wfmodels.WorkflowStep{
		ID: "step2", WorkflowID: "wf1", Name: "Step 2", Position: 1,
		Events: wfmodels.StepEvents{},
	}

	taskRepo := newMockTaskRepo()
	agentMgr := &mockAgentManager{
		isAgentRunning: true,
		// PromptAgent succeeds so the message is consumed normally.
	}
	log := testLogger()
	svc := &Service{
		logger:       log,
		repo:         repo,
		taskRepo:     taskRepo,
		agentManager: agentMgr,
		messageQueue: messagequeue.NewServiceMemory(log),
	}
	svc.SetWorkflowStepGetter(stepGetter)
	svc.executor = executor.NewExecutor(agentMgr, repo, log, executor.ExecutorConfig{})

	queuedMsg := &messagequeue.QueuedMessage{
		ID:        "q1",
		SessionID: "s1",
		TaskID:    "t1",
		Content:   "auto-start prompt",
		QueuedBy:  "workflow-auto-start",
	}

	svc.executeQueuedMessage("s1", queuedMsg)

	// Verify on_turn_start moved the task from step1 to step2.
	updatedTask, err := repo.GetTask(ctx, "t1")
	if err != nil {
		t.Fatalf("failed to get task: %v", err)
	}
	if updatedTask.WorkflowStepID != "step2" {
		t.Errorf("expected task workflow step to be 'step2', got %q", updatedTask.WorkflowStepID)
	}
}

func TestExecuteQueuedMessage_NoOnTurnStart_StepUnchanged(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")

	session, err := repo.GetTaskSession(ctx, "s1")
	if err != nil {
		t.Fatalf("failed to get session: %v", err)
	}
	session.State = models.TaskSessionStateWaitingForInput
	session.AgentExecutionID = "exec-1"
	seedExecutorRunning(t, repo, session.ID, session.TaskID, "exec-1")
	if err := repo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatalf("failed to update session: %v", err)
	}

	stepGetter := newMockStepGetter()
	stepGetter.steps["step1"] = &wfmodels.WorkflowStep{
		ID: "step1", WorkflowID: "wf1", Name: "Step 1", Position: 0,
		Events: wfmodels.StepEvents{}, // no on_turn_start
	}

	taskRepo := newMockTaskRepo()
	agentMgr := &mockAgentManager{isAgentRunning: true}
	log := testLogger()
	svc := &Service{
		logger:       log,
		repo:         repo,
		taskRepo:     taskRepo,
		agentManager: agentMgr,
		messageQueue: messagequeue.NewServiceMemory(log),
	}
	svc.SetWorkflowStepGetter(stepGetter)
	svc.executor = executor.NewExecutor(agentMgr, repo, log, executor.ExecutorConfig{})

	queuedMsg := &messagequeue.QueuedMessage{
		ID:        "q1",
		SessionID: "s1",
		TaskID:    "t1",
		Content:   "auto-start prompt",
		QueuedBy:  "workflow-auto-start",
	}

	svc.executeQueuedMessage("s1", queuedMsg)

	// Verify task stayed on step1 (no on_turn_start actions).
	updatedTask, err := repo.GetTask(ctx, "t1")
	if err != nil {
		t.Fatalf("failed to get task: %v", err)
	}
	if updatedTask.WorkflowStepID != "step1" {
		t.Errorf("expected task workflow step to remain 'step1', got %q", updatedTask.WorkflowStepID)
	}
}

func createTestServiceWithScheduler(repo *sqliterepo.Repository, stepGetter *mockStepGetter, taskRepo *mockTaskRepo, agentMgr executor.AgentManagerClient) *Service {
	log := testLogger()
	exec := executor.NewExecutor(agentMgr, repo, log, executor.ExecutorConfig{})
	sched := scheduler.NewScheduler(queue.NewTaskQueue(100), exec, taskRepo, log, scheduler.SchedulerConfig{})
	return &Service{
		logger:             log,
		repo:               repo,
		workflowStepGetter: stepGetter,
		taskRepo:           taskRepo,
		agentManager:       agentMgr,
		messageQueue:       messagequeue.NewServiceMemory(log),
		executor:           exec,
		scheduler:          sched,
	}
}

func TestHandleAgentCompleted_CleansUpExecution(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "")

	taskRepo := newMockTaskRepo()
	agentMgr := &mockAgentManager{repoForExecutionLookup: repo}
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), taskRepo, agentMgr)

	svc.handleAgentCompleted(ctx, watcher.AgentEventData{
		TaskID:           "t1",
		SessionID:        "s1",
		AgentExecutionID: "exec-1",
	})

	// cleanupAgentExecution runs in a goroutine; give it time to execute
	waitForStopCall(t, agentMgr)

	agentMgr.mu.Lock()
	defer agentMgr.mu.Unlock()
	call := agentMgr.stopAgentWithReasonArgs[0]
	if call.ExecutionID != "exec-1" {
		t.Errorf("expected execution ID %q, got %q", "exec-1", call.ExecutionID)
	}
	if !call.Force {
		t.Error("expected force=true for cleanup after completion")
	}
}

func TestHandleAgentCompleted_LegacyAutoStartDoesNotReenterCancellationGuard(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-legacy-auto-start", "session-legacy-auto-start", "step-1")
	seedExecutorRunning(t, repo, "session-legacy-auto-start", "task-legacy-auto-start", "execution-legacy-auto-start")
	stepGetter := newMockStepGetter()
	stepGetter.steps["step-1"] = &wfmodels.WorkflowStep{
		ID: "step-1", WorkflowID: "wf1", Name: "Work", Position: 0,
		Events: wfmodels.StepEvents{OnTurnComplete: []wfmodels.OnTurnCompleteAction{{
			Type: wfmodels.OnTurnCompleteMoveToNext,
		}}},
	}
	stepGetter.steps["step-2"] = &wfmodels.WorkflowStep{
		ID: "step-2", WorkflowID: "wf1", Name: "Continue", Position: 1,
		Events: wfmodels.StepEvents{OnEnter: []wfmodels.OnEnterAction{{
			Type: wfmodels.OnEnterAutoStartAgent,
		}}},
	}
	manager := &mockAgentManager{
		isAgentRunning:         true,
		repoForExecutionLookup: repo,
		promptDone:             make(chan struct{}),
	}
	taskRepo := newMockTaskRepo()
	seedMockTaskState(taskRepo, "task-legacy-auto-start", v1.TaskStateInProgress)
	svc := createTestServiceWithScheduler(repo, stepGetter, taskRepo, manager)
	// Explicitly pin the legacy path whose on_enter used to run inline and
	// deadlock by re-entering the completion handler's session guard.
	svc.workflowEngine = nil

	done := make(chan struct{})
	go func() {
		svc.handleAgentCompleted(ctx, watcher.AgentEventData{
			TaskID:           "task-legacy-auto-start",
			SessionID:        "session-legacy-auto-start",
			AgentExecutionID: "execution-legacy-auto-start",
		})
		close(done)
	}()
	coordinatorStopAwaitSignal(t, done, "legacy completion handler")
	select {
	case <-manager.promptDone:
	case <-time.After(2 * time.Second):
		t.Fatal("legacy on_enter auto-start did not acquire the released session guard")
	}
	task, err := repo.GetTask(ctx, "task-legacy-auto-start")
	require.NoError(t, err)
	require.Equal(t, "step-2", task.WorkflowStepID)
}

func TestHandleAgentCompleted_MarksRotatedExecutionCompleted(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "")
	seedExecutorRunning(t, repo, "s1", "t1", "exec-live")

	taskRepo := newMockTaskRepo()
	agentMgr := &mockAgentManager{repoForExecutionLookup: repo}
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), taskRepo, agentMgr)

	svc.handleAgentCompleted(ctx, watcher.AgentEventData{
		TaskID:           "t1",
		SessionID:        "s1",
		AgentExecutionID: "exec-old",
	})
	waitForStopCall(t, agentMgr)

	if !svc.isExecutionCompleted("s1", "exec-old") {
		t.Fatal("rotated agent.completed events must still block late tool events from the old execution")
	}
}

func TestHandleAgentCompleted_DoesNotMoveTaskToReviewWhileSiblingRuns(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s-finishing", "")
	seedExecutorRunning(t, repo, "s-finishing", "t1", "exec-finishing")

	now := time.Now().UTC()
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID:        "s-running",
		TaskID:    "t1",
		State:     models.TaskSessionStateRunning,
		StartedAt: now.Add(time.Second),
		UpdatedAt: now.Add(time.Second),
	}); err != nil {
		t.Fatalf("failed to create running sibling session: %v", err)
	}

	taskRepo := newMockTaskRepo()
	agentMgr := &mockAgentManager{repoForExecutionLookup: repo}
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), taskRepo, agentMgr)

	svc.handleAgentCompleted(ctx, watcher.AgentEventData{
		TaskID:           "t1",
		SessionID:        "s-finishing",
		AgentExecutionID: "exec-finishing",
	})
	waitForStopCall(t, agentMgr)

	if got := taskRepo.stateWrites["t1"]; got != 0 {
		t.Fatalf("agent completion must not move the task to REVIEW while another session is running, writes=%d state=%q",
			got, taskRepo.updatedStates["t1"])
	}

	updatedFinishing, err := repo.GetTaskSession(ctx, "s-finishing")
	if err != nil {
		t.Fatalf("failed to load finishing session: %v", err)
	}
	if updatedFinishing.State != models.TaskSessionStateWaitingForInput {
		t.Fatalf("expected finishing session to leave RUNNING, got %q", updatedFinishing.State)
	}
}

func TestHandleAgentCompleted_MovesTaskToReviewWhenLastSiblingFinishes(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s-first", "step1")
	seedExecutorRunning(t, repo, "s-first", "t1", "exec-first")

	now := time.Now().UTC()
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID:        "s-last",
		TaskID:    "t1",
		State:     models.TaskSessionStateRunning,
		StartedAt: now.Add(time.Second),
		UpdatedAt: now.Add(time.Second),
	}); err != nil {
		t.Fatalf("failed to create running sibling session: %v", err)
	}
	seedExecutorRunning(t, repo, "s-last", "t1", "exec-last")

	taskRepo := newMockTaskRepo()
	seedMockTaskState(taskRepo, "t1", v1.TaskStateInProgress)
	agentMgr := &mockAgentManager{repoForExecutionLookup: repo}
	stepGetter := newMockStepGetter()
	stepGetter.steps["step1"] = &wfmodels.WorkflowStep{
		ID:         "step1",
		WorkflowID: "wf1",
		Name:       "Step 1",
		Position:   0,
	}
	svc := createTestServiceWithScheduler(repo, stepGetter, taskRepo, agentMgr)

	svc.handleAgentCompleted(ctx, watcher.AgentEventData{
		TaskID:           "t1",
		SessionID:        "s-first",
		AgentExecutionID: "exec-first",
	})

	if got := taskRepo.stateWrites["t1"]; got != 0 {
		t.Fatalf("first completion must not move task to REVIEW while sibling runs, writes=%d state=%q",
			got, taskRepo.updatedStates["t1"])
	}

	svc.handleAgentCompleted(ctx, watcher.AgentEventData{
		TaskID:           "t1",
		SessionID:        "s-last",
		AgentExecutionID: "exec-last",
	})

	if got := taskRepo.stateWrites["t1"]; got != 1 {
		t.Fatalf("last sibling completion must move task to REVIEW once, writes=%d state=%q",
			got, taskRepo.updatedStates["t1"])
	}
	if state := taskRepo.updatedStates["t1"]; state != v1.TaskStateReview {
		t.Fatalf("expected task state %q, got %q", v1.TaskStateReview, state)
	}
}

func TestHandleAgentCompleted_NoWorkflowStepLastSiblingMovesTaskToReview(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s-first", "")
	seedExecutorRunning(t, repo, "s-first", "t1", "exec-first")

	now := time.Now().UTC()
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID:        "s-last",
		TaskID:    "t1",
		State:     models.TaskSessionStateRunning,
		StartedAt: now.Add(time.Second),
		UpdatedAt: now.Add(time.Second),
	}); err != nil {
		t.Fatalf("failed to create running sibling session: %v", err)
	}
	seedExecutorRunning(t, repo, "s-last", "t1", "exec-last")

	taskRepo := newMockTaskRepo()
	seedMockTaskState(taskRepo, "t1", v1.TaskStateInProgress)
	agentMgr := &mockAgentManager{repoForExecutionLookup: repo}
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), taskRepo, agentMgr)

	svc.handleAgentCompleted(ctx, watcher.AgentEventData{
		TaskID:           "t1",
		SessionID:        "s-first",
		AgentExecutionID: "exec-first",
	})

	if got := taskRepo.stateWrites["t1"]; got != 0 {
		t.Fatalf("first no-step completion must not move task to REVIEW while sibling runs, writes=%d state=%q",
			got, taskRepo.updatedStates["t1"])
	}
	updatedFirst, err := repo.GetTaskSession(ctx, "s-first")
	if err != nil {
		t.Fatalf("failed to load first session: %v", err)
	}
	if updatedFirst.State != models.TaskSessionStateWaitingForInput {
		t.Fatalf("expected first session state %q, got %q", models.TaskSessionStateWaitingForInput, updatedFirst.State)
	}

	svc.handleAgentCompleted(ctx, watcher.AgentEventData{
		TaskID:           "t1",
		SessionID:        "s-last",
		AgentExecutionID: "exec-last",
	})

	if got := taskRepo.stateWrites["t1"]; got != 1 {
		t.Fatalf("last no-step sibling completion must move task to REVIEW once, writes=%d state=%q",
			got, taskRepo.updatedStates["t1"])
	}
	updatedFirst, err = repo.GetTaskSession(ctx, "s-first")
	if err != nil {
		t.Fatalf("failed to reload first session: %v", err)
	}
	if updatedFirst.State != models.TaskSessionStateWaitingForInput {
		t.Fatalf("expected first session to remain %q, got %q", models.TaskSessionStateWaitingForInput, updatedFirst.State)
	}
	updatedLast, err := repo.GetTaskSession(ctx, "s-last")
	if err != nil {
		t.Fatalf("failed to load last session: %v", err)
	}
	if updatedLast.State != models.TaskSessionStateWaitingForInput {
		t.Fatalf("expected last session state %q, got %q", models.TaskSessionStateWaitingForInput, updatedLast.State)
	}
}

func TestHandleAgentCompleted_ExitOnlyCompletesActiveTurn(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "task1", "session1", "")
	seedExecutorRunning(t, repo, "session1", "task1", "exec-1")

	taskRepo := newMockTaskRepo()
	agentMgr := &mockAgentManager{repoForExecutionLookup: repo}
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), taskRepo, agentMgr)
	svc.turnService = &repoTurnService{repo: repo}

	if _, err := svc.turnService.StartTurn(ctx, "session1"); err != nil {
		t.Fatalf("failed to seed active turn: %v", err)
	}
	if open := openTurnCount(t, repo, "session1"); open != 1 {
		t.Fatalf("expected 1 open turn before completion, got %d", open)
	}

	svc.handleAgentCompleted(ctx, watcher.AgentEventData{
		TaskID:           "task1",
		SessionID:        "session1",
		AgentExecutionID: "exec-1",
	})
	waitForStopCall(t, agentMgr)

	if open := openTurnCount(t, repo, "session1"); open != 0 {
		t.Fatalf("expected agent.completed to close the active turn, got %d open turns", open)
	}
	updated, err := repo.GetTaskSession(ctx, "session1")
	if err != nil {
		t.Fatalf("failed to load session: %v", err)
	}
	if updated.State != models.TaskSessionStateWaitingForInput {
		t.Fatalf("expected session state %q, got %q", models.TaskSessionStateWaitingForInput, updated.State)
	}
}

func TestHandleAgentFailedWithoutSession_DoesNotMoveTaskToReviewWhileSessionRuns(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s-running", "")

	taskRepo := newMockTaskRepo()
	agentMgr := &mockAgentManager{repoForExecutionLookup: repo}
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), taskRepo, agentMgr)

	svc.handleAgentFailed(ctx, watcher.AgentEventData{
		TaskID:       "t1",
		ErrorMessage: "agent failed before session binding",
	})

	if got := taskRepo.stateWrites["t1"]; got != 0 {
		t.Fatalf("sessionless failure must not move the task to REVIEW while a session is running, writes=%d state=%q",
			got, taskRepo.updatedStates["t1"])
	}
}

func TestHandleAgentFailed_CleansUpExecution(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "")

	taskRepo := newMockTaskRepo()
	agentMgr := &mockAgentManager{repoForExecutionLookup: repo}
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), taskRepo, agentMgr)

	svc.handleAgentFailed(ctx, watcher.AgentEventData{
		TaskID:           "t1",
		SessionID:        "s1",
		AgentExecutionID: "exec-1",
		ErrorMessage:     "agent crashed",
	})

	// cleanupAgentExecution runs in a goroutine; give it time to execute
	waitForStopCall(t, agentMgr)

	agentMgr.mu.Lock()
	defer agentMgr.mu.Unlock()
	call := agentMgr.stopAgentWithReasonArgs[0]
	if call.ExecutionID != "exec-1" {
		t.Errorf("expected execution ID %q, got %q", "exec-1", call.ExecutionID)
	}
}

func TestCleanupAgentExecution_SkipsEmptyExecutionID(t *testing.T) {
	repo := setupTestRepo(t)
	agentMgr := &mockAgentManager{repoForExecutionLookup: repo}
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)

	// Should return immediately without calling StopAgentWithReason
	svc.cleanupAgentExecution("", "t1", "s1")

	agentMgr.mu.Lock()
	defer agentMgr.mu.Unlock()
	if len(agentMgr.stopAgentWithReasonArgs) != 0 {
		t.Error("expected no StopAgentWithReason call for empty execution ID")
	}
}

func TestHandleAgentRunning_PassthroughGuard(t *testing.T) {
	ctx := context.Background()

	t.Run("ACP session skips on_turn_start", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1")

		stepGetter := newMockStepGetter()
		stepGetter.steps["step1"] = &wfmodels.WorkflowStep{
			ID: "step1", WorkflowID: "wf1", Name: "Step 1", Position: 0,
			Events: wfmodels.StepEvents{
				OnTurnStart: []wfmodels.OnTurnStartAction{
					{Type: wfmodels.OnTurnStartMoveToNext},
				},
			},
		}
		stepGetter.steps["step2"] = &wfmodels.WorkflowStep{
			ID: "step2", WorkflowID: "wf1", Name: "Step 2", Position: 1,
		}

		taskRepo := newMockTaskRepo()
		agentMgr := &mockAgentManager{isPassthrough: false}
		svc := createTestServiceWithAgent(repo, stepGetter, taskRepo, agentMgr)

		svc.handleAgentRunning(ctx, watcher.AgentEventData{TaskID: "t1", SessionID: "s1"})

		// Workflow step must remain step1 because on_turn_start is skipped for ACP sessions.
		updatedTask, err := repo.GetTask(ctx, "t1")
		if err != nil {
			t.Fatalf("failed to get task: %v", err)
		}
		if updatedTask.WorkflowStepID != "step1" {
			t.Errorf("expected task workflow step to remain 'step1', got %q", updatedTask.WorkflowStepID)
		}
	})

	t.Run("passthrough session fires on_turn_start", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1")

		stepGetter := newMockStepGetter()
		stepGetter.steps["step1"] = &wfmodels.WorkflowStep{
			ID: "step1", WorkflowID: "wf1", Name: "Step 1", Position: 0,
			Events: wfmodels.StepEvents{
				OnTurnStart: []wfmodels.OnTurnStartAction{
					{Type: wfmodels.OnTurnStartMoveToNext},
				},
			},
		}
		stepGetter.steps["step2"] = &wfmodels.WorkflowStep{
			ID: "step2", WorkflowID: "wf1", Name: "Step 2", Position: 1,
		}

		taskRepo := newMockTaskRepo()
		agentMgr := &mockAgentManager{isPassthrough: true}
		svc := createTestServiceWithAgent(repo, stepGetter, taskRepo, agentMgr)

		svc.handleAgentRunning(ctx, watcher.AgentEventData{TaskID: "t1", SessionID: "s1"})

		// Workflow step must move to step2 because passthrough sessions fire on_turn_start.
		updatedTask, err := repo.GetTask(ctx, "t1")
		if err != nil {
			t.Fatalf("failed to get task: %v", err)
		}
		if updatedTask.WorkflowStepID != "step2" {
			t.Errorf("expected task workflow step to be 'step2', got %q", updatedTask.WorkflowStepID)
		}
	})

	t.Run("missing session_id is ignored", func(t *testing.T) {
		repo := setupTestRepo(t)
		taskRepo := newMockTaskRepo()
		svc := createTestService(repo, newMockStepGetter(), taskRepo)

		// Should not panic or error with empty session ID.
		svc.handleAgentRunning(ctx, watcher.AgentEventData{TaskID: "t1", SessionID: ""})
	})

	t.Run("coordinator cancellation wins while passthrough event waits", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1")
		stepGetter := newMockStepGetter()
		stepGetter.steps["step1"] = &wfmodels.WorkflowStep{
			ID: "step1", WorkflowID: "wf1", Name: "Step 1", Position: 0,
			Events: wfmodels.StepEvents{OnTurnStart: []wfmodels.OnTurnStartAction{{
				Type: wfmodels.OnTurnStartMoveToNext,
			}}},
		}
		stepGetter.steps["step2"] = &wfmodels.WorkflowStep{
			ID: "step2", WorkflowID: "wf1", Name: "Step 2", Position: 1,
		}
		svc := createTestServiceWithAgent(
			repo,
			stepGetter,
			newMockTaskRepo(),
			&mockAgentManager{isPassthrough: true},
		)

		guard, release := svc.acquireCancelInFlightGuard("s1")
		guard.Lock()
		done := make(chan struct{})
		go func() {
			svc.handleAgentRunning(ctx, watcher.AgentEventData{TaskID: "t1", SessionID: "s1"})
			close(done)
		}()
		coordinatorStopWaitForGuardRefs(t, svc, "s1", 2)
		changed, _, err := repo.CancelActiveTaskSession(ctx, "s1", coordinatorMCPStopReason)
		require.NoError(t, err)
		require.True(t, changed)
		guard.Unlock()
		release()
		coordinatorStopAwaitSignal(t, done, "guarded passthrough running event")

		session, err := repo.GetTaskSession(ctx, "s1")
		require.NoError(t, err)
		require.Equal(t, models.TaskSessionStateCancelled, session.State)
		task, err := repo.GetTask(ctx, "t1")
		require.NoError(t, err)
		require.Equal(t, "step1", task.WorkflowStepID)
	})
}

// TestHandleAgentRunning_DoesNotWakeWaitingAcpSession is the regression guard for
// the silent-resume flicker (session-resume-keeps-review-state e2e). On resume,
// the agent.running boot event fires for the reconnecting ACP session; it must
// NOT wake a settled WAITING_FOR_INPUT session to RUNNING (which would displace
// the task into the Running sidebar bucket). ACP turns drive RUNNING via the
// prompt path instead. Passthrough sessions, which have no prompt path, must
// still wake on agent.running.
func TestHandleAgentRunning_DoesNotWakeWaitingAcpSession(t *testing.T) {
	ctx := context.Background()

	settleWaiting := func(t *testing.T, repo *sqliterepo.Repository) {
		t.Helper()
		if err := repo.UpdateTaskSessionState(ctx, "s1", models.TaskSessionStateWaitingForInput, ""); err != nil {
			t.Fatalf("failed to settle session to WAITING_FOR_INPUT: %v", err)
		}
	}

	t.Run("ACP boot does not wake session", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1")
		settleWaiting(t, repo)

		svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(),
			&mockAgentManager{isPassthrough: false})
		svc.handleAgentRunning(ctx, watcher.AgentEventData{TaskID: "t1", SessionID: "s1"})

		sess, err := repo.GetTaskSession(ctx, "s1")
		if err != nil {
			t.Fatalf("failed to get session: %v", err)
		}
		if sess.State != models.TaskSessionStateWaitingForInput {
			t.Errorf("ACP agent.running boot must keep session WAITING_FOR_INPUT, got %q", sess.State)
		}
	})

	t.Run("passthrough turn wakes session", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1")
		settleWaiting(t, repo)

		svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(),
			&mockAgentManager{isPassthrough: true})
		svc.handleAgentRunning(ctx, watcher.AgentEventData{TaskID: "t1", SessionID: "s1"})

		sess, err := repo.GetTaskSession(ctx, "s1")
		if err != nil {
			t.Fatalf("failed to get session: %v", err)
		}
		if sess.State != models.TaskSessionStateRunning {
			t.Errorf("passthrough agent.running must wake session to RUNNING, got %q", sess.State)
		}
	})
}

func TestDeliverPassthroughPrompt(t *testing.T) {
	ctx := context.Background()

	t.Run("writes to stdin and marks running", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1")

		agentMgr := &mockAgentManager{isPassthrough: true}
		svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)

		err := svc.deliverPassthroughPrompt(ctx, "s1", "hello")
		if err != nil {
			t.Fatalf("deliverPassthroughPrompt returned error: %v", err)
		}

		agentMgr.mu.Lock()
		defer agentMgr.mu.Unlock()

		if len(agentMgr.passthroughStdinCalls) != 1 {
			t.Fatalf("expected 1 stdin call, got %d", len(agentMgr.passthroughStdinCalls))
		}
		call := agentMgr.passthroughStdinCalls[0]
		if call.SessionID != "s1" {
			t.Errorf("stdin sessionID = %q, want %q", call.SessionID, "s1")
		}
		if call.Data != "hello\r" {
			t.Errorf("stdin data = %q, want %q", call.Data, "hello\r")
		}
		if len(agentMgr.markPassthroughCalls) != 1 || agentMgr.markPassthroughCalls[0] != "s1" {
			t.Errorf("markPassthroughRunning calls = %v, want [s1]", agentMgr.markPassthroughCalls)
		}
	})

	t.Run("returns error when stdin write fails", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1")

		agentMgr := &mockAgentManager{
			isPassthrough:       true,
			passthroughStdinErr: fmt.Errorf("stdin write failed"),
		}
		svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)

		err := svc.deliverPassthroughPrompt(ctx, "s1", "hello")
		if err == nil {
			t.Fatal("expected error, got nil")
		}

		agentMgr.mu.Lock()
		defer agentMgr.mu.Unlock()

		// markPassthroughRunning now fires BEFORE any writes so concurrent
		// PromptTask calls are blocked during the inter-chunk SubmitDelay
		// window (Greptile P1). Expect exactly one mark call even when the
		// subsequent write fails.
		if len(agentMgr.markPassthroughCalls) != 1 {
			t.Errorf("markPassthroughRunning should fire once before the write; got %d calls", len(agentMgr.markPassthroughCalls))
		}
	})
}

func TestHandleAgentReady_PassthroughQueuedMessage(t *testing.T) {
	ctx := context.Background()

	t.Run("delivers queued message to passthrough via stdin", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1")

		// Set session to RUNNING so handleAgentReady doesn't early-return
		session, _ := repo.GetTaskSession(ctx, "s1")
		session.State = models.TaskSessionStateRunning
		_ = repo.UpdateTaskSession(ctx, session)

		stepGetter := newMockStepGetter()
		stepGetter.steps["step1"] = &wfmodels.WorkflowStep{
			ID: "step1", WorkflowID: "wf1", Name: "Step 1", Position: 0,
		}

		agentMgr := &mockAgentManager{isPassthrough: true}
		svc := createTestServiceWithAgent(repo, stepGetter, newMockTaskRepo(), agentMgr)

		// Queue a message
		if _, err := svc.messageQueue.QueueMessage(ctx, "s1", "t1", "queued prompt", "", "test", false, nil); err != nil {
			t.Fatalf("failed to queue message: %v", err)
		}

		svc.handleAgentReady(ctx, watcher.AgentEventData{TaskID: "t1", SessionID: "s1"})

		agentMgr.mu.Lock()
		defer agentMgr.mu.Unlock()

		// Verify the queued message was delivered to passthrough stdin
		if len(agentMgr.passthroughStdinCalls) != 1 {
			t.Fatalf("expected 1 stdin call, got %d", len(agentMgr.passthroughStdinCalls))
		}
		call := agentMgr.passthroughStdinCalls[0]
		if call.Data != "queued prompt\r" {
			t.Errorf("stdin data = %q, want %q", call.Data, "queued prompt\r")
		}

		// Queue should be empty after delivery
		status := svc.messageQueue.GetStatus(ctx, "s1")
		if status.Count != 0 {
			t.Errorf("expected queue to be empty after delivery, count=%d", status.Count)
		}
	})

	t.Run("skips delivery when no queued message exists", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1")

		session, _ := repo.GetTaskSession(ctx, "s1")
		session.State = models.TaskSessionStateRunning
		_ = repo.UpdateTaskSession(ctx, session)

		stepGetter := newMockStepGetter()
		stepGetter.steps["step1"] = &wfmodels.WorkflowStep{
			ID: "step1", WorkflowID: "wf1", Name: "Step 1", Position: 0,
		}

		agentMgr := &mockAgentManager{isPassthrough: true}
		svc := createTestServiceWithAgent(repo, stepGetter, newMockTaskRepo(), agentMgr)

		// No queued message — should return early
		svc.handleAgentReady(ctx, watcher.AgentEventData{TaskID: "t1", SessionID: "s1"})

		agentMgr.mu.Lock()
		defer agentMgr.mu.Unlock()

		if len(agentMgr.passthroughStdinCalls) != 0 {
			t.Errorf("expected no stdin calls, got %d", len(agentMgr.passthroughStdinCalls))
		}
	})
}

func TestAutoStartPassthroughPrompt(t *testing.T) {
	ctx := context.Background()

	t.Run("writes prompt to stdin and logs step name", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1")

		agentMgr := &mockAgentManager{isPassthrough: true}
		svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)

		session, _ := repo.GetTaskSession(ctx, "s1")
		err := svc.autoStartPassthroughPrompt(ctx, "t1", session, "Analyze", "do analysis")
		if err != nil {
			t.Fatalf("autoStartPassthroughPrompt returned error: %v", err)
		}

		agentMgr.mu.Lock()
		defer agentMgr.mu.Unlock()

		if len(agentMgr.passthroughStdinCalls) != 1 {
			t.Fatalf("expected 1 stdin call, got %d", len(agentMgr.passthroughStdinCalls))
		}
		if agentMgr.passthroughStdinCalls[0].Data != "do analysis\r" {
			t.Errorf("stdin data = %q, want %q", agentMgr.passthroughStdinCalls[0].Data, "do analysis\r")
		}
	})

	t.Run("returns error when stdin write fails", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1")

		agentMgr := &mockAgentManager{
			isPassthrough:       true,
			passthroughStdinErr: fmt.Errorf("stdin write failed"),
		}
		svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)

		session, _ := repo.GetTaskSession(ctx, "s1")
		err := svc.autoStartPassthroughPrompt(ctx, "t1", session, "Analyze", "do analysis")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})
}

func TestClearResumeToken(t *testing.T) {
	ctx := context.Background()

	t.Run("clears existing resume token", func(t *testing.T) {
		repo := setupTestRepo(t)
		now := time.Now().UTC()
		ws := &models.Workspace{ID: "ws1", Name: "Test", CreatedAt: now, UpdatedAt: now}
		_ = repo.CreateWorkspace(ctx, ws)
		wf := &models.Workflow{ID: "wf1", WorkspaceID: "ws1", Name: "WF", CreatedAt: now, UpdatedAt: now}
		_ = repo.CreateWorkflow(ctx, wf)
		task := &models.Task{ID: "t1", WorkflowID: "wf1", Title: "T", State: v1.TaskStateInProgress, CreatedAt: now, UpdatedAt: now}
		_ = repo.CreateTask(ctx, task)
		session := &models.TaskSession{ID: "s1", TaskID: "t1", State: models.TaskSessionStateRunning, StartedAt: now, UpdatedAt: now}
		_ = repo.CreateTaskSession(ctx, session)
		_ = repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
			ID: "er1", SessionID: "s1", TaskID: "t1", ResumeToken: "acp-session-123",
			CreatedAt: now, UpdatedAt: now,
		})

		svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
		svc.clearResumeToken(ctx, "s1")

		running, err := repo.GetExecutorRunningBySessionID(ctx, "s1")
		if err != nil {
			t.Fatalf("failed to get executor running: %v", err)
		}
		if running.ResumeToken != "" {
			t.Errorf("expected empty resume token, got %q", running.ResumeToken)
		}
	})

	t.Run("no-op when no executor running record", func(t *testing.T) {
		repo := setupTestRepo(t)
		svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
		// Should not panic
		svc.clearResumeToken(ctx, "nonexistent-session")
	})

	t.Run("no-op when token already empty", func(t *testing.T) {
		repo := setupTestRepo(t)
		now := time.Now().UTC()
		ws := &models.Workspace{ID: "ws1", Name: "Test", CreatedAt: now, UpdatedAt: now}
		_ = repo.CreateWorkspace(ctx, ws)
		wf := &models.Workflow{ID: "wf1", WorkspaceID: "ws1", Name: "WF", CreatedAt: now, UpdatedAt: now}
		_ = repo.CreateWorkflow(ctx, wf)
		task := &models.Task{ID: "t1", WorkflowID: "wf1", Title: "T", State: v1.TaskStateInProgress, CreatedAt: now, UpdatedAt: now}
		_ = repo.CreateTask(ctx, task)
		session := &models.TaskSession{ID: "s1", TaskID: "t1", State: models.TaskSessionStateRunning, StartedAt: now, UpdatedAt: now}
		_ = repo.CreateTaskSession(ctx, session)
		_ = repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
			ID: "er1", SessionID: "s1", TaskID: "t1", ResumeToken: "",
			CreatedAt: now, UpdatedAt: now,
		})

		svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
		// Should not panic or error
		svc.clearResumeToken(ctx, "s1")
	})
}

func TestHandleRecoverableFailure(t *testing.T) {
	ctx := context.Background()

	t.Run("sets session to WAITING_FOR_INPUT with error message", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1")

		taskRepo := newMockTaskRepo()
		agentMgr := &mockAgentManager{repoForExecutionLookup: repo}
		svc := createTestServiceWithScheduler(repo, newMockStepGetter(), taskRepo, agentMgr)

		svc.handleRecoverableFailure(ctx, watcher.AgentEventData{
			TaskID:           "t1",
			SessionID:        "s1",
			AgentExecutionID: "exec-1",
			ErrorMessage:     "agent crashed unexpectedly",
		})

		session, err := repo.GetTaskSession(ctx, "s1")
		if err != nil {
			t.Fatalf("failed to get session: %v", err)
		}
		if session.State != models.TaskSessionStateWaitingForInput {
			t.Errorf("expected session state %q, got %q", models.TaskSessionStateWaitingForInput, session.State)
		}
		if session.ErrorMessage != "agent crashed unexpectedly" {
			t.Errorf("expected error message %q, got %q", "agent crashed unexpectedly", session.ErrorMessage)
		}
	})

	t.Run("sets task to REVIEW state", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1")

		taskRepo := newMockTaskRepo()
		seedMockTaskState(taskRepo, "t1", v1.TaskStateInProgress)
		agentMgr := &mockAgentManager{repoForExecutionLookup: repo}
		svc := createTestServiceWithScheduler(repo, newMockStepGetter(), taskRepo, agentMgr)

		svc.handleRecoverableFailure(ctx, watcher.AgentEventData{
			TaskID:           "t1",
			SessionID:        "s1",
			AgentExecutionID: "exec-1",
			ErrorMessage:     "agent crashed",
		})

		if state, ok := taskRepo.updatedStates["t1"]; !ok || state != v1.TaskStateReview {
			t.Errorf("expected task state %q, got %q (ok=%v)", v1.TaskStateReview, state, ok)
		}
	})

	t.Run("cleans up agent execution", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1")

		taskRepo := newMockTaskRepo()
		agentMgr := &mockAgentManager{repoForExecutionLookup: repo}
		svc := createTestServiceWithScheduler(repo, newMockStepGetter(), taskRepo, agentMgr)

		svc.handleRecoverableFailure(ctx, watcher.AgentEventData{
			TaskID:           "t1",
			SessionID:        "s1",
			AgentExecutionID: "exec-1",
			ErrorMessage:     "agent crashed",
		})

		waitForStopCall(t, agentMgr)

		agentMgr.mu.Lock()
		defer agentMgr.mu.Unlock()
		if len(agentMgr.stopAgentWithReasonArgs) == 0 {
			t.Error("expected cleanup to call StopAgentWithReason")
		}
	})
}

func TestIsOfficeSessionUsesCanonicalTaskOwnership(t *testing.T) {
	tests := []struct {
		name         string
		isFromOffice bool
		sessionAgent string
		want         bool
	}{
		{name: "unassigned Office task", isFromOffice: true, want: true},
		{name: "assigned Kanban session", sessionAgent: "assigned-agent", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			repo := setupTestRepo(t)
			seedSession(t, repo, "t1", "s1", "")
			session, err := repo.GetTaskSession(ctx, "s1")
			if err != nil {
				t.Fatalf("get session: %v", err)
			}
			session.AgentProfileID = tt.sessionAgent
			if err := repo.UpdateTaskSession(ctx, session); err != nil {
				t.Fatalf("update session: %v", err)
			}

			svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
			svc.repo = ownershipOverrideRepo{
				sessionExecutorStore: repo,
				tasks: map[string]*models.Task{"t1": {
					ID: "t1", IsFromOffice: tt.isFromOffice,
				}},
			}
			if got := svc.isOfficeSession(ctx, "s1"); got != tt.want {
				t.Fatalf("isOfficeSession = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHandleAgentStartFailed(t *testing.T) {
	ctx := context.Background()

	t.Run("non-auth resume failure sets suppressToast and returns false", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1")
		svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())

		handled := svc.handleAgentStartFailed(ctx, "t1", "s1", "exec-1",
			fmt.Errorf("ACP initialize handshake failed"), true)

		if handled {
			t.Error("expected handled=false so default FAILED transition runs")
		}
		v, ok := svc.suppressToast.Load("s1")
		if !ok || v.(bool) != true {
			t.Errorf("expected suppressToast[s1]=true, got ok=%v val=%v", ok, v)
		}
	})

	t.Run("non-auth fresh-start failure does not set suppressToast", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1")
		svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())

		handled := svc.handleAgentStartFailed(ctx, "t1", "s1", "exec-1",
			fmt.Errorf("ACP initialize handshake failed"), false)

		if handled {
			t.Error("expected handled=false")
		}
		if _, ok := svc.suppressToast.Load("s1"); ok {
			t.Error("expected suppressToast NOT set for fresh-start failure")
		}
	})

	t.Run("auth error returns true regardless of fromResume", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1")
		taskRepo := newMockTaskRepo()
		agentMgr := &mockAgentManager{repoForExecutionLookup: repo}
		svc := createTestServiceWithScheduler(repo, newMockStepGetter(), taskRepo, agentMgr)

		handled := svc.handleAgentStartFailed(ctx, "t1", "s1", "exec-1",
			fmt.Errorf("authentication required: please log in"), true)

		if !handled {
			t.Error("expected handled=true for auth errors")
		}
	})

	t.Run("session read failure leaves strict executor cleanup in charge", func(t *testing.T) {
		baseRepo := setupTestRepo(t)
		seedSession(t, baseRepo, "t1", "s1", "step1")
		repo := &coordinatorStopRepoHooks{
			repoStore: baseRepo,
			getSessionFunc: func(context.Context, string) (*models.TaskSession, error) {
				return nil, errors.New("temporary session read failure")
			},
		}
		svc := createTestService(baseRepo, newMockStepGetter(), newMockTaskRepo())
		svc.repo = repo

		handled := svc.handleAgentStartFailed(
			ctx,
			"t1",
			"s1",
			"exec-1",
			errors.New("ACP initialize handshake failed"),
			false,
		)

		if handled {
			t.Error("expected handled=false so executor transition and forced cleanup still run")
		}
	})

	t.Run("cancelled session enqueues exact cleanup arbitration", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1")
		require.NoError(t, repo.UpdateTaskSessionState(
			ctx,
			"s1",
			models.TaskSessionStateCancelled,
			"cancelled outside coordinator stop",
		))
		agentMgr := &mockAgentManager{repoForExecutionLookup: repo}
		svc := createTestServiceWithScheduler(
			repo,
			newMockStepGetter(),
			newMockTaskRepo(),
			agentMgr,
		)

		handled := svc.handleAgentStartFailed(
			ctx,
			"t1",
			"s1",
			"exec-unclaimed",
			errors.New("ACP initialize handshake failed"),
			false,
		)

		require.False(t, handled)
		waitForStopCall(t, agentMgr)
		agentMgr.mu.Lock()
		calls := append([]stopAgentCall(nil), agentMgr.stopAgentWithReasonArgs...)
		agentMgr.mu.Unlock()
		require.Len(t, calls, 1)
		require.Equal(t, "exec-unclaimed", calls[0].ExecutionID)
		require.True(t, calls[0].Force)
	})
}

func TestHandleResumeFailure(t *testing.T) {
	ctx := context.Background()

	t.Run("clears resume token and sets WAITING_FOR_INPUT", func(t *testing.T) {
		repo := setupTestRepo(t)
		now := time.Now().UTC()
		seedSession(t, repo, "t1", "s1", "step1")

		// Add executor running with resume token
		_ = repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
			ID: "er1", SessionID: "s1", TaskID: "t1", ResumeToken: "acp-session-old",
			CreatedAt: now, UpdatedAt: now,
		})

		taskRepo := newMockTaskRepo()
		seedMockTaskState(taskRepo, "t1", v1.TaskStateInProgress)
		svc := createTestService(repo, newMockStepGetter(), taskRepo)

		result := svc.handleResumeFailure(ctx, watcher.AgentEventData{
			TaskID:           "t1",
			SessionID:        "s1",
			AgentExecutionID: "exec-1",
			ErrorMessage:     "resume failed: session expired",
		})

		if !result {
			t.Error("expected handleResumeFailure to return true")
		}

		// Verify resume token was cleared
		running, err := repo.GetExecutorRunningBySessionID(ctx, "s1")
		if err != nil {
			t.Fatalf("failed to get executor running: %v", err)
		}
		if running.ResumeToken != "" {
			t.Errorf("expected empty resume token, got %q", running.ResumeToken)
		}

		// Verify session state
		session, err := repo.GetTaskSession(ctx, "s1")
		if err != nil {
			t.Fatalf("failed to get session: %v", err)
		}
		if session.State != models.TaskSessionStateWaitingForInput {
			t.Errorf("expected session state %q, got %q", models.TaskSessionStateWaitingForInput, session.State)
		}

		// Verify task moved to REVIEW
		if state, ok := taskRepo.updatedStates["t1"]; !ok || state != v1.TaskStateReview {
			t.Errorf("expected task state %q, got %q (ok=%v)", v1.TaskStateReview, state, ok)
		}
	})
}

func TestHandleAgentFailed_RecoverableWithSession(t *testing.T) {
	ctx := context.Background()

	t.Run("routes to recoverable failure when session exists", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1")

		taskRepo := newMockTaskRepo()
		agentMgr := &mockAgentManager{repoForExecutionLookup: repo}
		svc := createTestServiceWithScheduler(repo, newMockStepGetter(), taskRepo, agentMgr)

		svc.handleAgentFailed(ctx, watcher.AgentEventData{
			TaskID:           "t1",
			SessionID:        "s1",
			AgentExecutionID: "exec-1",
			ErrorMessage:     "agent process exited",
		})

		// Should set session to WAITING_FOR_INPUT (recoverable), not FAILED
		session, err := repo.GetTaskSession(ctx, "s1")
		if err != nil {
			t.Fatalf("failed to get session: %v", err)
		}
		if session.State != models.TaskSessionStateWaitingForInput {
			t.Errorf("expected recoverable state %q, got %q", models.TaskSessionStateWaitingForInput, session.State)
		}
		lastErrRaw := session.Metadata[models.SessionMetaKeyLastAgentError]
		lastErr, ok := lastErrRaw.(map[string]interface{})
		if !ok {
			t.Fatalf("expected last agent error metadata map, got %#v", lastErrRaw)
		}
		if got := lastErr["message"]; got != "agent process exited" {
			t.Fatalf("expected last agent error message to persist, got %#v", got)
		}
	})

	t.Run("routes to resume failure when resume token exists and init not completed", func(t *testing.T) {
		repo := setupTestRepo(t)
		now := time.Now().UTC()
		seedSession(t, repo, "t1", "s1", "step1")

		_ = repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
			ID: "er1", SessionID: "s1", TaskID: "t1", ResumeToken: "acp-session-old",
			CreatedAt: now, UpdatedAt: now,
		})

		taskRepo := newMockTaskRepo()
		agentMgr := &mockAgentManager{repoForExecutionLookup: repo} // WasSessionInitialized returns false by default
		svc := createTestServiceWithScheduler(repo, newMockStepGetter(), taskRepo, agentMgr)

		svc.handleAgentFailed(ctx, watcher.AgentEventData{
			TaskID:           "t1",
			SessionID:        "s1",
			AgentExecutionID: "exec-1",
			ErrorMessage:     "resume failed",
		})

		// Resume token should be cleared
		running, err := repo.GetExecutorRunningBySessionID(ctx, "s1")
		if err != nil {
			t.Fatalf("failed to get executor running: %v", err)
		}
		if running.ResumeToken != "" {
			t.Errorf("expected resume token to be cleared, got %q", running.ResumeToken)
		}

		// Session should be WAITING_FOR_INPUT
		session, err := repo.GetTaskSession(ctx, "s1")
		if err != nil {
			t.Fatalf("failed to get session: %v", err)
		}
		if session.State != models.TaskSessionStateWaitingForInput {
			t.Errorf("expected session state %q, got %q", models.TaskSessionStateWaitingForInput, session.State)
		}
	})
}

func TestHandleAgentStopped_PreservesRecoveryState(t *testing.T) {
	ctx := context.Background()

	t.Run("does not clobber WAITING_FOR_INPUT state", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1")

		// Set session to WAITING_FOR_INPUT (recovery state)
		session, _ := repo.GetTaskSession(ctx, "s1")
		session.State = models.TaskSessionStateWaitingForInput
		_ = repo.UpdateTaskSession(ctx, session)

		svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())

		svc.handleAgentStopped(ctx, watcher.AgentEventData{
			TaskID:           "t1",
			SessionID:        "s1",
			AgentExecutionID: "exec-1",
		})

		updated, _ := repo.GetTaskSession(ctx, "s1")
		if updated.State != models.TaskSessionStateWaitingForInput {
			t.Errorf("expected state to remain %q, got %q", models.TaskSessionStateWaitingForInput, updated.State)
		}
	})

	t.Run("sets CANCELLED when session is not in recovery", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1")

		svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())

		svc.handleAgentStopped(ctx, watcher.AgentEventData{
			TaskID:           "t1",
			SessionID:        "s1",
			AgentExecutionID: "exec-1",
		})

		updated, _ := repo.GetTaskSession(ctx, "s1")
		if updated.State != models.TaskSessionStateCancelled {
			t.Errorf("expected state %q, got %q", models.TaskSessionStateCancelled, updated.State)
		}
	})

	// Office fire-and-forget regression: when the office turn-complete
	// handler sets the session to IDLE before stopping the agent, the
	// resulting agent.stopped event must NOT clobber IDLE → CANCELLED.
	// Without this guard, the next office run's EnsureSessionForAgent
	// sees a terminal session, tries to INSERT a new row, and the partial
	// unique index on (task_id, agent_profile_id) rejects it. Comments
	// silently fail to wake the agent.
	t.Run("does not clobber IDLE state (office fire-and-forget)", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1")

		session, _ := repo.GetTaskSession(ctx, "s1")
		session.State = models.TaskSessionStateIdle
		_ = repo.UpdateTaskSession(ctx, session)

		svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())

		svc.handleAgentStopped(ctx, watcher.AgentEventData{
			TaskID:           "t1",
			SessionID:        "s1",
			AgentExecutionID: "exec-1",
		})

		updated, _ := repo.GetTaskSession(ctx, "s1")
		if updated.State != models.TaskSessionStateIdle {
			t.Errorf("expected state to remain %q, got %q (would break next office run)",
				models.TaskSessionStateIdle, updated.State)
		}
	})

	// Rotated-execution regression: a previous cycle's agent.stopped event must
	// not flip the session to CANCELLED when a fresh resume cycle has already
	// taken over (executors_running.agent_execution_id rotated). Without this
	// guard, an intermittent ACP notification-queue overflow on cycle 1
	// produces a late stopped event whose state mutation poisons the in-
	// progress cycle 2 — the user sees the task transition to FAILED even
	// though cycle 2 loaded the session cleanly. See discussion in the cycle
	// 1/2/3 analysis (task a263caf5).
	t.Run("drops stopped events from rotated executions", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1")

		// Make sure session is in RUNNING — the state we'd clobber if the
		// guard didn't fire.
		session, _ := repo.GetTaskSession(ctx, "s1")
		session.State = models.TaskSessionStateRunning
		_ = repo.UpdateTaskSession(ctx, session)

		// Live execution is exec-2 (cycle 2). The stale event below carries
		// exec-1 (cycle 1).
		seedExecutorRunning(t, repo, "s1", "t1", "exec-2")

		agentMgr := &mockAgentManager{repoForExecutionLookup: repo}
		svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)

		svc.handleAgentStopped(ctx, watcher.AgentEventData{
			TaskID:           "t1",
			SessionID:        "s1",
			AgentExecutionID: "exec-1",
		})

		updated, _ := repo.GetTaskSession(ctx, "s1")
		if updated.State != models.TaskSessionStateRunning {
			t.Errorf("expected state to remain %q (rotated event ignored), got %q",
				models.TaskSessionStateRunning, updated.State)
		}
	})
}

// waitForStopCall polls until the mock agent manager has received at least one
// StopAgentWithReason call, or fails the test after a timeout.
func waitForStopCall(t *testing.T, agentMgr *mockAgentManager) {
	t.Helper()
	deadline := time.After(2 * time.Second)
	for {
		agentMgr.mu.Lock()
		calls := len(agentMgr.stopAgentWithReasonArgs)
		agentMgr.mu.Unlock()
		if calls > 0 {
			return
		}
		select {
		case <-deadline:
			t.Fatal("expected StopAgentWithReason to be called, but it was not")
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}
}
