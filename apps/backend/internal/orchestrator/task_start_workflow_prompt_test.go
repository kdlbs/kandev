package orchestrator

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	workflowmove "github.com/kandev/kandev/internal/workflow/move"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	currentStepPromptTestID      = "review-step"
	otherStepPromptTestID        = "selected-step"
	currentStepPromptTestMarker  = "REVIEW_STEP_PROMPT_SENTINEL"
	otherStepPromptTestMarker    = "SELECTED_STEP_PROMPT_SENTINEL"
	currentTaskPromptTestDesc    = "TASK_DESCRIPTION_SENTINEL"
	currentTaskPromptTestInput   = "CALLER_PROMPT_SENTINEL"
	currentWorkflowInstructions  = "WORKFLOW_INSTRUCTIONS_SENTINEL"
	currentOneTimeInstructions   = "ONE_TIME_INSTRUCTIONS_SENTINEL"
	currentPromptSavedReference  = "@saved-workflow-reference"
	currentPromptReferenceResult = fakeResolvedPromptReferenceContext
)

type currentStepPromptCase struct {
	name                    string
	currentTemplate         string
	otherTemplate           string
	workflowPrompt          string
	callerPrompt            string
	callerProfileID         string
	currentStepProfileID    string
	otherStepProfileID      string
	requestStepID           string
	profileExplicit         bool
	preserveWorkflowBinding bool
	entryOptions            *workflowmove.EntryOptions
	isOffice                bool
	isEphemeral             bool
	stepEnablesPlanMode     bool
	wantContains            []string
	wantAbsent              []string
	wantSavedReference      bool
	wantPlanMode            bool
	wantRuntimeConfig       bool
	wantProfileID           string
	wantCurrentTaskStepID   string
}

// @covers AC-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-007.1
// @covers AC-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-007.2
// @covers AC-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-007.3
// @covers AC-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-007.4
// @covers AC-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-007.5
// @covers AC-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-007.6
// @covers AC-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-007.7
func TestStartTask_CurrentStepPrompt(t *testing.T) {
	cases := []currentStepPromptCase{
		{
			name:                  "omitted step composes the stored step and configures the original session",
			currentTemplate:       currentStepPromptTestMarker + ": {{task_prompt}}",
			callerPrompt:          currentTaskPromptTestInput,
			currentStepProfileID:  "review-profile",
			wantContains:          []string{currentStepPromptTestMarker, currentTaskPromptTestInput},
			wantRuntimeConfig:     true,
			wantProfileID:         "review-profile",
			wantCurrentTaskStepID: currentStepPromptTestID,
		},
		{
			name:                  "explicit current step retains its prompt",
			currentTemplate:       currentStepPromptTestMarker + ": {{task_prompt}}",
			callerPrompt:          currentTaskPromptTestInput,
			currentStepProfileID:  "review-profile",
			requestStepID:         currentStepPromptTestID,
			wantContains:          []string{currentStepPromptTestMarker, currentTaskPromptTestInput},
			wantRuntimeConfig:     true,
			wantProfileID:         "review-profile",
			wantCurrentTaskStepID: currentStepPromptTestID,
		},
		{
			name:                  "explicit different step wins over the stored step",
			currentTemplate:       currentStepPromptTestMarker + ": {{task_prompt}}",
			otherTemplate:         otherStepPromptTestMarker + ": {{task_prompt}}",
			callerPrompt:          currentTaskPromptTestInput,
			currentStepProfileID:  "review-profile",
			otherStepProfileID:    "selected-profile",
			requestStepID:         otherStepPromptTestID,
			wantContains:          []string{otherStepPromptTestMarker, currentTaskPromptTestInput},
			wantAbsent:            []string{currentStepPromptTestMarker},
			wantProfileID:         "selected-profile",
			wantCurrentTaskStepID: otherStepPromptTestID,
		},
		{
			name:                  "explicit profile keeps precedence while stored step supplies prompt",
			currentTemplate:       currentStepPromptTestMarker + ": {{task_prompt}}",
			callerPrompt:          currentTaskPromptTestInput,
			callerProfileID:       "explicit-profile",
			currentStepProfileID:  "review-profile",
			profileExplicit:       true,
			wantContains:          []string{currentStepPromptTestMarker, currentTaskPromptTestInput},
			wantRuntimeConfig:     true,
			wantProfileID:         "explicit-profile",
			wantCurrentTaskStepID: currentStepPromptTestID,
		},
		{
			name:                  "template without placeholder replaces caller prompt",
			currentTemplate:       currentStepPromptTestMarker + " replaces the caller prompt",
			callerPrompt:          currentTaskPromptTestInput,
			wantContains:          []string{currentStepPromptTestMarker},
			wantAbsent:            []string{currentTaskPromptTestInput},
			wantProfileID:         "caller-profile",
			wantCurrentTaskStepID: currentStepPromptTestID,
		},
		{
			name:                  "empty template keeps caller prompt",
			currentTemplate:       "",
			callerPrompt:          currentTaskPromptTestInput,
			wantContains:          []string{currentTaskPromptTestInput},
			wantAbsent:            []string{currentStepPromptTestMarker},
			wantProfileID:         "caller-profile",
			wantCurrentTaskStepID: currentStepPromptTestID,
		},
		{
			name:                  "empty caller prompt inserts task description",
			currentTemplate:       currentStepPromptTestMarker + ": {{task_prompt}}",
			wantContains:          []string{currentStepPromptTestMarker, currentTaskPromptTestDesc},
			wantProfileID:         "caller-profile",
			wantCurrentTaskStepID: currentStepPromptTestID,
		},
		{
			name:                  "skip override on omitted step keeps workflow and one-time instructions",
			currentTemplate:       currentStepPromptTestMarker + ": {{task_prompt}}",
			workflowPrompt:        currentWorkflowInstructions,
			callerPrompt:          currentTaskPromptTestInput,
			entryOptions:          &workflowmove.EntryOptions{SkipStepPrompt: true, Instructions: currentOneTimeInstructions},
			wantContains:          []string{currentWorkflowInstructions, currentOneTimeInstructions},
			wantAbsent:            []string{currentStepPromptTestMarker, currentTaskPromptTestInput, currentTaskPromptTestDesc},
			wantRuntimeConfig:     true,
			wantProfileID:         "caller-profile",
			wantCurrentTaskStepID: currentStepPromptTestID,
		},
		{
			name:                  "skip override on explicit step keeps workflow and one-time instructions",
			currentTemplate:       currentStepPromptTestMarker + ": {{task_prompt}}",
			workflowPrompt:        currentWorkflowInstructions,
			callerPrompt:          currentTaskPromptTestInput,
			requestStepID:         currentStepPromptTestID,
			entryOptions:          &workflowmove.EntryOptions{SkipStepPrompt: true, Instructions: currentOneTimeInstructions},
			wantContains:          []string{currentWorkflowInstructions, currentOneTimeInstructions},
			wantAbsent:            []string{currentStepPromptTestMarker, currentTaskPromptTestInput, currentTaskPromptTestDesc},
			wantRuntimeConfig:     true,
			wantProfileID:         "caller-profile",
			wantCurrentTaskStepID: currentStepPromptTestID,
		},
		{
			name:                  "stored step plan mode and saved reference reach stored and dispatched prompts once",
			currentTemplate:       currentStepPromptTestMarker + " " + currentPromptSavedReference + ": {{task_prompt}}",
			callerPrompt:          currentTaskPromptTestInput,
			stepEnablesPlanMode:   true,
			wantContains:          []string{currentStepPromptTestMarker, currentTaskPromptTestInput},
			wantSavedReference:    true,
			wantPlanMode:          true,
			wantProfileID:         "caller-profile",
			wantCurrentTaskStepID: currentStepPromptTestID,
		},
		{
			name:                  "office prompt keeps caller context when step is omitted",
			currentTemplate:       currentStepPromptTestMarker + ": {{task_prompt}}",
			callerPrompt:          currentTaskPromptTestInput,
			isOffice:              true,
			wantContains:          []string{currentTaskPromptTestInput},
			wantAbsent:            []string{currentStepPromptTestMarker, currentTaskPromptTestDesc},
			wantProfileID:         "caller-profile",
			wantCurrentTaskStepID: currentStepPromptTestID,
		},
		{
			name:                    "helper prompt keeps caller context when step is omitted",
			currentTemplate:         currentStepPromptTestMarker + ": {{task_prompt}}",
			callerPrompt:            currentTaskPromptTestInput,
			callerProfileID:         "helper-profile",
			profileExplicit:         true,
			preserveWorkflowBinding: true,
			wantContains:            []string{currentTaskPromptTestInput},
			wantAbsent:              []string{currentStepPromptTestMarker, currentTaskPromptTestDesc},
			wantProfileID:           "helper-profile",
			wantCurrentTaskStepID:   currentStepPromptTestID,
		},
		{
			name:                  "ephemeral task keeps caller context",
			currentTemplate:       currentStepPromptTestMarker + ": {{task_prompt}}",
			callerPrompt:          currentTaskPromptTestInput,
			isEphemeral:           true,
			wantContains:          []string{currentTaskPromptTestInput},
			wantAbsent:            []string{currentStepPromptTestMarker, currentTaskPromptTestDesc},
			wantProfileID:         "caller-profile",
			wantCurrentTaskStepID: currentStepPromptTestID,
		},
		{
			name:                  "unbound task keeps caller context",
			callerPrompt:          currentTaskPromptTestInput,
			wantContains:          []string{currentTaskPromptTestInput},
			wantAbsent:            []string{currentStepPromptTestMarker, currentTaskPromptTestDesc},
			wantProfileID:         "caller-profile",
			wantCurrentTaskStepID: "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			runCurrentStepPromptCase(t, tc)
		})
	}
}

// @covers AC-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-007.8
func TestStartTask_CurrentStepPromptRetainsDirectPromptWhenStoredStepLookupFails(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskWithoutSession(t, repo, "task1", currentStepPromptTestID)
	stepGetter := newMockStepGetter()
	stepGetter.steps[currentStepPromptTestID] = currentStepPromptTestStep(
		currentStepPromptTestID,
		currentStepPromptTestMarker+": {{task_prompt}}",
		"",
		true,
		false,
	)
	taskRepo := newMockTaskRepo()
	taskRepo.tasks["task1"] = &v1.Task{
		ID: "task1", WorkspaceID: "ws1", WorkflowID: "wf1", Title: "Test task",
		Description: currentTaskPromptTestDesc, State: v1.TaskStateInProgress,
	}
	agentManager := &mockAgentManager{resolveProfileInfo: &executor.AgentProfileInfo{
		ProfileID: "caller-profile", AgentID: "codex", AgentName: "codex", SupportsMCP: true,
	}}
	launchCalls := 0
	launchPrompt := ""
	agentManager.launchAgentFunc = func(_ context.Context, request *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
		launchCalls++
		launchPrompt = request.TaskDescription
		return &executor.LaunchAgentResponse{AgentExecutionID: "exec-1", Status: v1.AgentStatusStarting}, nil
	}
	service := createTestServiceWithScheduler(repo, stepGetter, taskRepo, agentManager)
	messages := &mockMessageCreator{}
	service.messageCreator = messages
	lookupErr := errors.New("temporary workflow step lookup failure")
	failingRepo := &currentStepPromptLookupFailureRepository{
		Repository: repo, failOnRead: 4, err: lookupErr,
	}
	service.repo = failingRepo

	_, err := service.startTask(ctx, "task1", "caller-profile", "", "", "", currentTaskPromptTestInput, "", false, false, nil, startTaskOptions{})

	require.NoError(t, err, "the existing start behavior tolerates a failed omitted-step lookup")
	require.Equal(t, 4, failingRepo.failedAtRead, "the current-step lookup fails after prior task reads succeed")
	require.True(t, failingRepo.failedRead)
	require.Equal(t, 1, launchCalls, "the failed lookup retains existing direct-prompt launch behavior")
	require.Len(t, messages.userMessages, 1)
	assert.Contains(t, messages.userMessages[0].content, currentTaskPromptTestInput)
	assert.NotContains(t, messages.userMessages[0].content, currentStepPromptTestMarker)
	assert.Equal(t, messages.userMessages[0].content, launchPrompt, "stored and dispatched direct prompts must match")
	sessions, listErr := repo.ListTaskSessions(ctx, "task1")
	require.NoError(t, listErr)
	assert.Len(t, sessions, 1)
}

func runCurrentStepPromptCase(t *testing.T, tc currentStepPromptCase) {
	t.Helper()
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskWithoutSession(t, repo, "task1", currentStepPromptTestID)
	if tc.wantCurrentTaskStepID == "" {
		task, err := repo.GetTask(ctx, "task1")
		require.NoError(t, err)
		task.WorkflowStepID = ""
		require.NoError(t, repo.UpdateTask(ctx, task))
	}
	dbTask, err := repo.GetTask(ctx, "task1")
	require.NoError(t, err)
	dbTask.Description = currentTaskPromptTestDesc
	dbTask.IsEphemeral = tc.isEphemeral
	if tc.isOffice {
		dbTask.ProjectID = "office-project"
	}
	require.NoError(t, repo.UpdateTask(ctx, dbTask))

	stepGetter := newMockStepGetter()
	stepGetter.workflowPrompts["wf1"] = tc.workflowPrompt
	if tc.wantCurrentTaskStepID != "" {
		stepGetter.steps[currentStepPromptTestID] = currentStepPromptTestStep(currentStepPromptTestID, tc.currentTemplate, tc.currentStepProfileID, true, tc.stepEnablesPlanMode)
	}
	if tc.requestStepID == otherStepPromptTestID {
		stepGetter.steps[otherStepPromptTestID] = currentStepPromptTestStep(otherStepPromptTestID, tc.otherTemplate, tc.otherStepProfileID, false, false)
	}

	callerProfileID := tc.callerProfileID
	if callerProfileID == "" {
		callerProfileID = "caller-profile"
	}
	wantProfileID := tc.wantProfileID
	if wantProfileID == "" {
		wantProfileID = callerProfileID
	}
	agentManager := &mockAgentManager{
		resolveProfileInfo: &executor.AgentProfileInfo{
			ProfileID:   wantProfileID,
			AgentID:     "codex",
			AgentName:   "codex",
			SupportsMCP: true,
		},
	}
	var capture currentStepPromptLaunchCapture
	agentManager.launchAgentFunc = func(launchCtx context.Context, request *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
		capture.calls++
		capture.prompt = request.TaskDescription
		capture.profileID = request.AgentProfileID
		capture.sessionID = request.SessionID
		launchedSession, sessionErr := repo.GetTaskSession(launchCtx, request.SessionID)
		capture.sessionErr = sessionErr
		if sessionErr == nil {
			capture.runtimeConfig, capture.hasRuntimeConfig = models.LoadSessionRuntimeConfigOverrides(launchedSession.Metadata)
		}
		return &executor.LaunchAgentResponse{AgentExecutionID: "exec-1", Status: v1.AgentStatusStarting}, nil
	}
	taskRepo := newMockTaskRepo()
	taskRepo.tasks["task1"] = &v1.Task{
		ID: "task1", WorkspaceID: "ws1", WorkflowID: "wf1", Title: "Test task",
		Description: currentTaskPromptTestDesc, State: v1.TaskStateInProgress, IsEphemeral: tc.isEphemeral,
	}
	service := createTestServiceWithScheduler(repo, stepGetter, taskRepo, agentManager)
	service.promptExpander = &fakePromptReferenceExpander{}
	messages := &mockMessageCreator{}
	service.messageCreator = messages

	options := startTaskOptions{
		ProfileExplicit:               tc.profileExplicit,
		PreserveWorkflowSourceBinding: tc.preserveWorkflowBinding,
		EntryOptions:                  tc.entryOptions,
	}
	if tc.isOffice {
		options.Env = validOfficeRuntimeEnv()
		options.Env["KANDEV_TASK_ID"] = "task1"
	}
	_, err = service.startTask(ctx, "task1", callerProfileID, "", "", "", tc.callerPrompt, tc.requestStepID, false, false, nil, options)
	require.NoError(t, err)

	require.Equal(t, 1, capture.calls, "the start must launch one agent and must not repeat the step entry action")
	require.NoError(t, capture.sessionErr)
	require.Equal(t, wantProfileID, capture.profileID)
	require.Len(t, messages.userMessages, 1)
	storedPrompt := messages.userMessages[0].content
	for _, want := range tc.wantContains {
		assert.Contains(t, storedPrompt, want)
		assert.Contains(t, capture.prompt, want)
	}
	for _, absent := range tc.wantAbsent {
		assert.NotContains(t, storedPrompt, absent)
		assert.NotContains(t, capture.prompt, absent)
	}
	assert.Equal(t, storedPrompt, capture.prompt, "the stored first prompt and launch prompt must match")
	if tc.wantSavedReference {
		assert.Equal(t, 1, strings.Count(storedPrompt, currentPromptReferenceResult))
		assert.Equal(t, 1, strings.Count(capture.prompt, currentPromptReferenceResult))
	}
	if tc.wantRuntimeConfig {
		require.True(t, capture.hasRuntimeConfig)
		assert.Equal(t, "gpt-5.6-luna", capture.runtimeConfig.Model)
		assert.Equal(t, "max", capture.runtimeConfig.ConfigOptions["reasoning_effort"])
	}
	if tc.wantPlanMode {
		session, sessionErr := repo.GetTaskSession(ctx, capture.sessionID)
		require.NoError(t, sessionErr)
		assert.Equal(t, true, session.Metadata["plan_mode"])
	}
	task, err := repo.GetTask(ctx, "task1")
	require.NoError(t, err)
	assert.Equal(t, tc.wantCurrentTaskStepID, task.WorkflowStepID)
}

func currentStepPromptTestStep(id, prompt, profileID string, configure bool, planMode bool) *wfmodels.WorkflowStep {
	step := &wfmodels.WorkflowStep{
		ID: id, WorkflowID: "wf1", Prompt: prompt, AgentProfileID: profileID,
		Events: wfmodels.StepEvents{OnEnter: []wfmodels.OnEnterAction{{Type: wfmodels.OnEnterAutoStartAgent}}},
	}
	if configure {
		configuration := configureSessionStep(currentStepPromptTestID, "codex", "set", "gpt-5.6-luna", map[string]string{"reasoning_effort": "max"})
		step.Events.OnEnter = append(step.Events.OnEnter, configuration.Events.OnEnter...)
	}
	if planMode {
		step.Events.OnEnter = append(step.Events.OnEnter, wfmodels.OnEnterAction{Type: wfmodels.OnEnterEnablePlanMode})
	}
	return step
}

type currentStepPromptLookupFailureRepository struct {
	*sqliterepo.Repository
	taskReads    int
	failOnRead   int
	failedAtRead int
	failedRead   bool
	err          error
}

func (r *currentStepPromptLookupFailureRepository) GetTask(ctx context.Context, taskID string) (*models.Task, error) {
	r.taskReads++
	if r.taskReads == r.failOnRead {
		r.failedRead = true
		r.failedAtRead = r.taskReads
		return nil, r.err
	}
	return r.Repository.GetTask(ctx, taskID)
}

type currentStepPromptLaunchCapture struct {
	calls            int
	prompt           string
	profileID        string
	sessionID        string
	runtimeConfig    models.SessionRuntimeConfig
	hasRuntimeConfig bool
	sessionErr       error
}
