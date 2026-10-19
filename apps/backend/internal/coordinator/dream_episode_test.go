package coordinator

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/mcp/profile"
	"github.com/kandev/kandev/internal/orchestrator"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	taskrepo "github.com/kandev/kandev/internal/task/repository"
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
