package orchestrator_test

// AC-69: proves the launch-recogniser seam is genuinely extensible "by
// construction". background_launch_extensibility_test.go (package
// orchestrator) already proves the same behavioral outcome more cheaply, but
// AC-69's own text requires the guarantee to be checkable mechanically: "the
// test's only production-code interaction is the registration call, and the
// test package imports nothing from the probe, projection or rendering
// paths." A test living in package orchestrator reaches the projection's
// unexported methods by same-package access, which doesn't prove that. This
// file is a true external test package: it registers a second recogniser
// through backgroundlaunch's public API, then drives the exact bus-event
// path production code uses — bus.NewMemoryEventBus plus
// events.BuildAgentStreamSubject, the same plumbing
// lifecycle.EventPublisher.PublishAgentStreamEventPayload uses in production
// — and reads the outcome back through orchestrator.Service's exported
// accessors. It never calls an unexported orchestrator function.

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	"github.com/kandev/kandev/internal/agent/agents"
	client "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/agentctl/server/adapter/transport/acp/backgroundlaunch"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/orchestrator"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/scheduler"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// fakeVendorRecognizer mirrors the existing in-package test double exactly —
// it is not itself the thing under test; the registration and the resulting
// pipeline behavior are.
type fakeVendorRecognizer struct{ agentID string }

func (f fakeVendorRecognizer) AgentID() string { return f.agentID }

func (f fakeVendorRecognizer) RecognizesDetachedLaunch(payload *streams.NormalizedPayload) bool {
	return payload != nil && payload.ShellExec() != nil && payload.ShellExec().Background
}

// extAgentManager is a no-op stub satisfying executor.AgentManagerClient.
// This test never exercises a launch/prompt/cancel path — it only needs
// orchestrator.NewService's constructor to be satisfiable from outside the
// package.
type extAgentManager struct{}

func (extAgentManager) LaunchAgent(context.Context, *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
	return nil, nil
}
func (extAgentManager) StartAgentProcess(context.Context, string) error { return nil }
func (extAgentManager) IsAgentCommandConfigured(string) bool            { return true }
func (extAgentManager) StopAgent(context.Context, string, bool) error   { return nil }
func (extAgentManager) StopAgentWithReason(context.Context, string, string, bool) error {
	return nil
}
func (extAgentManager) PromptAgent(context.Context, string, string, []v1.MessageAttachment, bool) (*executor.PromptResult, error) {
	return nil, nil
}
func (extAgentManager) CancelAgent(context.Context, string) error { return nil }
func (extAgentManager) RespondToPermissionBySessionID(context.Context, string, string, string, bool) error {
	return nil
}
func (extAgentManager) ProbeBackgroundWorkloads(context.Context, string) (client.ProbeResult, error) {
	return client.ProbeResultUnknown, nil
}
func (extAgentManager) IsAgentRunningForSession(context.Context, string) bool { return false }
func (extAgentManager) IsAgentReadyForPrompt(context.Context, string) bool    { return false }
func (extAgentManager) ResolveAgentProfile(context.Context, string) (*executor.AgentProfileInfo, error) {
	return nil, nil
}
func (extAgentManager) SetExecutionDescription(context.Context, string, string) error { return nil }
func (extAgentManager) SetExecutionEnv(context.Context, string, map[string]string) error {
	return nil
}
func (extAgentManager) SetMcpMode(context.Context, string, string) error  { return nil }
func (extAgentManager) RestartAgentProcess(context.Context, string) error { return nil }
func (extAgentManager) ResetAgentContext(context.Context, string) error   { return nil }
func (extAgentManager) SetSessionModelBySessionID(context.Context, string, string) error {
	return nil
}
func (extAgentManager) SetSessionModeBySessionID(context.Context, string, string) error {
	return nil
}
func (extAgentManager) WasSessionInitialized(string) bool { return false }
func (extAgentManager) GetSessionAuthMethods(string) []streams.AuthMethodInfo {
	return nil
}
func (extAgentManager) IsPassthroughSession(context.Context, string) bool { return false }
func (extAgentManager) WritePassthroughStdin(context.Context, string, string) error {
	return nil
}
func (extAgentManager) ResolvePassthroughConfig(context.Context, string) (agents.PassthroughConfig, error) {
	return agents.PassthroughConfig{}, nil
}
func (extAgentManager) MarkPassthroughRunning(string) error { return nil }
func (extAgentManager) GetRemoteRuntimeStatusBySession(context.Context, string) (*executor.RemoteRuntimeStatus, error) {
	return nil, nil
}
func (extAgentManager) PollRemoteStatusForRecords(context.Context, []executor.RemoteStatusPollRequest) {
}
func (extAgentManager) CleanupStaleExecutionBySessionID(context.Context, string) error { return nil }
func (extAgentManager) EnsureWorkspaceExecutionForSession(context.Context, string, string) error {
	return nil
}
func (extAgentManager) GetExecutionIDForSession(context.Context, string) (string, error) {
	return "", nil
}
func (extAgentManager) GetGitLog(context.Context, string, string, int, string) (*client.GitLogResult, error) {
	return nil, nil
}
func (extAgentManager) GetCumulativeDiff(context.Context, string, string) (*client.CumulativeDiffResult, error) {
	return nil, nil
}
func (extAgentManager) GetGitStatus(context.Context, string) (*client.GitStatusResult, error) {
	return nil, nil
}
func (extAgentManager) GetGitStatusFresh(context.Context, string) (*client.GitStatusResult, error) {
	return nil, nil
}
func (extAgentManager) WaitForAgentctlReady(context.Context, string) error { return nil }

var _ executor.AgentManagerClient = extAgentManager{}

// extTaskRepo is a minimal in-memory scheduler.TaskRepository.
type extTaskRepo struct {
	mu    sync.Mutex
	tasks map[string]*v1.Task
}

func newExtTaskRepo() *extTaskRepo { return &extTaskRepo{tasks: make(map[string]*v1.Task)} }

func (r *extTaskRepo) seed(id string, state v1.TaskState) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tasks[id] = &v1.Task{ID: id, State: state}
}

func (r *extTaskRepo) GetTask(_ context.Context, taskID string) (*v1.Task, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.tasks[taskID], nil
}

func (r *extTaskRepo) UpdateTaskState(_ context.Context, taskID string, state v1.TaskState) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if t, ok := r.tasks[taskID]; ok {
		t.State = state
	}
	return nil
}

func (r *extTaskRepo) UpdateTaskStateIfCurrentIn(
	_ context.Context, taskID string, state v1.TaskState, allowed []v1.TaskState,
) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	t, ok := r.tasks[taskID]
	if !ok {
		return false, nil
	}
	for _, candidate := range allowed {
		if t.State == candidate {
			t.State = state
			return true, nil
		}
	}
	return false, nil
}

func (r *extTaskRepo) UpdateTaskStateIfNotArchived(_ context.Context, taskID string, state v1.TaskState) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if t, ok := r.tasks[taskID]; ok {
		t.State = state
		return true, nil
	}
	return false, nil
}

func (r *extTaskRepo) UpdateTaskStateIfSessionState(
	ctx context.Context, taskID, _ string, _ models.TaskSessionState, state v1.TaskState,
) (bool, error) {
	return r.UpdateTaskStateIfNotArchived(ctx, taskID, state)
}

var _ scheduler.TaskRepository = (*extTaskRepo)(nil)

// extStepGetter is a no-op orchestrator.WorkflowStepGetter — this test never
// seeds a real workflow step, mirroring the existing ordering test
// (parked_projection_turn_finished_ordering_test.go), which shows the
// session-state transition this test relies on does not require one.
type extStepGetter struct{}

func (extStepGetter) GetStep(context.Context, string) (*wfmodels.WorkflowStep, error) {
	return nil, nil
}

func (extStepGetter) GetNextStepByPosition(context.Context, string, int) (*wfmodels.WorkflowStep, error) {
	return nil, nil
}

func (extStepGetter) GetPreviousStepByPosition(context.Context, string, int) (*wfmodels.WorkflowStep, error) {
	return nil, nil
}

func (extStepGetter) GetWorkflowMeta(context.Context, string) (orchestrator.WorkflowMeta, error) {
	return orchestrator.WorkflowMeta{}, nil
}

var _ orchestrator.WorkflowStepGetter = extStepGetter{}

// constantProbe is a fixed-answer orchestrator.BackgroundProbe.
type constantProbe struct{ result executor.ProbeResult }

func (p constantProbe) Probe(context.Context, string) (executor.ProbeResult, error) {
	return p.result, nil
}

var _ orchestrator.BackgroundProbe = constantProbe{}

// publishAgentStreamEvent publishes a stream event through the exact subject
// and envelope lifecycle.EventPublisher.PublishAgentStreamEventPayload uses
// in production — the only path that reaches orchestrator.Service's
// unexported handleAgentStreamEvent from outside the package.
func publishAgentStreamEvent(
	t *testing.T, eventBus bus.EventBus, taskID, sessionID string, data *lifecycle.AgentStreamEventData,
) {
	t.Helper()
	payload := lifecycle.AgentStreamEventPayload{
		Type:      "agent/event",
		Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
		TaskID:    taskID,
		SessionID: sessionID,
		Data:      data,
	}
	busEvent := bus.NewEvent(events.AgentStream, "test", payload)
	subject := events.BuildAgentStreamSubject(sessionID)
	if err := eventBus.Publish(context.Background(), subject, busEvent); err != nil {
		t.Fatalf("publish %s stream event: %v", data.Type, err)
	}
}

// AC-69(a): a second agent's recogniser, registered through the public
// backgroundlaunch.Register API with zero change to orchestrator production
// code, drives a detached-shell launch all the way through the parked
// projection — attestation, the settle-time probe, and both the session- and
// task-level snapshots — exactly as a Claude session does. Unlike
// background_launch_extensibility_test.go (package orchestrator), this test
// package imports nothing from the probe, projection, or rendering paths:
// every interaction with orchestrator.Service goes through NewService and
// its exported setters/accessors, and the only way a stream event reaches
// the service is the same bus.EventBus publish production code uses.
func TestSecondRegisteredRecognizer_DetachedLaunchParksThroughSettle_ByConstruction(t *testing.T) {
	const fakeAgentID = "orchestrator-extensibility-external-test-agent"
	backgroundlaunch.Register(fakeVendorRecognizer{agentID: fakeAgentID})
	t.Cleanup(func() { backgroundlaunch.Unregister(fakeAgentID) })

	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "console"})
	if err != nil {
		t.Fatalf("build logger: %v", err)
	}

	tmpDir := t.TempDir()
	dbConn, err := db.OpenSQLite(filepath.Join(tmpDir, "test.db"))
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	sqlxDB := sqlx.NewDb(dbConn, "sqlite3")
	t.Cleanup(func() { _ = sqlxDB.Close() })
	repo, cleanup, err := repository.Provide(sqlxDB, sqlxDB, nil)
	if err != nil {
		t.Fatalf("create test repository: %v", err)
	}
	t.Cleanup(func() { _ = cleanup() })

	eventBus := bus.NewMemoryEventBus(log)
	taskRepo := newExtTaskRepo()
	svc := orchestrator.NewService(
		orchestrator.DefaultServiceConfig(),
		eventBus,
		extAgentManager{},
		taskRepo,
		repo,
		nil, // executor.ShellPreferenceProvider — unused on the stream-event path this test drives
		nil, // secrets.SecretStore — unused on the stream-event path this test drives
		nil, // messagequeue.Service — nil defaults to an in-memory queue
		log,
	)
	svc.SetWorkflowStepGetter(extStepGetter{})
	svc.SetBackgroundProbe(constantProbe{result: executor.ProbeResultLive})

	t.Cleanup(func() { _ = svc.Stop() })
	if err := svc.Start(context.Background()); err != nil {
		t.Fatalf("start orchestrator service: %v", err)
	}

	const taskID = "task-second-agent-ext"
	const sessionID = "sess-second-agent-ext"
	ctx := context.Background()
	now := time.Now().UTC()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{
		ID: "ws-ext", Name: "Test", CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := repo.CreateTask(ctx, &models.Task{
		ID: taskID, WorkspaceID: "ws-ext", Title: "Test", Description: "Test",
		State: v1.TaskStateInProgress, CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("seed task: %v", err)
	}
	// STARTING (not RUNNING): handleCompleteStreamEvent defers the
	// running->waiting transition to a later READY event when the session is
	// still RUNNING at complete time, mirroring
	// parked_projection_turn_finished_ordering_test.go's seeding choice.
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: sessionID, TaskID: taskID, State: models.TaskSessionStateStarting,
		StartedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("seed task session: %v", err)
	}
	taskRepo.seed(taskID, v1.TaskStateInProgress)

	// Mirrors stampBackgroundShellWork (acp/normalize.go) exactly: the
	// orchestrator never talks to the registry itself, only to whatever the
	// adapter layer already stamped onto the normalized payload. This is the
	// test's only production-code interaction beyond the registration call.
	payload := streams.NewShellExec("sleep 300", "", "", 0, true)
	if backgroundlaunch.RecognizesDetachedLaunch(fakeAgentID, payload) {
		payload.SetBackgroundWorkIdentity(streams.BackgroundWorkKindShell, "", true, false)
	}

	publishAgentStreamEvent(t, eventBus, taskID, sessionID, &lifecycle.AgentStreamEventData{
		Type:       "tool_update",
		ToolCallID: "tool-1",
		ToolStatus: "completed",
		Normalized: payload,
	})
	publishAgentStreamEvent(t, eventBus, taskID, sessionID, &lifecycle.AgentStreamEventData{
		Type: "complete",
	})

	if parked, revision := svc.ParkedSnapshot(sessionID); !parked || revision != 1 {
		t.Fatalf("ParkedSnapshot = (%v, %d), want (true, 1)", parked, revision)
	}
	if parked, _ := svc.TaskParkedSnapshot(taskID); !parked {
		t.Fatalf("TaskParkedSnapshot(%q).parked = false, want true", taskID)
	}
}
