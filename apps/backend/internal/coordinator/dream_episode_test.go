package coordinator

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/mcp/profile"
	"github.com/kandev/kandev/internal/orchestrator"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	taskrepo "github.com/kandev/kandev/internal/task/repository"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

type fakeEpisodeSessions struct {
	resp *orchestrator.EnsureSessionResponse
	err  error
	opts orchestrator.EnsureSessionOptions
}

func (f *fakeEpisodeSessions) EnsureSession(_ context.Context, _ string, opts ...orchestrator.EnsureSessionOptions) (*orchestrator.EnsureSessionResponse, error) {
	if len(opts) > 0 {
		f.opts = opts[0]
	}
	return f.resp, f.err
}

type recordingDreamOrchestrator struct {
	sessionState   taskmodels.TaskSessionState
	launchRequest  *orchestrator.LaunchSessionRequest
	launchResponse *orchestrator.LaunchSessionResponse
	launchErr      error
	waitErr        error
	promptCalls    int
	events         []string
}

func (f *recordingDreamOrchestrator) LaunchSession(_ context.Context, req *orchestrator.LaunchSessionRequest) (*orchestrator.LaunchSessionResponse, error) {
	f.events = append(f.events, "launch")
	requestCopy := *req
	f.launchRequest = &requestCopy
	if f.launchErr != nil {
		return nil, f.launchErr
	}
	if f.launchResponse != nil {
		return f.launchResponse, nil
	}
	f.sessionState = taskmodels.TaskSessionStateStarting
	return &orchestrator.LaunchSessionResponse{TaskID: req.TaskID, SessionID: req.SessionID}, nil
}

func (f *recordingDreamOrchestrator) WaitForSessionPromptable(context.Context, string, string) error {
	f.events = append(f.events, "wait")
	if f.waitErr != nil {
		return f.waitErr
	}
	f.sessionState = taskmodels.TaskSessionStateWaitingForInput
	return nil
}

func (f *recordingDreamOrchestrator) PromptTask(_ context.Context, _, _, _, _ string, _ bool, _ []v1.MessageAttachment, _ bool) (*orchestrator.PromptResult, error) {
	f.events = append(f.events, "prompt")
	f.promptCalls++
	if f.sessionState == taskmodels.TaskSessionStateCreated {
		return nil, orchestrator.ErrSessionNotPromptable
	}
	return &orchestrator.PromptResult{AgentMessage: "dream answer"}, nil
}

func TestDreamEpisode_TaskIsEphemeralDreamTaskBoundToOneTool(t *testing.T) {
	tasks := newFakeConversationTasks()
	ep := DreamEpisode{Tasks: tasks}
	c := &Coordinator{ID: "c1", WorkspaceID: "ws", Name: "Ops", AgentProfileID: "ap", ExecutorProfileID: "ep", PolicyRevision: 3}
	id, err := ep.CreateTask(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}
	task := tasks.tasks[id]
	if !task.IsEphemeral || task.Metadata[taskmodels.MetaKeyCoordinatorPurpose] != taskmodels.CoordinatorPurposeDream {
		t.Fatalf("task = %+v", task)
	}
	if task.Metadata[taskmodels.MetaKeyCoordinatorID] != "c1" {
		t.Fatalf("coordinator id = %v", task.Metadata[taskmodels.MetaKeyCoordinatorID])
	}
	got, err := profile.ParseCoordinatorToolPolicyMetadata(task.Metadata[profile.CoordinatorToolPolicyMetadataKey])
	if err != nil {
		t.Fatal(err)
	}
	if len(got.ToolNames) != 1 || got.ToolNames[0] != "list_coordinator_turns_kandev" || got.ConversationTaskID != id {
		t.Fatalf("policy = %+v", got)
	}
}

func TestDreamEpisode_BindFailureStillReturnsTheTaskIDForCleanup(t *testing.T) {
	tasks := newFakeConversationTasks()
	tasks.updateErr = errors.New("boom")
	id, err := DreamEpisode{Tasks: tasks}.CreateTask(context.Background(), &Coordinator{ID: "c1", WorkspaceID: "ws"})
	if err == nil || id == "" {
		t.Fatalf("id = %q err = %v", id, err)
	}
}

func TestDreamEpisode_SessionIsNotAutoStarted(t *testing.T) {
	s := &fakeEpisodeSessions{resp: &orchestrator.EnsureSessionResponse{SessionID: "s1"}}
	sid, err := DreamEpisode{Sessions: s}.CreateSession(context.Background(), "t1")
	if err != nil || sid != "s1" {
		t.Fatalf("sid = %q err = %v", sid, err)
	}
	if s.opts.AutoStart == nil || *s.opts.AutoStart {
		t.Fatal("session auto-started")
	}
	if _, err := (DreamEpisode{Sessions: &fakeEpisodeSessions{resp: &orchestrator.EnsureSessionResponse{}}}).CreateSession(context.Background(), "t1"); err == nil {
		t.Fatal("empty session id accepted")
	}
}

func TestDreamEpisode_ArchiveTreatsMissingAndArchivedAsArchived(t *testing.T) {
	tasks := newFakeConversationTasks()
	ep := DreamEpisode{Tasks: tasks}
	id, err := ep.CreateTask(context.Background(), &Coordinator{ID: "c1", WorkspaceID: "ws"})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := ep.Archive(context.Background(), id); err != nil {
			t.Fatalf("archive %d: %v", i, err)
		}
	}
	if err := ep.Archive(context.Background(), "gone"); err != nil {
		t.Fatalf("missing task: %v", err)
	}
	tasks.archiveErr = errors.New("db down")
	if err := ep.Archive(context.Background(), id); err == nil || errors.Is(err, taskrepo.ErrTaskNotFound) {
		t.Fatalf("real failure swallowed: %v", err)
	}
}

func TestDreamEpisode_PromptReturnsTheAgentMessage(t *testing.T) {
	ep := DreamEpisode{Prompter: func(_ context.Context, task, session, prompt string) (string, error) {
		return task + session + prompt, nil
	}}
	got, err := ep.Prompt(context.Background(), "t", "s", "p")
	if err != nil || got != "tsp" {
		t.Fatalf("got %q, %v", got, err)
	}
}

// @covers AC-COORDINATOR-SHADOW-DREAM-002.4
// @covers AC-COORDINATOR-SHADOW-DREAM-003.2
func TestPromptDreamEpisode_StartsPreparedSessionBeforePrompt(t *testing.T) {
	fake := &recordingDreamOrchestrator{sessionState: taskmodels.TaskSessionStateCreated}
	got, err := PromptDreamEpisode(fake)(context.Background(), "task-1", "session-1", "dream prompt")
	if err != nil {
		t.Fatalf("PromptDreamEpisode error = %v", err)
	}
	if got != "dream answer" {
		t.Fatalf("answer = %q, want dream answer", got)
	}
	if len(fake.events) != 2 || fake.events[0] != "launch" || fake.events[1] != "prompt" {
		t.Fatalf("events = %v, want launch before prompt", fake.events)
	}
	if fake.launchRequest == nil {
		t.Fatal("LaunchSession was not called")
	}
	if got := fake.launchRequest; got.TaskID != "task-1" || got.SessionID != "session-1" || got.Intent != orchestrator.IntentStartCreated || !got.AutoStart || !got.NoInitialPrompt || !got.SkipMessageRecord || got.Prompt != "" || got.ActivationSource != "" {
		t.Fatalf("launch request = %+v", got)
	}
}

func TestPromptDreamEpisode_LaunchFailureSendsNoPrompt(t *testing.T) {
	launchErr := errors.New("launch failed")
	fake := &recordingDreamOrchestrator{sessionState: taskmodels.TaskSessionStateCreated, launchErr: launchErr}
	_, err := PromptDreamEpisode(fake)(context.Background(), "task-1", "session-1", "dream prompt")
	if !errors.Is(err, launchErr) || err.Error() != "start dream episode session: launch failed" {
		t.Fatalf("PromptDreamEpisode error = %v", err)
	}
	if fake.promptCalls != 0 || len(fake.events) != 1 || fake.events[0] != "launch" {
		t.Fatalf("prompt calls = %d, events = %v; want no prompt after launch failure", fake.promptCalls, fake.events)
	}
}

func TestPromptDreamEpisode_WaitsForQueuedSessionBeforePrompt(t *testing.T) {
	fake := &recordingDreamOrchestrator{
		sessionState: taskmodels.TaskSessionStateCreated,
		launchResponse: &orchestrator.LaunchSessionResponse{
			TaskID: "task-1", SessionID: "session-1",
			State: string(taskmodels.TaskSessionStateCreated), ActivationDisposition: orchestrator.LaunchActivationDispositionQueued,
		},
	}

	got, err := PromptDreamEpisode(fake)(context.Background(), "task-1", "session-1", "dream prompt")
	if err != nil {
		t.Fatalf("PromptDreamEpisode error = %v", err)
	}
	if got != "dream answer" {
		t.Fatalf("answer = %q, want dream answer", got)
	}
	if len(fake.events) != 3 || fake.events[0] != "launch" || fake.events[1] != "wait" || fake.events[2] != "prompt" {
		t.Fatalf("events = %v, want launch, wait, prompt", fake.events)
	}
	if fake.promptCalls != 1 {
		t.Fatalf("prompt calls = %d, want exactly one", fake.promptCalls)
	}
}

func TestPromptDreamEpisode_QueuedSessionWaitFailureSendsNoPrompt(t *testing.T) {
	waitErr := errors.New("session readiness timed out")
	fake := &recordingDreamOrchestrator{
		sessionState: taskmodels.TaskSessionStateCreated,
		launchResponse: &orchestrator.LaunchSessionResponse{
			TaskID: "task-1", SessionID: "session-1",
			State: string(taskmodels.TaskSessionStateCreated), ActivationDisposition: orchestrator.LaunchActivationDispositionQueued,
		},
		waitErr: waitErr,
	}

	_, err := PromptDreamEpisode(fake)(context.Background(), "task-1", "session-1", "dream prompt")
	if !errors.Is(err, waitErr) || err.Error() != "wait for queued dream episode session: session readiness timed out" {
		t.Fatalf("PromptDreamEpisode error = %v", err)
	}
	if fake.promptCalls != 0 || len(fake.events) != 2 || fake.events[0] != "launch" || fake.events[1] != "wait" {
		t.Fatalf("prompt calls = %d, events = %v; want wait failure and no prompt", fake.promptCalls, fake.events)
	}
}
