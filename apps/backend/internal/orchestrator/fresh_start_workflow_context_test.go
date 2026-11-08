package orchestrator

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/sysprompt"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
)

// @covers AC-TASKS-PROMPT-ATTACHMENTS-002.1
// @covers AC-TASKS-PROMPT-ATTACHMENTS-002.3
// @covers AC-TASKS-PROMPT-ATTACHMENTS-002.5
// @covers AC-TASKS-PROMPT-ATTACHMENTS-002.6
// @covers AC-TASKS-PROMPT-ATTACHMENTS-002.7
// @covers AC-TASKS-PROMPT-ATTACHMENTS-002.9
// @covers AC-TASKS-PROMPT-ATTACHMENTS-002.10
// @covers AC-TASKS-PROMPT-ATTACHMENTS-002.11
func TestFreshStartWorkflowContext(t *testing.T) {
	const (
		workflowConstraint = "Only report a GitHub issue. Do not implement."
		stepPrefix         = "Step constraint:"
		submissionText     = "Report the requested change"
	)

	t.Run("receipt replay includes current workflow instructions before task input", func(t *testing.T) {
		fixture := newFreshStartWorkflowFixture(t, freshStartWorkflowOptions{
			withReceipt:    true,
			receiptContent: submissionText,
			description:    "unused task description",
			workflowPrompt: workflowConstraint,
			stepPrompt:     stepPrefix + " {{task_prompt}}",
		})

		observation := fixture.recover(t, "fresh_start")

		require.NoError(t, observation.recoveryErr)
		require.Empty(t, observation.launchDescription)
		require.Len(t, observation.capturedPrompts, 1)
		prompt := observation.capturedPrompts[0]
		require.Equal(t, 1, strings.Count(prompt, workflowConstraint))
		require.Equal(t, 1, strings.Count(prompt, stepPrefix))
		require.Equal(t, 1, strings.Count(prompt, submissionText))
		require.Less(t, strings.Index(prompt, workflowConstraint), strings.Index(prompt, stepPrefix))
		require.Less(t, strings.Index(prompt, stepPrefix), strings.Index(prompt, submissionText))
		require.Equal(t, models.InitialPromptSubmissionAccepted, observation.receipt.State)
		require.Equal(t, submissionText, observation.receipt.Content, "the raw receipt must not be rewritten")
	})

	t.Run("legacy text fallback composes workflow instructions into the launch", func(t *testing.T) {
		fixture := newFreshStartWorkflowFixture(t, freshStartWorkflowOptions{
			description:    submissionText,
			workflowPrompt: workflowConstraint,
			stepPrompt:     stepPrefix + " {{task_prompt}}",
		})

		observation := fixture.recover(t, "fresh_start")

		require.NoError(t, observation.recoveryErr)
		require.Empty(t, observation.capturedPrompts, "legacy recovery dispatches at launch, not after readiness")
		require.Equal(t, 1, observation.launchCount)
		require.Equal(t, 1, strings.Count(observation.launchDescription, workflowConstraint))
		require.Equal(t, 1, strings.Count(observation.launchDescription, stepPrefix))
		require.Equal(t, 1, strings.Count(observation.launchDescription, submissionText))
		require.Less(t, strings.Index(observation.launchDescription, workflowConstraint), strings.Index(observation.launchDescription, submissionText))
	})

	t.Run("saved prompt uses the current definition instead of stored context", func(t *testing.T) {
		fixture := newFreshStartWorkflowFixture(t, freshStartWorkflowOptions{
			withReceipt:           true,
			receiptContent:        submissionText,
			description:           "unused task description",
			stepPrompt:            "Follow @principles.",
			savedPromptDefinition: "Current saved instruction.",
			stalePromptContext:    "Stale saved instruction.",
		})

		observation := fixture.recover(t, "fresh_start")

		require.NoError(t, observation.recoveryErr)
		require.Len(t, observation.capturedPrompts, 1)
		prompt := observation.capturedPrompts[0]
		require.Contains(t, prompt, "Current saved instruction.")
		require.NotContains(t, prompt, "Stale saved instruction.")
		require.Equal(t, 1, strings.Count(prompt, "Current saved instruction."))
	})

	t.Run("legacy saved prompt uses the current definition", func(t *testing.T) {
		fixture := newFreshStartWorkflowFixture(t, freshStartWorkflowOptions{
			description:           submissionText,
			stepPrompt:            "Follow @principles. {{task_prompt}}",
			savedPromptDefinition: "Current legacy instruction.",
			stalePromptContext:    "Stale legacy instruction.",
		})

		observation := fixture.recover(t, "fresh_start")

		require.NoError(t, observation.recoveryErr)
		require.Equal(t, 1, observation.launchCount)
		require.Contains(t, observation.launchDescription, "Current legacy instruction.")
		require.NotContains(t, observation.launchDescription, "Stale legacy instruction.")
		require.Equal(t, 1, strings.Count(observation.launchDescription, "Current legacy instruction."))
	})

	t.Run("passthrough receives visible workflow text without hidden expansion", func(t *testing.T) {
		fixture := newFreshStartWorkflowFixture(t, freshStartWorkflowOptions{
			withReceipt:           true,
			receiptContent:        submissionText,
			description:           "unused task description",
			stepPrompt:            "Follow @principles. {{task_prompt}}",
			savedPromptDefinition: "Hidden saved instruction.",
			passthrough:           true,
		})

		observation := fixture.recover(t, "fresh_start")

		require.NoError(t, observation.recoveryErr)
		require.Len(t, observation.capturedPrompts, 1)
		prompt := observation.capturedPrompts[0]
		require.Contains(t, prompt, "Follow @principles.")
		require.Contains(t, prompt, submissionText)
		require.NotContains(t, prompt, "Hidden saved instruction.")
		require.NotContains(t, prompt, sysprompt.TagStart)
	})

	t.Run("replacing step template keeps its first-launch replacement rule", func(t *testing.T) {
		fixture := newFreshStartWorkflowFixture(t, freshStartWorkflowOptions{
			withReceipt:    true,
			receiptContent: submissionText,
			description:    "unused task description",
			workflowPrompt: workflowConstraint,
			stepPrompt:     "Step replaces the submitted text.",
		})

		observation := fixture.recover(t, "fresh_start")

		require.NoError(t, observation.recoveryErr)
		require.Len(t, observation.capturedPrompts, 1)
		prompt := observation.capturedPrompts[0]
		require.Contains(t, prompt, workflowConstraint)
		require.Contains(t, prompt, "Step replaces the submitted text.")
		require.NotContains(t, prompt, submissionText)
	})

	t.Run("empty step prompt retains workflow and submitted input", func(t *testing.T) {
		fixture := newFreshStartWorkflowFixture(t, freshStartWorkflowOptions{
			withReceipt:    true,
			receiptContent: submissionText,
			description:    "unused task description",
			workflowPrompt: workflowConstraint,
		})

		observation := fixture.recover(t, "fresh_start")

		require.NoError(t, observation.recoveryErr)
		require.Len(t, observation.capturedPrompts, 1)
		prompt := observation.capturedPrompts[0]
		require.Equal(t, 1, strings.Count(prompt, workflowConstraint))
		require.Equal(t, 1, strings.Count(prompt, submissionText))
	})

	t.Run("step plan mode applies once to receipt recovery", func(t *testing.T) {
		fixture := newFreshStartWorkflowFixture(t, freshStartWorkflowOptions{
			withReceipt:    true,
			receiptContent: submissionText,
			description:    "unused task description",
			workflowPrompt: workflowConstraint,
			stepPrompt:     stepPrefix + " {{task_prompt}}",
			stepEvents: wfmodels.StepEvents{OnEnter: []wfmodels.OnEnterAction{{
				Type: wfmodels.OnEnterEnablePlanMode,
			}}},
		})

		observation := fixture.recover(t, "fresh_start")

		require.NoError(t, observation.recoveryErr)
		require.Len(t, observation.capturedPrompts, 1)
		prompt := observation.capturedPrompts[0]
		require.Equal(t, 1, strings.Count(prompt, sysprompt.PlanMode()))
		require.Equal(t, 1, strings.Count(prompt, workflowConstraint))
		require.Equal(t, 1, strings.Count(prompt, stepPrefix))
		require.False(t, observation.receipt.PlanMode, "raw receipt plan mode must remain unchanged")
	})

	t.Run("step plan mode applies once to legacy recovery", func(t *testing.T) {
		fixture := newFreshStartWorkflowFixture(t, freshStartWorkflowOptions{
			description:    submissionText,
			workflowPrompt: workflowConstraint,
			stepPrompt:     stepPrefix + " {{task_prompt}}",
			stepEvents: wfmodels.StepEvents{OnEnter: []wfmodels.OnEnterAction{{
				Type: wfmodels.OnEnterEnablePlanMode,
			}}},
		})

		observation := fixture.recover(t, "fresh_start")

		require.NoError(t, observation.recoveryErr)
		require.Equal(t, 1, observation.launchCount)
		require.Equal(t, 1, strings.Count(observation.launchDescription, sysprompt.PlanMode()))
		require.Equal(t, 1, strings.Count(observation.launchDescription, workflowConstraint))
		require.Equal(t, 1, strings.Count(observation.launchDescription, stepPrefix))
	})

	t.Run("configuration context and workflow text appear once", func(t *testing.T) {
		fixture := newFreshStartWorkflowFixture(t, freshStartWorkflowOptions{
			withReceipt:    true,
			receiptContent: submissionText,
			description:    "unused task description",
			workflowPrompt: workflowConstraint,
			stepPrompt:     stepPrefix + " {{task_prompt}}",
			configMode:     true,
		})

		observation := fixture.recover(t, "fresh_start")

		require.NoError(t, observation.recoveryErr)
		require.Len(t, observation.capturedPrompts, 1)
		prompt := observation.capturedPrompts[0]
		require.Equal(t, 1, strings.Count(prompt, sysprompt.FormatConfigContext(fixture.sessionID)))
		require.Equal(t, 1, strings.Count(prompt, workflowConstraint))
		require.Equal(t, 1, strings.Count(prompt, submissionText))
	})

	t.Run("missing step policy fails before reset or dispatch", func(t *testing.T) {
		fixture := newFreshStartWorkflowFixture(t, freshStartWorkflowOptions{
			withReceipt:    true,
			receiptContent: submissionText,
			description:    "unused task description",
			stepReadErr:    errors.New("workflow step unavailable"),
			providerToken:  "fresh-start-provider-session",
		})

		observation := fixture.recover(t, "fresh_start")

		require.Error(t, observation.recoveryErr)
		require.Zero(t, observation.launchCount)
		require.Empty(t, observation.capturedPrompts)
		require.Equal(t, models.TaskSessionStateFailed, observation.session.State)
		require.Equal(t, "fresh-start-workflow-previous-execution", observation.session.AgentExecutionID)
		require.Equal(t, "fresh-start-provider-session", observation.session.DownstreamACPSessionID)
		require.Equal(t, models.InitialPromptSubmissionPending, observation.receipt.State)
	})

	t.Run("legacy missing step policy fails before reset or dispatch", func(t *testing.T) {
		fixture := newFreshStartWorkflowFixture(t, freshStartWorkflowOptions{
			description: submissionText,
			stepReadErr: errors.New("workflow step unavailable"),
		})

		observation := fixture.recover(t, "fresh_start")

		require.Error(t, observation.recoveryErr)
		require.Zero(t, observation.launchCount)
		require.Empty(t, observation.capturedPrompts)
		require.Equal(t, models.TaskSessionStateFailed, observation.session.State)
		require.Equal(t, "fresh-start-workflow-previous-execution", observation.session.AgentExecutionID)
	})

	t.Run("failed workflow policy read fails before reset or dispatch", func(t *testing.T) {
		fixture := newFreshStartWorkflowFixture(t, freshStartWorkflowOptions{
			withReceipt:     true,
			receiptContent:  submissionText,
			description:     "unused task description",
			stepPrompt:      stepPrefix + " {{task_prompt}}",
			workflowReadErr: errors.New("workflow policy unavailable"),
			providerToken:   "fresh-start-provider-session",
		})

		observation := fixture.recover(t, "fresh_start")

		require.Error(t, observation.recoveryErr)
		require.Zero(t, observation.launchCount)
		require.Empty(t, observation.capturedPrompts)
		require.Equal(t, models.TaskSessionStateFailed, observation.session.State)
		require.Equal(t, "fresh-start-workflow-previous-execution", observation.session.AgentExecutionID)
		require.Equal(t, "fresh-start-provider-session", observation.session.DownstreamACPSessionID)
		require.Equal(t, models.InitialPromptSubmissionPending, observation.receipt.State)
	})

	t.Run("accepted receipt does not select the legacy workflow fallback", func(t *testing.T) {
		fixture := newFreshStartWorkflowFixture(t, freshStartWorkflowOptions{
			acceptedReceipt: true,
			receiptContent:  "accepted request",
			description:     submissionText,
			workflowPrompt:  workflowConstraint,
			stepPrompt:      stepPrefix + " {{task_prompt}}",
		})

		observation := fixture.recover(t, "fresh_start")

		require.NoError(t, observation.recoveryErr)
		require.Equal(t, 1, observation.launchCount)
		require.Equal(t, submissionText, observation.launchDescription)
		require.NotContains(t, observation.launchDescription, workflowConstraint)
		require.NotContains(t, observation.launchDescription, stepPrefix)
		require.Empty(t, observation.capturedPrompts)
	})

	t.Run("session provider token prevents the legacy workflow fallback", func(t *testing.T) {
		fixture := newFreshStartWorkflowFixture(t, freshStartWorkflowOptions{
			description:    submissionText,
			workflowPrompt: workflowConstraint,
			stepPrompt:     stepPrefix + " {{task_prompt}}",
			providerToken:  "existing-provider-conversation",
		})

		observation := fixture.recover(t, "fresh_start")

		require.NoError(t, observation.recoveryErr)
		require.Equal(t, 1, observation.launchCount)
		require.Equal(t, submissionText, observation.launchDescription)
		require.NotContains(t, observation.launchDescription, workflowConstraint)
		require.NotContains(t, observation.launchDescription, stepPrefix)
	})

	t.Run("runtime provider token prevents the legacy workflow fallback", func(t *testing.T) {
		fixture := newFreshStartWorkflowFixture(t, freshStartWorkflowOptions{
			description:    submissionText,
			workflowPrompt: workflowConstraint,
			stepPrompt:     stepPrefix + " {{task_prompt}}",
			runningToken:   "existing-provider-runtime",
		})

		observation := fixture.recover(t, "fresh_start")

		require.NoError(t, observation.recoveryErr)
		require.Equal(t, 1, observation.launchCount)
		require.Equal(t, submissionText, observation.launchDescription)
		require.NotContains(t, observation.launchDescription, workflowConstraint)
		require.NotContains(t, observation.launchDescription, stepPrefix)
	})

	t.Run("provider identity read failure stops before reset", func(t *testing.T) {
		fixture := newFreshStartWorkflowFixture(t, freshStartWorkflowOptions{
			description:    submissionText,
			workflowPrompt: workflowConstraint,
			stepPrompt:     stepPrefix + " {{task_prompt}}",
		})
		fixture.svc.repo = &freshStartWorkflowProviderReadErrorRepo{
			Repository: fixture.repo,
			err:        errors.New("provider identity unavailable"),
		}

		observation := fixture.recover(t, "fresh_start")

		require.Error(t, observation.recoveryErr)
		require.Zero(t, observation.launchCount)
		require.Empty(t, observation.capturedPrompts)
		require.Equal(t, models.TaskSessionStateFailed, observation.session.State)
		require.Equal(t, "fresh-start-workflow-previous-execution", observation.session.AgentExecutionID)
		task, taskErr := fixture.repo.GetTask(fixture.ctx, fixture.taskID)
		require.NoError(t, taskErr)
		require.Equal(t, submissionText, task.Description)
	})

	t.Run("queue ownership read failure stops before reset", func(t *testing.T) {
		fixture := newFreshStartWorkflowFixture(t, freshStartWorkflowOptions{
			description:    submissionText,
			workflowPrompt: workflowConstraint,
			stepPrompt:     stepPrefix + " {{task_prompt}}",
		})
		fixture.svc.messageQueue = messagequeue.NewService(
			&freshStartWorkflowQueueReadErrorRepository{
				Repository: newAuthoritativeMemoryRepository(fixture.repo),
				err:        errors.New("queue storage unavailable"),
			},
			messagequeue.DefaultMaxPerSession,
			testLogger(),
		)

		observation := fixture.recover(t, "fresh_start")

		require.Error(t, observation.recoveryErr)
		require.Zero(t, observation.launchCount)
		require.Empty(t, observation.capturedPrompts)
		require.Equal(t, models.TaskSessionStateFailed, observation.session.State)
		require.Equal(t, "fresh-start-workflow-previous-execution", observation.session.AgentExecutionID)
		task, taskErr := fixture.repo.GetTask(fixture.ctx, fixture.taskID)
		require.NoError(t, taskErr)
		require.Equal(t, submissionText, task.Description)
	})

	for _, withReceipt := range []bool{true, false} {
		withReceipt := withReceipt
		sourceName := "legacy"
		if withReceipt {
			sourceName = "receipt"
		}
		t.Run("moved workflow entry supersedes prepared "+sourceName+" instructions", func(t *testing.T) {
			const destinationStepID = "fresh-start-workflow-step-b"
			fixture := newFreshStartWorkflowFixture(t, freshStartWorkflowOptions{
				withReceipt:    withReceipt,
				receiptContent: submissionText,
				description:    submissionText,
				stepPrompt:     "Prepared step instructions: {{task_prompt}}",
			})
			fixture.stepGetter.steps[destinationStepID] = &wfmodels.WorkflowStep{
				ID: destinationStepID, WorkflowID: "wf1", Name: "Destination",
				Prompt: "Destination step instructions: {{task_prompt}}",
			}
			barrier := &freshStartWorkflowResetBarrierRepo{
				Repository: fixture.repo,
				entered:    make(chan struct{}),
				release:    make(chan struct{}),
			}
			fixture.svc.repo = barrier

			done := make(chan error, 1)
			go func() {
				_, err := fixture.svc.RecoverSessionWithOptions(
					fixture.ctx, fixture.taskID, fixture.sessionID, "fresh_start", RecoverSessionOptions{},
				)
				done <- err
			}()
			select {
			case <-barrier.entered:
			case <-time.After(3 * time.Second):
				t.Fatal("timed out waiting for recovery to pause before reset")
			}

			task, err := fixture.repo.GetTask(fixture.ctx, fixture.taskID)
			require.NoError(t, err)
			task.WorkflowStepID = destinationStepID
			require.NoError(t, fixture.repo.UpdateTask(fixture.ctx, task))
			task, err = fixture.repo.GetTask(fixture.ctx, fixture.taskID)
			require.NoError(t, err)
			task.WorkflowStepID = fixture.stepID
			require.NoError(t, fixture.repo.UpdateTask(fixture.ctx, task))
			close(barrier.release)

			select {
			case err := <-done:
				require.ErrorIs(t, err, ErrCeilingLaunchSuperseded)
			case <-fixture.started:
				t.Fatal("superseded workflow entry still launched the agent")
			case <-time.After(3 * time.Second):
				t.Fatal("timed out waiting for superseded recovery to settle")
			}
			fixture.mu.Lock()
			require.Zero(t, len(fixture.launches))
			fixture.mu.Unlock()
			fixture.manager.mu.Lock()
			require.Empty(t, fixture.manager.capturedPrompts)
			fixture.manager.mu.Unlock()
			stored, loadErr := fixture.repo.GetTaskSession(fixture.ctx, fixture.sessionID)
			require.NoError(t, loadErr)
			require.Equal(t, models.TaskSessionStateFailed, stored.State)
			if withReceipt {
				receipt, found, receiptErr := models.LoadInitialPromptSubmission(stored.Metadata)
				require.NoError(t, receiptErr)
				require.True(t, found)
				require.Equal(t, models.InitialPromptSubmissionPending, receipt.State)
			}
		})
	}

	t.Run("empty legacy fallback does not create input from a step template", func(t *testing.T) {
		fixture := newFreshStartWorkflowFixture(t, freshStartWorkflowOptions{
			stepPrompt: stepPrefix + " {{task_prompt}}",
		})

		observation := fixture.recover(t, "fresh_start")

		require.NoError(t, observation.recoveryErr)
		require.Equal(t, 1, observation.launchCount)
		require.Empty(t, observation.launchDescription)
		require.Empty(t, observation.capturedPrompts)
	})

	t.Run("ordinary resume does not compose the initial workflow fallback", func(t *testing.T) {
		fixture := newFreshStartWorkflowFixture(t, freshStartWorkflowOptions{
			description:    submissionText,
			workflowPrompt: workflowConstraint,
			stepPrompt:     stepPrefix + " {{task_prompt}}",
		})

		observation := fixture.recover(t, "resume")

		require.NoError(t, observation.recoveryErr)
		require.Equal(t, submissionText, observation.launchDescription)
		require.NotContains(t, observation.launchDescription, workflowConstraint)
		require.NotContains(t, observation.launchDescription, stepPrefix)
	})
}

type freshStartWorkflowOptions struct {
	withReceipt           bool
	acceptedReceipt       bool
	receiptContent        string
	receiptPlanMode       bool
	description           string
	workflowPrompt        string
	stepPrompt            string
	stepEvents            wfmodels.StepEvents
	stepReadErr           error
	workflowReadErr       error
	passthrough           bool
	configMode            bool
	providerToken         string
	runningToken          string
	savedPromptDefinition string
	stalePromptContext    string
}

type freshStartWorkflowObservation struct {
	launchCount       int
	launchDescription string
	capturedPrompts   []string
	recoveryErr       error
	session           *models.TaskSession
	receipt           *models.InitialPromptSubmission
}

type freshStartWorkflowFixture struct {
	ctx         context.Context
	repo        *sqliterepo.Repository
	svc         *Service
	manager     *mockAgentManager
	stepGetter  *mockStepGetter
	taskID      string
	sessionID   string
	stepID      string
	started     chan struct{}
	startOnce   sync.Once
	processed   chan struct{}
	processOnce sync.Once
	done        chan error
	mu          sync.Mutex
	launches    []string
}

func newFreshStartWorkflowFixture(t *testing.T, options freshStartWorkflowOptions) *freshStartWorkflowFixture {
	t.Helper()
	const (
		taskID    = "fresh-start-workflow-task"
		sessionID = "fresh-start-workflow-session"
		stepID    = "fresh-start-workflow-step"
	)
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateFailed)

	task, err := repo.GetTask(ctx, taskID)
	require.NoError(t, err)
	task.WorkflowStepID = stepID
	task.Description = options.description
	require.NoError(t, repo.UpdateTask(ctx, task))

	session, err := repo.GetTaskSession(ctx, sessionID)
	require.NoError(t, err)
	session.AgentProfileID = "fresh-start-workflow-profile"
	session.TaskEnvironmentID = "fresh-start-workflow-environment"
	session.IsPassthrough = options.passthrough
	session.DownstreamACPSessionID = options.providerToken
	require.NoError(t, repo.UpdateTaskSession(ctx, session))
	sessionMetadata := map[string]interface{}{}
	if options.configMode {
		sessionMetadata["config_mode"] = true
	}
	if options.stalePromptContext != "" {
		sessionMetadata["prompt_reference_context"] = options.stalePromptContext
	}
	if len(sessionMetadata) > 0 {
		require.NoError(t, repo.UpdateSessionMetadata(ctx, sessionID, sessionMetadata))
	}
	require.NoError(t, repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{
		ID: session.TaskEnvironmentID, TaskID: taskID,
		ExecutorType: string(models.ExecutorTypeLocal), Status: models.TaskEnvironmentStatusReady,
	}))
	seedExecutorRunning(t, repo, sessionID, taskID, "fresh-start-workflow-previous-execution")
	if options.runningToken != "" {
		running, runningErr := repo.GetExecutorRunningBySessionID(ctx, sessionID)
		require.NoError(t, runningErr)
		running.ResumeToken = options.runningToken
		require.NoError(t, repo.UpsertExecutorRunning(ctx, running))
	}
	require.NoError(t, repo.SetSessionMetadataKey(ctx, sessionID, models.SessionMetaKeyLastAgentError, models.LastAgentError{
		Message: "agent startup failed", Phase: models.LaunchErrorPhaseBootstrap,
		StampValue: "fresh-start-workflow-error",
	}))

	if options.withReceipt || options.acceptedReceipt {
		submission, submissionErr := models.NewInitialPromptSubmission(
			options.receiptContent, options.receiptPlanMode, nil,
		)
		require.NoError(t, submissionErr)
		if options.acceptedReceipt {
			submission.State = models.InitialPromptSubmissionAccepted
			submission.ExecutionID = "fresh-start-workflow-accepted-execution"
			submission.AcceptedAt = time.Now().UTC().Format(time.RFC3339Nano)
		}
		require.NoError(t, repo.SetSessionMetadataKey(
			ctx, sessionID, models.SessionMetaKeyInitialPromptSubmission, submission,
		))
	}

	stepGetter := newMockStepGetter()
	if options.stepReadErr != nil {
		stepGetter.getStepFunc = func(context.Context, string) (*wfmodels.WorkflowStep, error) {
			return nil, options.stepReadErr
		}
	} else {
		stepGetter.steps[stepID] = &wfmodels.WorkflowStep{
			ID: stepID, WorkflowID: "wf1", Name: "Fresh start workflow",
			Prompt: options.stepPrompt, Events: options.stepEvents,
		}
	}
	if options.workflowPrompt != "" {
		stepGetter.workflowPrompts["wf1"] = options.workflowPrompt
	}
	if options.workflowReadErr != nil {
		stepGetter.workflowMetaErr = options.workflowReadErr
	}

	taskRepo := newMockTaskRepo()
	taskRepo.tasks[taskID] = &v1.Task{
		ID: taskID, WorkspaceID: "ws1", WorkflowID: "wf1", Title: "Fresh start workflow task",
		Description: options.description, State: v1.TaskStateInProgress,
	}

	started := make(chan struct{})
	processed := make(chan struct{})
	fixture := &freshStartWorkflowFixture{
		ctx: ctx, repo: repo,
		taskID: taskID, sessionID: sessionID, stepID: stepID,
		stepGetter: stepGetter,
		started:    started, processed: processed,
	}
	manager := &mockAgentManager{
		repoForExecutionLookup: repo,
		isAgentRunning:         true,
		isAgentReadyFn:         func(context.Context, string) bool { return true },
		launchAgentFunc: func(_ context.Context, req *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
			fixture.mu.Lock()
			fixture.launches = append(fixture.launches, req.TaskDescription)
			fixture.mu.Unlock()
			return &executor.LaunchAgentResponse{AgentExecutionID: "fresh-start-workflow-execution"}, nil
		},
		promptAgentFunc: func(context.Context, string, string, []v1.MessageAttachment, bool) (*executor.PromptResult, error) {
			return &executor.PromptResult{}, nil
		},
	}
	fixture.manager = manager
	manager.startAgentProcessFunc = func(context.Context, string) error {
		fixture.startOnce.Do(func() { close(started) })
		return nil
	}
	svc := createTestServiceWithScheduler(repo, stepGetter, taskRepo, manager)
	fixture.svc = svc
	if options.savedPromptDefinition != "" {
		promptService := newPromptServiceForLaunchFallbackTest(t)
		_, promptErr := promptService.CreatePrompt(ctx, "principles", options.savedPromptDefinition)
		require.NoError(t, promptErr)
		svc.promptExpander = promptService
	}
	svc.messageCreator = &mockMessageCreator{}
	svc.turnService = &repoTurnService{repo: repo}
	svc.executor.SetOnAgentProcessStarted(func(callbackCtx context.Context, callbackTaskID, callbackSessionID, callbackExecutionID string) {
		svc.handleAgentProcessStarted(callbackCtx, callbackTaskID, callbackSessionID, callbackExecutionID)
		fixture.processOnce.Do(func() { close(processed) })
	})
	svc.executor.SetOnAgentProcessStartFailed(svc.handleAgentProcessStartFailed)
	return fixture
}

func (f *freshStartWorkflowFixture) recover(t *testing.T, action string) freshStartWorkflowObservation {
	t.Helper()
	f.done = make(chan error, 1)
	go func() {
		_, err := f.svc.RecoverSessionWithOptions(f.ctx, f.taskID, f.sessionID, action, RecoverSessionOptions{})
		f.done <- err
	}()

	var recoveryErr error
	select {
	case recoveryErr = <-f.done:
	case <-f.started:
		attempt, ok := f.svc.resumeAttemptStore().current(f.sessionID)
		require.True(t, ok)
		require.Eventually(t, func() bool { return attempt.execution() != "" }, 3*time.Second, 10*time.Millisecond)
		if action != "resume" {
			select {
			case <-f.processed:
			case <-time.After(3 * time.Second):
				t.Fatal("timed out waiting for provider process startup")
			}
		}
		f.svc.handleAgentBootReady(f.ctx, watcher.AgentEventData{
			TaskID: f.taskID, SessionID: f.sessionID,
			AgentExecutionID: "fresh-start-workflow-execution", AttemptID: attempt.identity(),
		})
		select {
		case recoveryErr = <-f.done:
		case <-time.After(3 * time.Second):
			t.Fatal("timed out waiting for fresh recovery")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for fresh recovery launch")
	}

	observation := freshStartWorkflowObservation{recoveryErr: recoveryErr}
	f.mu.Lock()
	observation.launchCount = len(f.launches)
	if len(f.launches) > 0 {
		observation.launchDescription = f.launches[0]
	}
	f.mu.Unlock()
	f.manager.mu.Lock()
	observation.capturedPrompts = append([]string(nil), f.manager.capturedPrompts...)
	f.manager.mu.Unlock()

	stored, loadErr := f.repo.GetTaskSession(f.ctx, f.sessionID)
	require.NoError(t, loadErr)
	observation.session = stored
	if receipt, found, receiptErr := models.LoadInitialPromptSubmission(stored.Metadata); receiptErr == nil && found {
		observation.receipt = receipt
	}
	return observation
}

type freshStartWorkflowResetBarrierRepo struct {
	*sqliterepo.Repository
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func (r *freshStartWorkflowResetBarrierRepo) UpdateResumeToken(
	ctx context.Context, sessionID, expectedExecID, resumeToken, lastMessageUUID string,
) error {
	r.once.Do(func() {
		close(r.entered)
		<-r.release
	})
	return r.Repository.UpdateResumeToken(ctx, sessionID, expectedExecID, resumeToken, lastMessageUUID)
}

type freshStartWorkflowProviderReadErrorRepo struct {
	*sqliterepo.Repository
	err   error
	calls atomic.Int32
}

func (r *freshStartWorkflowProviderReadErrorRepo) GetExecutorRunningBySessionID(
	ctx context.Context, sessionID string,
) (*models.ExecutorRunning, error) {
	if r.calls.Add(1) == 1 {
		return nil, r.err
	}
	return r.Repository.GetExecutorRunningBySessionID(ctx, sessionID)
}

type freshStartWorkflowQueueReadErrorRepository struct {
	messagequeue.Repository
	err error
}

func (r *freshStartWorkflowQueueReadErrorRepository) ListBySession(
	context.Context, string,
) ([]messagequeue.QueuedMessage, error) {
	return nil, r.err
}
