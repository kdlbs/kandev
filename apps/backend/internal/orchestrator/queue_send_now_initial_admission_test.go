package orchestrator

import (
	"context"
	"testing"
	"time"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
)

func TestSendNowCreatedClaimWaitsForPreparedAdmission(t *testing.T) {
	for _, outcome := range []string{"accepted", "sql_failure", "shutdown_before_admission", "shutdown_after_admission"} {
		t.Run(outcome, func(t *testing.T) {
			ctx := t.Context()
			repo := setupTestRepo(t)
			seedTaskAndSessionWithStep(t, repo, "task", "session", "step")
			seedExecutorRunning(t, repo, "session", "task", "execution")
			steps := newMockStepGetter()
			steps.steps["step"] = &wfmodels.WorkflowStep{ID: "step", WorkflowID: "workflow", Name: "Step"}
			steps.workflowAgentProfileID = "profile"
			tasks := newMockTaskRepo()
			seedMockTaskState(tasks, "task", v1.TaskStateInProgress)
			manager := &retentionInitialPromptManager{durableDeliveryTestAgentManager: &durableDeliveryTestAgentManager{
				mockAgentManager: &mockAgentManager{repoForExecutionLookup: repo}, advertised: true,
				capability: agentruntime.DurableDeliveryCapability{Version: journal.CurrentVersion, Durable: true},
			}}
			started := make(chan struct{}, 1)
			manager.startAgentProcessFunc = func(context.Context, string) error { started <- struct{}{}; return nil }
			svc := createTestServiceWithScheduler(repo, steps, tasks, manager)
			svc.messageCreator = &mockMessageCreator{}
			queue, queueDB := newWorkflowTransferQueue(t, t.TempDir()+"/queue.db")
			t.Cleanup(func() { require.NoError(t, queueDB.Close()) })
			svc.messageQueue = queue
			require.NoError(t, queue.SetAutoRun(ctx, "session", false))
			source, err := queue.QueueMessage(ctx, "session", "task", "queued instruction", "", messagequeue.QueuedByUser, false, nil)
			require.NoError(t, err)
			claim, err := queue.ClaimSendNow(ctx, "session", []messagequeue.QueuedMessage{*source})
			require.NoError(t, err)
			_, submissionID, _ := claim.DeliverySubmission()
			if outcome == "sql_failure" {
				installRetentionInitialPromptFailure(t, repo, "SQLite", normalizeDeliverySubmissionID(submissionID))
			}
			reservation := svc.markQueuedDispatchInFlight("session", claim.Dispatch.ID)
			workerCtx, cancel := context.WithCancel(ctx)
			done := make(chan struct{})
			t.Cleanup(func() {
				cancel()
				select {
				case <-done:
				case <-time.After(5 * time.Second):
					t.Error("Send Now worker did not stop")
				}
			})
			go func() { defer close(done); svc.executeSendNowClaimWithContext(workerCtx, claim, reservation) }()
			select {
			case <-started:
			case <-time.After(5 * time.Second):
				t.Fatal("initial process did not start")
			}
			pending, err := queue.ListPendingSendNowClaims(ctx)
			require.NoError(t, err)
			require.Len(t, pending, 1, "startup is not prompt acceptance")
			select {
			case <-done:
				t.Fatal("claim worker finished before admission")
			default:
			}
			if outcome == "shutdown_before_admission" {
				cancel()
			} else {
				manager.mu.Lock()
				admit, accepted, failed := manager.initialPromptAdmissionCallback, manager.initialPromptDispatchCallback, manager.initialPromptFailureCallback
				manager.mu.Unlock()
				require.NotNil(t, admit)
				err = admit(ctx, "prepared queued instruction", []v1.MessageAttachment{{Type: "resource", AttachmentID: "file", Name: "bundle-2.zip", MimeType: "application/zip", SizeBytes: 16, DeliveryMode: "prompt"}})
				switch outcome {
				case "sql_failure":
					require.Error(t, err)
					failed()
				case "shutdown_after_admission":
					require.NoError(t, err)
					cancel()
				default:
					require.NoError(t, err)
					accepted()
				}
			}
			select {
			case <-done:
			case <-time.After(5 * time.Second):
				t.Fatal("claim worker did not settle")
			}
			pending, err = queue.ListPendingSendNowClaims(ctx)
			require.NoError(t, err)
			if outcome == "shutdown_after_admission" {
				require.Len(t, pending, 1, "uncertainty must retain the original claim")
				block, err := svc.GetOpenSessionRecoveryBlock(ctx, "session")
				require.NoError(t, err)
				require.Equal(t, "unknown_send_now_outcome", block.Reason)
				manager.mu.Lock()
				lateAdmission := manager.initialPromptAdmissionCallback
				manager.mu.Unlock()
				require.ErrorIs(t, lateAdmission(ctx, "late prompt", nil), context.Canceled)
				submissions, err := repo.ListAgentDeliverySubmissions(ctx, "session")
				require.NoError(t, err)
				require.Len(t, submissions, 1, "stopped worker cannot admit a second initial prompt")
			} else {
				require.Empty(t, pending)
			}
			if outcome == "shutdown_before_admission" {
				manager.mu.Lock()
				lateAdmission := manager.initialPromptAdmissionCallback
				manager.mu.Unlock()
				require.ErrorIs(t, lateAdmission(ctx, "late prompt", nil), context.Canceled)
				submissions, err := repo.ListAgentDeliverySubmissions(ctx, "session")
				require.NoError(t, err)
				require.Empty(t, submissions)
			}
			status := queue.GetStatus(ctx, "session")
			if outcome == "sql_failure" || outcome == "shutdown_before_admission" {
				require.Len(t, status.Entries, 1)
				require.Equal(t, source.ID, status.Entries[0].ID)
			} else {
				require.Empty(t, status.Entries)
			}
		})
	}
}
