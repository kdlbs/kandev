package executor

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

func TestRegisterResumeInitialPromptCallbacks(t *testing.T) {
	manager := &resumePromptCallbackManager{mockAgentManager: &mockAgentManager{}}
	exec := newTestExecutor(t, manager, newMockRepository())
	var before, dispatched, failed bool
	options := ResumeOptions{
		BeforeInitialPromptDispatch: func(context.Context) error {
			before = true
			return nil
		},
		OnInitialPromptDispatched: func() { dispatched = true },
		OnInitialPromptFailed:     func() { failed = true },
	}
	if err := exec.registerResumeInitialPromptCallbacks("execution", options); err != nil {
		t.Fatalf("registerResumeInitialPromptCallbacks returned error: %v", err)
	}
	if manager.beforeAdmission == nil || manager.onDispatched == nil || manager.onFailure == nil {
		t.Fatal("resume initial prompt callbacks were not registered")
	}
	if err := manager.beforeAdmission(context.Background(), "prompt", nil); err != nil {
		t.Fatalf("before admission returned error: %v", err)
	}
	manager.onDispatched()
	manager.onFailure()
	if !before || !dispatched || !failed {
		t.Fatalf("callbacks were not invoked: before=%v dispatched=%v failed=%v", before, dispatched, failed)
	}
}

type resumePromptCallbackManager struct {
	*mockAgentManager
	beforeAdmission func(context.Context, string, []v1.MessageAttachment) error
	onDispatched    func()
	onFailure       func()
}

func (m *resumePromptCallbackManager) RegisterInitialPromptAdmissionCallbacks(
	_ string,
	beforeAdmission func(context.Context, string, []v1.MessageAttachment) error,
	onDispatched, onFailure func(),
) error {
	m.beforeAdmission = beforeAdmission
	m.onDispatched = onDispatched
	m.onFailure = onFailure
	return nil
}

func TestBuildResumeRequestWithOptions_FreshStartPromptOverride(t *testing.T) {
	task := &v1.Task{ID: "task-override", Description: "plain task description"}
	session := &models.TaskSession{ID: "session-override"}
	override := "composed workflow instructions"

	t.Run("absent override keeps the description fallback", func(t *testing.T) {
		req, _ := newResumeLaunchRequest(task, session, true, ResumeOptions{})
		if req.TaskDescription != "plain task description" {
			t.Fatalf("TaskDescription = %q, want the ordinary description fallback", req.TaskDescription)
		}
	})

	t.Run("nonempty override replaces the description fallback", func(t *testing.T) {
		req, _ := newResumeLaunchRequest(task, session, true, ResumeOptions{
			InitialPromptOverride: &override,
		})
		if req.TaskDescription != override {
			t.Fatalf("TaskDescription = %q, want the recovery override", req.TaskDescription)
		}
	})

	t.Run("explicit empty override suppresses the description fallback", func(t *testing.T) {
		empty := ""
		req, _ := newResumeLaunchRequest(task, session, true, ResumeOptions{
			InitialPromptOverride: &empty,
		})
		if req.TaskDescription != "" {
			t.Fatalf("TaskDescription = %q, want an explicitly empty prompt", req.TaskDescription)
		}
	})

	t.Run("NoInitialPrompt takes precedence over the override", func(t *testing.T) {
		req, _ := newResumeLaunchRequest(task, session, true, ResumeOptions{
			InitialPromptOverride: &override,
			NoInitialPrompt:       true,
		})
		if req.TaskDescription != "" {
			t.Fatalf("TaskDescription = %q, want promptless suppression", req.TaskDescription)
		}
	})

	t.Run("context continuation takes precedence over the override", func(t *testing.T) {
		req, _ := newResumeLaunchRequest(task, session, true, ResumeOptions{
			InitialPromptOverride:    &override,
			ForceContextContinuation: true,
			ContinuationPrompt:       "bounded continuation",
		})
		if req.TaskDescription != "bounded continuation" {
			t.Fatalf("TaskDescription = %q, want the continuation prompt", req.TaskDescription)
		}
	})

	t.Run("native resume suppression takes precedence over the override", func(t *testing.T) {
		repo := newMockRepository()
		setupLiveResumeTestFixture(repo)
		exec := newTestExecutor(t, &mockAgentManager{}, repo)

		req, _, _, _, _, err := exec.buildResumeRequestAtCredentialBoundaryWithOptions(
			context.Background(), repo.tasks["task-1"].ToAPI(), repo.sessions["sess-1"], true, nil,
			ResumeOptions{InitialPromptOverride: &override},
		)
		if err != nil {
			t.Fatalf("buildResumeRequestWithOptions returned error: %v", err)
		}
		if req.TaskDescription != "" {
			t.Fatalf("TaskDescription = %q, want native resume suppression", req.TaskDescription)
		}
		if req.ACPSessionID != "token-abc" {
			t.Fatalf("ACPSessionID = %q, want the saved provider conversation", req.ACPSessionID)
		}
	})
}
