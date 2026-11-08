package orchestrator

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/sysprompt"
	"github.com/kandev/kandev/internal/task/models"
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
		cached, cachedOK := fixture.svc.lastTurnPrompt.Load(fixture.sessionID)
		require.True(t, cachedOK)
		captured, capturedOK := cached.(capturedPrompt)
		require.True(t, capturedOK)
		require.Equal(t, submissionText, captured.text, "dynamic fallback must retain the raw submission")
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

	t.Run("saved prompt read failure stops both recovery paths", func(t *testing.T) {
		for _, withReceipt := range []bool{true, false} {
			withReceipt := withReceipt
			sourceName := "legacy"
			if withReceipt {
				sourceName = "receipt"
			}
			t.Run(sourceName, func(t *testing.T) {
				fixture := newFreshStartWorkflowFixture(t, freshStartWorkflowOptions{
					withReceipt:     withReceipt,
					receiptContent:  submissionText,
					description:     submissionText,
					stepPrompt:      "Follow @principles. {{task_prompt}}",
					strictPromptErr: true,
				})

				observation := fixture.recover(t, "fresh_start")

				require.Error(t, observation.recoveryErr)
				require.Contains(t, observation.recoveryErr.Error(), "saved prompt")
				require.Zero(t, observation.launchCount)
				require.Empty(t, observation.capturedPrompts)
				require.Equal(t, models.TaskSessionStateFailed, observation.session.State)
				if withReceipt {
					require.Equal(t, models.InitialPromptSubmissionPending, observation.receipt.State)
				}
			})
		}
	})

	t.Run("unfenced workflow route stops both recovery paths", func(t *testing.T) {
		for _, withReceipt := range []bool{true, false} {
			withReceipt := withReceipt
			sourceName := "legacy"
			if withReceipt {
				sourceName = "receipt"
			}
			t.Run(sourceName, func(t *testing.T) {
				fixture := newFreshStartWorkflowFixture(t, freshStartWorkflowOptions{
					withReceipt:          withReceipt,
					receiptContent:       submissionText,
					description:          submissionText,
					stepPrompt:           "Prepared step instructions: {{task_prompt}}",
					routeWithoutIdentity: true,
				})

				observation := fixture.recover(t, "fresh_start")

				require.ErrorIs(t, observation.recoveryErr, ErrCeilingLaunchSuperseded)
				require.Zero(t, observation.launchCount)
				require.Empty(t, observation.capturedPrompts)
				require.Equal(t, models.TaskSessionStateFailed, observation.session.State)
				if withReceipt {
					require.Equal(t, models.InitialPromptSubmissionPending, observation.receipt.State)
				}
			})
		}
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

	t.Run("receipt recovery retains transcript, attachments, and failure history", func(t *testing.T) {
		fixture := newFreshStartWorkflowFixture(t, freshStartWorkflowOptions{
			withReceipt:        true,
			receiptContent:     submissionText,
			description:        "unused task description",
			stepPrompt:         stepPrefix + " {{task_prompt}}",
			repositoryMessages: true,
			withAttachment:     true,
			seedTranscript:     true,
			failureStamp:       "original-failure",
		})

		observation := fixture.recover(t, "fresh_start")

		require.NoError(t, observation.recoveryErr)
		require.Len(t, observation.messages, 1, "recovery must not add another user message")
		message := observation.messages[0]
		require.Equal(t, initialSubmissionUserMessageID(fixture.taskID, fixture.sessionID), message.ID)
		require.Equal(t, "fresh-start-workflow-original-turn", message.TurnID)
		require.Equal(t, submissionText, message.Content)
		payload, marshalErr := json.Marshal(message.Metadata["attachments"])
		require.NoError(t, marshalErr)
		var attachments []v1.MessageAttachment
		require.NoError(t, json.Unmarshal(payload, &attachments))
		require.Len(t, attachments, 1)
		require.Equal(t, "screen.png", attachments[0].Name)
		require.NotNil(t, observation.lastError)
		require.Equal(t, "original-failure", observation.lastError.Stamp())
		require.True(t, observation.lastError.IsDismissed())
	})

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
