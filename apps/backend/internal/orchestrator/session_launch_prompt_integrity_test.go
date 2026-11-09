package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/sysprompt"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

const explicitLaunchBrief = "Independent READ-ONLY review. Do not edit, rebase, commit, push, or publish."

type failingSubmissionCASRepository struct {
	*sqliterepo.Repository
	err error
}

func (r *failingSubmissionCASRepository) SetSessionMetadataKeyIfJSONValue(
	context.Context, string, string, interface{}, interface{},
) (bool, error) {
	return false, r.err
}

type explicitLaunchFixture struct {
	repo       *sqliterepo.Repository
	svc        *Service
	agent      *mockAgentManager
	messages   *repositoryBackedMessageCreator
	stepGetter *mockStepGetter
	taskRepo   *mockTaskRepo
	taskID     string
	sessionID  string
	stepID     string
	launched   []string
	launchReq  *executor.LaunchAgentRequest
}

func newExplicitLaunchFixture(t *testing.T, name, stepPrompt, description string) *explicitLaunchFixture {
	t.Helper()
	f := &explicitLaunchFixture{
		repo:      setupTestRepo(t),
		taskID:    "task-launch-integrity-" + name,
		sessionID: "session-launch-integrity-" + name,
		stepID:    "step-launch-integrity-" + name,
	}
	seedTaskAndSessionWithStep(t, f.repo, f.taskID, f.sessionID, f.stepID)

	stepGetter := newMockStepGetter()
	stepGetter.steps[f.stepID] = &wfmodels.WorkflowStep{
		ID: f.stepID, WorkflowID: "wf1", Name: "Launch integrity", Prompt: stepPrompt,
	}
	taskRepo := newMockTaskRepo()
	taskRepo.tasks[f.taskID] = &v1.Task{
		ID: f.taskID, Title: "Launch integrity", Description: description, State: v1.TaskStateInProgress,
	}
	f.agent = &mockAgentManager{
		repoForExecutionLookup: f.repo,
		launchAgentFunc: func(_ context.Context, req *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
			f.launchReq = req
			f.launched = append(f.launched, req.TaskDescription)
			return &executor.LaunchAgentResponse{AgentExecutionID: "exec-" + f.sessionID}, nil
		},
	}
	f.svc = createTestServiceWithScheduler(f.repo, stepGetter, taskRepo, f.agent)
	f.svc.turnService = &repoTurnService{repo: f.repo}
	f.messages = &repositoryBackedMessageCreator{mockMessageCreator: &mockMessageCreator{}, repo: f.repo}
	f.svc.messageCreator = f.messages
	f.stepGetter = stepGetter
	f.taskRepo = taskRepo
	return f
}

func (f *explicitLaunchFixture) launch(t *testing.T, req *LaunchSessionRequest) *LaunchSessionResponse {
	t.Helper()
	if req.TaskID == "" {
		req.TaskID = f.taskID
	}
	if req.SessionID == "" {
		req.SessionID = f.sessionID
	}
	if req.Intent == "" {
		req.Intent = IntentStartCreated
	}
	if req.AgentProfileID == "" {
		req.AgentProfileID = "profile1"
	}
	response, err := f.svc.LaunchSession(context.Background(), req)
	require.NoError(t, err)
	return response
}

func (f *explicitLaunchFixture) storedUserMessages(t *testing.T) []*models.Message {
	t.Helper()
	messages, err := f.repo.ListMessages(context.Background(), f.sessionID)
	require.NoError(t, err)
	user := make([]*models.Message, 0, 1)
	for _, message := range messages {
		if message.AuthorType == models.MessageAuthorUser {
			user = append(user, message)
		}
	}
	return user
}

// @covers AC-TASKS-INITIAL-TASK-BRIEF-001.13, AC-TASKS-INITIAL-TASK-BRIEF-001.14
func TestLaunchStartCreated_ExplicitPromptIntegrity(t *testing.T) {
	const replacing = "Read the retained implementation plan and continue."

	for _, test := range []struct {
		name       string
		stepPrompt string
	}{
		{name: "replacing-template", stepPrompt: replacing},
		{name: "placeholder-template", stepPrompt: replacing + "\n\n{{task_prompt}}"},
		{name: "empty-template"},
	} {
		t.Run(test.name, func(t *testing.T) {
			f := newExplicitLaunchFixture(t, test.name, test.stepPrompt, "The task description must not replace the explicit brief.")
			f.launch(t, &LaunchSessionRequest{Prompt: explicitLaunchBrief, AutoStart: true})

			require.Len(t, f.launched, 1, "the launch must reach the manager once")
			launched := f.launched[0]
			require.Contains(t, launched, explicitLaunchBrief)
			require.Equal(t, 1, strings.Count(launched, explicitLaunchBrief),
				"the explicit brief must appear once in the manager-bound prompt")
			if test.stepPrompt != "" {
				require.Contains(t, launched, replacing)
			}

			stored := f.storedUserMessages(t)
			require.Len(t, stored, 1, "the launch must record one user message")
			apiMessage := stored[0].ToAPI()
			require.Contains(t, apiMessage.Content, explicitLaunchBrief)
			require.Equal(t, 1, strings.Count(apiMessage.Content, explicitLaunchBrief),
				"the visible message must retain the explicit brief once")
			require.Contains(t, apiMessage.RawContent, explicitLaunchBrief)
			require.Equal(t, 1, strings.Count(apiMessage.RawContent, explicitLaunchBrief),
				"the raw message must retain the explicit brief once")
			require.NotContains(t, apiMessage.Content,
				"The task description must not replace the explicit brief.")
		})
	}
}

// @covers AC-TASKS-SAVED-PROMPT-DELIVERY-001.6, AC-TASKS-SAVED-PROMPT-DELIVERY-001.8
func TestLaunchStartCreated_ExplicitPromptWithSavedReference(t *testing.T) {
	ctx := context.Background()
	promptService := newPromptServiceForLaunchFallbackTest(t)
	_, err := promptService.CreatePrompt(ctx, "principles", "Apply the repository principles.")
	require.NoError(t, err)

	const replacing = "Read the retained implementation plan and continue."
	reference := "Follow @principles."
	f := newExplicitLaunchFixture(t, "saved-reference", replacing, "Description must not replace the brief.")
	f.svc.promptExpander = promptService
	f.launch(t, &LaunchSessionRequest{Prompt: explicitLaunchBrief + "\n\n" + reference, AutoStart: true})

	require.Len(t, f.launched, 1)
	launched := f.launched[0]
	require.Equal(t, 1, strings.Count(launched, explicitLaunchBrief))
	require.Equal(t, 1, strings.Count(launched, "Apply the repository principles."))
	require.Contains(t, launched, replacing)

	stored := f.storedUserMessages(t)
	require.Len(t, stored, 1)
	apiMessage := stored[0].ToAPI()
	require.Equal(t, 1, strings.Count(apiMessage.Content, explicitLaunchBrief))
	require.NotContains(t, apiMessage.Content, "Apply the repository principles.",
		"the expanded saved prompt stays hidden from visible content")
	require.Equal(t, 1, strings.Count(apiMessage.RawContent, "Apply the repository principles."))
}

// @covers AC-TASKS-INITIAL-TASK-BRIEF-001.14, AC-TASKS-SAVED-PROMPT-DELIVERY-001.6, AC-TASKS-PROMPT-ATTACHMENTS-001.2, AC-TASKS-PROMPT-ATTACHMENTS-001.3
func TestLaunchStartCreated_MixedContextCanonicalPrompt(t *testing.T) {
	ctx := context.Background()
	const (
		workflowPrompt = "Workflow rule: preserve the accepted brief."
		stepPrompt     = "Read the retained implementation plan and continue."
		savedText      = "Apply the repository principles."
	)
	promptService := newPromptServiceForLaunchFallbackTest(t)
	_, err := promptService.CreatePrompt(ctx, "principles", savedText)
	require.NoError(t, err)

	attachment := v1.MessageAttachment{
		Type: "resource", AttachmentID: "attachment-launch-brief",
		MimeType: "text/plain", Name: "brief.txt", SizeBytes: 12,
	}
	f := newExplicitLaunchFixture(t, "mixed-context", stepPrompt, "Description must not replace the brief.")
	f.stepGetter.workflowPrompts["wf1"] = workflowPrompt
	f.svc.promptExpander = promptService
	f.svc.executor.SetCanvasesEnabled(true)
	claimer := &rejectingLaunchAttachmentClaimer{}
	f.svc.SetLaunchAttachmentClaimer(claimer)
	f.launch(t, &LaunchSessionRequest{
		Prompt:      explicitLaunchBrief + "\n\nFollow @principles.",
		PlanMode:    true,
		Attachments: []v1.MessageAttachment{attachment},
		AutoStart:   true,
	})

	require.Len(t, f.launched, 1)
	launched := f.launched[0]
	require.Equal(t, 1, strings.Count(launched, explicitLaunchBrief))
	require.Equal(t, 1, strings.Count(launched, stepPrompt))
	require.Equal(t, 1, strings.Count(launched, workflowPrompt))
	require.Equal(t, 1, strings.Count(launched, sysprompt.DefaultPlanPrefix()))
	require.Equal(t, 1, strings.Count(launched, "CANVAS AUTHORING:"))
	require.Equal(t, 1, strings.Count(launched, savedText))
	require.Equal(t, 3, strings.Count(launched, sysprompt.TagStart),
		"plan, task context, and saved-prompt context each wrap once")
	require.NotContains(t, launched, "Internal description text.")

	require.NotNil(t, f.launchReq)
	require.Equal(t, []v1.MessageAttachment{attachment}, f.launchReq.Attachments)
	require.Equal(t, []v1.MessageAttachment{attachment}, claimer.attachments)

	stored := f.storedUserMessages(t)
	require.Len(t, stored, 1)
	require.Equal(t, launched, stored[0].Content,
		"the stored canonical prompt must match the manager-bound prompt")
	apiMessage := stored[0].ToAPI()
	require.Contains(t, apiMessage.Content, explicitLaunchBrief)
	require.Contains(t, apiMessage.Content, stepPrompt)
	require.Contains(t, apiMessage.Content, workflowPrompt)
	require.NotContains(t, apiMessage.Content, sysprompt.DefaultPlanPrefix())
	require.NotContains(t, apiMessage.Content, "CANVAS AUTHORING:")
	require.NotContains(t, apiMessage.Content, savedText)
	require.Equal(t, []v1.MessageAttachment{attachment}, storedMessageAttachments(t, stored[0]))

	withoutCanvas := newExplicitLaunchFixture(t, "mixed-context-no-canvas", stepPrompt, "Description must not replace the brief.")
	withoutCanvas.svc.executor.SetCanvasesEnabled(false)
	withoutCanvas.launch(t, &LaunchSessionRequest{Prompt: explicitLaunchBrief, AutoStart: true})
	require.Len(t, withoutCanvas.launched, 1)
	require.NotContains(t, withoutCanvas.launched[0], "CANVAS AUTHORING:")
}

func storedMessageAttachments(t *testing.T, message *models.Message) []v1.MessageAttachment {
	t.Helper()
	raw, ok := message.Metadata["attachments"]
	if !ok {
		return nil
	}
	data, err := json.Marshal(raw)
	require.NoError(t, err)
	var attachments []v1.MessageAttachment
	require.NoError(t, json.Unmarshal(data, &attachments))
	return attachments
}

// @covers AC-TASKS-INITIAL-TASK-BRIEF-001.14, AC-TASKS-INITIAL-TASK-BRIEF-001.16
func TestLaunchStartCreated_PendingSubmissionReceiptAdmission(t *testing.T) {
	ctx := context.Background()
	const prompt = "Explicit launch with a matching pending receipt."

	for _, test := range []struct {
		name    string
		failCAS bool
	}{
		{name: "matching receipt transitions after provider admission"},
		{name: "CAS failure before provider admission blocks native dispatch", failCAS: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			const stepPrompt = "Read the retained implementation plan and continue."
			f := newExplicitLaunchFixture(t, "pending-receipt-"+test.name, stepPrompt, "Description must not replace the brief.")
			pending, err := models.NewInitialPromptSubmission(prompt, false, nil)
			require.NoError(t, err)
			require.NoError(t, f.repo.SetSessionMetadataKey(
				ctx, f.sessionID, models.SessionMetaKeyInitialPromptSubmission, pending,
			))

			executionID := "exec-" + f.sessionID
			var nativeDispatches atomic.Int32
			startResult := make(chan error, 1)
			f.agent.startAgentProcessFunc = func(ctx context.Context, _ string) error {
				f.agent.mu.Lock()
				beforeAdmission := f.agent.initialPromptAdmissionCallback
				accepted := f.agent.initialPromptDispatchCallback
				f.agent.mu.Unlock()
				if beforeAdmission != nil {
					if err := beforeAdmission(ctx, executionID, nil); err != nil {
						startResult <- err
						return err
					}
				}
				nativeDispatches.Add(1)
				if accepted != nil {
					accepted()
				}
				startResult <- nil
				return nil
			}
			if test.failCAS {
				f.svc.repo = &failingSubmissionCASRepository{
					Repository: f.repo,
					err:        errors.New("submission receipt changed concurrently"),
				}
			}

			_, launchErr := f.svc.LaunchSession(ctx, &LaunchSessionRequest{
				TaskID: f.taskID, SessionID: f.sessionID, Intent: IntentStartCreated,
				AgentProfileID: "profile1", Prompt: prompt, AutoStart: true,
			})
			if launchErr != nil {
				startResult <- launchErr
			}
			startErr := <-startResult
			session, loadErr := f.repo.GetTaskSession(ctx, f.sessionID)
			require.NoError(t, loadErr)
			stored, found, loadErr := models.LoadInitialPromptSubmission(session.Metadata)
			require.NoError(t, loadErr)
			require.True(t, found)

			if test.failCAS {
				require.Error(t, startErr)
				require.Zero(t, nativeDispatches.Load(), "the admission failure must block native dispatch")
				require.Equal(t, models.InitialPromptSubmissionPending, stored.State)
				require.Equal(t, true, session.Metadata[models.SessionMetaKeyInitialPromptExplicitLaunch])
				recovery, recoveryErr := f.svc.prepareInitialRecoveryPrompt(ctx, f.taskID, session, stored)
				require.NoError(t, recoveryErr)
				require.NotNil(t, recovery)
				require.NotNil(t, recovery.receipt)
				require.Contains(t, recovery.receipt.content, prompt)
				require.Equal(t, 1, strings.Count(recovery.receipt.content, prompt))
				require.Contains(t, recovery.receipt.content, stepPrompt)
				return
			}
			require.NoError(t, startErr)
			require.Equal(t, int32(1), nativeDispatches.Load())
			require.Equal(t, models.InitialPromptSubmissionAccepted, stored.State)
			require.Equal(t, executionID, stored.ExecutionID)
			require.Len(t, f.launched, 1)
			require.Equal(t, 1, strings.Count(f.launched[0], prompt))
		})
	}
}

// @covers AC-TASKS-INITIAL-TASK-BRIEF-001.16, AC-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-003.2, AC-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-003.8
func TestLaunchStartCreated_PromptOriginAndAdmission(t *testing.T) {
	const (
		description = "Internal description text."
		replacement = "Workflow replacement text."
	)

	t.Run("explicit prompt equal to description stays explicit", func(t *testing.T) {
		f := newExplicitLaunchFixture(t, "equal-description", replacement, description)
		f.launch(t, &LaunchSessionRequest{Prompt: description, AutoStart: true})
		require.Len(t, f.launched, 1)
		require.Contains(t, f.launched[0], description)
		require.Equal(t, 1, strings.Count(f.launched[0], description))
	})

	t.Run("description origin keeps replacement semantics", func(t *testing.T) {
		f := newExplicitLaunchFixture(t, "description-origin", replacement, description)
		f.launch(t, &LaunchSessionRequest{
			Prompt: description, PromptIsTaskDescription: true, AutoStart: true,
		})
		require.Len(t, f.launched, 1)
		require.Contains(t, f.launched[0], replacement)
		require.NotContains(t, f.launched[0], description)
	})

	t.Run("empty prompt falls back to the description", func(t *testing.T) {
		f := newExplicitLaunchFixture(t, "empty-prompt", replacement, description)
		f.launch(t, &LaunchSessionRequest{AutoStart: true})
		require.Len(t, f.launched, 1)
		require.Contains(t, f.launched[0], replacement)
		require.NotContains(t, f.launched[0], description)
	})

	t.Run("whitespace prompt normalizes to the description fallback", func(t *testing.T) {
		f := newExplicitLaunchFixture(t, "whitespace-prompt", replacement, description)
		f.launch(t, &LaunchSessionRequest{Prompt: "   \n\t", AutoStart: true})
		require.Len(t, f.launched, 1)
		require.Contains(t, f.launched[0], replacement)
		require.NotContains(t, f.launched[0], description)
	})

	t.Run("no initial prompt creates no prompt turn", func(t *testing.T) {
		f := newExplicitLaunchFixture(t, "no-initial-prompt", replacement, description)
		f.launch(t, &LaunchSessionRequest{NoInitialPrompt: true, AutoStart: true})
		require.Len(t, f.launched, 1)
		require.Empty(t, f.launched[0])
		require.Empty(t, f.storedUserMessages(t))
	})
}

// @covers AC-TASKS-INITIAL-TASK-BRIEF-001.16
func TestLaunchSessionRequest_DescriptionOriginIsServerOnly(t *testing.T) {
	var request LaunchSessionRequest
	require.NoError(t, json.Unmarshal([]byte(`{
		"task_id": "task-1",
		"prompt": "explicit brief",
		"prompt_is_task_description": true,
		"PromptIsTaskDescription": true
	}`), &request))
	require.False(t, request.PromptIsTaskDescription,
		"clients must not be able to claim the description origin")

	encoded, err := json.Marshal(LaunchSessionRequest{
		TaskID: "task-1", Prompt: "explicit brief", PromptIsTaskDescription: true,
	})
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "prompt_is_task_description")
	require.NotContains(t, string(encoded), "PromptIsTaskDescription")
}

// @covers AC-TASKS-INITIAL-TASK-BRIEF-001.15
func TestLaunchStartCreated_DeferredPromptIntegrity(t *testing.T) {
	ctx := context.Background()
	first := newExplicitLaunchFixture(t, "ceiling-occupant", "", "Occupant description.")
	first.svc.sessionCeiling = newSessionCeilingController(1, nil, nil)
	first.launch(t, &LaunchSessionRequest{Prompt: "occupy the ceiling", AutoStart: true})

	const (
		stepPrompt  = "Read the retained implementation plan and continue."
		description = "Deferred task description."
	)
	second := &explicitLaunchFixture{
		repo:       first.repo,
		svc:        first.svc,
		agent:      first.agent,
		messages:   first.messages,
		stepGetter: first.stepGetter,
		taskRepo:   first.taskRepo,
		taskID:     "task-launch-integrity-deferred",
		sessionID:  "session-launch-integrity-deferred",
		stepID:     "step-launch-integrity-deferred",
	}
	seedTaskAndSessionWithStep(t, second.repo, second.taskID, second.sessionID, second.stepID)
	second.stepGetter.steps[second.stepID] = &wfmodels.WorkflowStep{
		ID: second.stepID, WorkflowID: "wf1", Name: "Deferred launch", Prompt: stepPrompt,
	}
	second.taskRepo.tasks[second.taskID] = &v1.Task{
		ID: second.taskID, Title: "Deferred launch", Description: description, State: v1.TaskStateInProgress,
	}
	second.agent.launchAgentFunc = func(_ context.Context, req *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
		second.launched = append(second.launched, req.TaskDescription)
		return &executor.LaunchAgentResponse{AgentExecutionID: "exec-" + second.sessionID}, nil
	}

	response, err := first.svc.LaunchSession(ctx, &LaunchSessionRequest{
		TaskID: second.taskID, SessionID: second.sessionID, Intent: IntentStartCreated,
		AgentProfileID: "profile1", Prompt: explicitLaunchBrief, AutoStart: true,
	})
	require.NoError(t, err)
	require.NotNil(t, response)
	require.Empty(t, response.AgentExecutionID, "the over-ceiling launch must be deferred")
	require.Len(t, second.launched, 0, "a deferred launch must not reach the manager")

	task, err := second.repo.GetTask(ctx, second.taskID)
	require.NoError(t, err)
	record, ok := task.Metadata[models.MetaKeyDeferredLaunch].(map[string]interface{})
	require.True(t, ok, "the deferred launch must be persisted on the task")
	deferral, err := models.ReadCeilingDeferral(record)
	require.NoError(t, err)
	require.Equal(t, true, deferral.Payload["preserve_direct_prompt"],
		"the producer must serialize the explicit-input semantics")

	// Release the ceiling and replay the stored payload through the consumer
	// that interprets preserve_direct_prompt.
	second.svc.sessionCeiling = nil
	result := second.svc.replayCeilingLaunchStartCreated(ctx, task, deferral.Payload)
	require.NoError(t, result.err)
	require.Len(t, second.launched, 1)
	require.Contains(t, second.launched[0], explicitLaunchBrief)
	require.Equal(t, 1, strings.Count(second.launched[0], explicitLaunchBrief))
	require.Contains(t, second.launched[0], stepPrompt)
}
