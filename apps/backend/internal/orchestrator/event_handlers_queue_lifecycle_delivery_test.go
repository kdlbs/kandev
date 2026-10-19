package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/github"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/plancomments"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	taskservice "github.com/kandev/kandev/internal/task/service"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

type lifecycleDeliveryFixture struct {
	logs           *observer.ObservedLogs
	service        *Service
	repo           *sqliterepo.Repository
	manager        *retainedDeliveryTestManager
	calls          int
	submission     *models.AgentDeliverySubmission
	transportError error
	atTransport    func()
}

func newLifecycleDeliveryFixture(t *testing.T, durable bool) *lifecycleDeliveryFixture {
	t.Helper()
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	seedExecutorRunning(t, repo, "s1", "t1", "exec-1")
	require.NoError(t, repo.UpdateTaskSessionState(ctx, "s1", models.TaskSessionStateWaitingForInput, ""))
	db := sqlx.NewDb(repo.DB(), "sqlite3")
	queueRepo, err := messagequeue.NewSQLiteRepository(db, db)
	require.NoError(t, err)
	f := &lifecycleDeliveryFixture{repo: repo}
	f.manager = &retainedDeliveryTestManager{
		durableDeliveryTestAgentManager: &durableDeliveryTestAgentManager{
			mockAgentManager: &mockAgentManager{isAgentRunning: true, repoForExecutionLookup: repo},
			advertised:       durable,
			capability:       agentruntime.DurableDeliveryCapability{Version: journal.CurrentVersion, Durable: true},
		},
		onSubmission: func(context.Context, string) {},
	}
	f.manager.promptAgentFunc = func(ctx context.Context, _ string, prompt string, attachments []v1.MessageAttachment, _ bool) (*executor.PromptResult, error) {
		f.calls++
		if durable {
			stored, readErr := repo.GetAgentDeliverySubmission(ctx, f.manager.submissionID)
			require.NoError(t, readErr)
			require.Equal(t, models.DeliverySubmissionDispatching, stored.State)
			require.Equal(t, journal.SubmissionHash(stored.Payload), stored.PayloadHash)
			var payload agentDeliveryPromptPayload
			require.NoError(t, json.Unmarshal(stored.Payload, &payload))
			require.Equal(t, prompt, payload.Text)
			require.Len(t, payload.Attachments, len(attachments))
			for i := range attachments {
				require.Equal(t, attachments[i], payload.Attachments[i])
			}
			f.submission = stored
		}
		if f.atTransport != nil {
			f.atTransport()
		}
		if f.transportError != nil {
			return nil, f.transportError
		}
		return &executor.PromptResult{}, nil
	}
	taskRepo := newMockTaskRepo()
	seedMockTaskState(taskRepo, "t1", v1.TaskStateInProgress)
	f.service = createTestServiceWithAgent(repo, newMockStepGetter(), taskRepo, f.manager)
	f.service.messageQueue = messagequeue.NewService(queueRepo, messagequeue.DefaultMaxPerSession, testLogger())
	taskSvc := taskservice.NewService(taskservice.Repos{
		Tasks: repo, TaskRepos: repo, Workspaces: repo, Workflows: repo,
		Messages: repo, Turns: repo, Sessions: repo, Executors: repo,
		TaskEnvironments: repo, GitSnapshots: repo, RepoEntities: repo,
	}, &recordingEventBus{}, testLogger(), taskservice.RepositoryDiscoveryConfig{})
	f.service.messageCreator = &serviceBackedMessageCreator{svc: taskSvc}
	f.service.SetTurnService(taskSvc)
	core, logs := observer.New(zap.DebugLevel)
	f.logs = logs
	f.service.logger, err = logger.NewFromZap(zap.New(core))
	require.NoError(t, err)
	f.service.executor = executor.NewExecutor(f.manager, repo, testLogger(), executor.ExecutorConfig{})
	t.Cleanup(f.service.cancelAllTransientRetries)
	return f
}

func (f *lifecycleDeliveryFixture) count(t *testing.T, table string) int {
	t.Helper()
	var count int
	// Table names are fixture-owned constants, never external input.
	require.NoError(t, f.repo.DB().QueryRow("SELECT COUNT(*) FROM "+table).Scan(&count))
	return count
}

func (f *lifecycleDeliveryFixture) reserve(t *testing.T, metadata map[string]interface{}) *messagequeue.QueuedMessage {
	t.Helper()
	ctx := context.Background()
	_, err := f.service.messageQueue.QueueMessageWithMetadata(ctx, "s1", "t1", "PR lifecycle delivery", "", messagequeue.QueuedByWorkflow, false, nil, metadata)
	require.NoError(t, err)
	identity, err := f.service.messageQueue.ResolveSessionIdentity(ctx, "t1", "s1")
	require.NoError(t, err)
	require.NoError(t, f.service.messageQueue.SetAutoRunForSession(ctx, identity, true))
	queued, ok, _, err := f.service.messageQueue.ReserveQueuedWithAutoRunForSession(ctx, identity)
	require.NoError(t, err)
	require.True(t, ok)
	return queued
}

func (f *lifecycleDeliveryFixture) execute(t *testing.T, queued *messagequeue.QueuedMessage) {
	t.Helper()
	identity, err := f.service.messageQueue.ResolveSessionIdentity(context.Background(), "t1", "s1")
	require.NoError(t, err)
	reservation := f.service.markQueuedDispatchInFlightWithIdentityLocked(context.Background(), identity, queued.ID, queued)
	// Execute synchronously: no competing drain is requested, so completion has
	// no follow-up worker to join and failure cannot create an unbounded loop.
	f.service.executeQueuedMessageWithReservation("s1", queued, reservation)
}

// @covers AC-UI-CI-PR-AUTOMATION-001.18
func TestQueuedLifecycleDeliverySQLite(t *testing.T) {
	for _, durable := range []bool{false, true} {
		mode := "legacy"
		if durable {
			mode = "durable_v1"
		}
		for _, event := range []string{"review_requested", "merged", "closed"} {
			t.Run(mode+"/"+event, func(t *testing.T) {
				f := newLifecycleDeliveryFixture(t, durable)
				queued := f.reserve(t, map[string]interface{}{"origin": githubPRAutomationOrigin, "event": event})
				require.True(t, queued.IsReservedLifecycleDelivery())
				require.Zero(t, f.count(t, "queue_dispatch_claims"))
				f.atTransport = func() {
					require.Equal(t, 1, f.count(t, "queued_messages"), "retain until transport accepts")
					require.Zero(t, f.count(t, "queue_dispatch_claims"))
				}
				f.execute(t, queued)
				require.Equal(t, 1, f.calls, "lifecycle reservation must reach transport without an ordinary claim: %v", f.logs.All())
				require.Zero(t, f.count(t, "queued_messages"), "acceptance acknowledges retained row")
				require.Zero(t, f.service.messageQueue.GetStatus(context.Background(), "s1").Count)
				require.Zero(t, f.count(t, "queue_dispatch_claims"))
				if durable {
					require.NotNil(t, f.submission)
				}
			})
		}
	}
}

func TestQueuedOrdinaryDeliveryClaimControls(t *testing.T) {
	for _, durable := range []bool{false, true} {
		for _, mutation := range []string{"valid", "missing", "stale"} {
			mode := "legacy"
			if durable {
				mode = "durable_v1"
			}
			t.Run(mode+"/"+mutation, func(t *testing.T) {
				f := newLifecycleDeliveryFixture(t, durable)
				queued := f.reserve(t, nil)
				require.False(t, queued.IsDurableDelivery())
				require.Equal(t, 1, f.count(t, "queue_dispatch_claims"))
				switch mutation {
				case "missing":
					_, err := f.repo.DB().Exec("DELETE FROM queue_dispatch_claims")
					require.NoError(t, err)
				case "stale":
					_, err := f.repo.DB().Exec("UPDATE queue_dispatch_claims SET attempt_id = 'replacement-attempt'")
					require.NoError(t, err)
				}
				if mutation == "valid" {
					f.atTransport = func() {
						var protocol, submissionID, hash string
						require.NoError(t, f.repo.DB().QueryRow("SELECT protocol_mode, submission_id, payload_hash FROM queue_dispatch_claims").Scan(&protocol, &submissionID, &hash))
						require.Equal(t, mode, protocol)
						if durable {
							require.Equal(t, f.submission.ID, normalizeDeliverySubmissionID(submissionID))
							require.Equal(t, f.submission.PayloadHash, hash)
						}
					}
				}
				f.execute(t, queued)
				if mutation == "valid" {
					require.Equal(t, 1, f.calls)
				} else {
					require.Zero(t, f.calls)
				}
			})
		}
	}
}

func TestQueuedLifecycleDeliveryFailure(t *testing.T) {
	for _, scenario := range []string{"unaccepted", "unresolved", "archived"} {
		t.Run(scenario, func(t *testing.T) {
			f := newLifecycleDeliveryFixture(t, scenario == "unresolved")
			queued := f.reserve(t, map[string]interface{}{"origin": githubPRAutomationOrigin})
			switch scenario {
			case "unaccepted":
				f.transportError = errors.New("transport refused before acceptance")
			case "unresolved":
				f.manager.capability.Unresolved = true
			case "archived":
				require.NoError(t, f.repo.ArchiveTask(context.Background(), "t1"))
			}
			f.execute(t, queued)
			if scenario == "unaccepted" {
				require.Positive(t, f.calls)
				require.Equal(t, 1, f.service.messageQueue.GetStatus(context.Background(), "s1").Count)
			} else {
				require.Zero(t, f.calls)
			}
			require.Zero(t, f.count(t, "queue_dispatch_claims"))
		})
	}
}

func TestQueuedDeliveryUpdaterEligibility(t *testing.T) {
	for _, kind := range []string{"plan_comment", "managed_input"} {
		t.Run(kind, func(t *testing.T) {
			f := newLifecycleDeliveryFixture(t, false)
			metadata := map[string]interface{}{plancomments.MetadataClientQueueID: "comment-1", plancomments.MetadataRequestFingerprint: "fingerprint"}
			if kind == "managed_input" {
				metadata = map[string]interface{}{messagequeue.MetadataManagedInput: true, messagequeue.MetadataManagedInputID: "input-1"}
			}
			queued := f.reserve(t, metadata)
			require.True(t, queued.IsDurableDelivery())
			require.False(t, queued.IsDurableLifecycle())
			require.Zero(t, f.count(t, "queue_dispatch_claims"))
			// These receipt kinds keep their existing ordinary updater rejection. A
			// broad retained-row bypass would reach transport and violate this scope.
			f.execute(t, queued)
			require.Zero(t, f.calls)
			require.Equal(t, 1, f.logs.FilterMessage("preparing direct agent delivery submission").Len(), "receipt must reach protocol preparation: %v", f.logs.All())
		})
	}
}

// @covers AC-UI-CI-PR-AUTOMATION-001.9
// @covers AC-UI-CI-PR-AUTOMATION-001.10
// @covers AC-UI-CI-PR-AUTOMATION-001.18
func TestQueuedLifecycleDeliveryCIAutoFix(t *testing.T) {
	for _, durable := range []bool{false, true} {
		mode := "legacy"
		if durable {
			mode = "durable_v1"
		}
		t.Run(mode, func(t *testing.T) {
			f := newLifecycleDeliveryFixture(t, durable)
			ctx := context.Background()
			history := streams.MCPAttachmentHistory{
				Version: streams.MCPAttachmentSchemaVersion,
				Current: streams.MCPAttachmentAttempt{AttemptID: "current", Servers: []streams.MCPServerAttachment{{
					Name: "kandev", Source: streams.MCPServerSourceKandev,
					Tools: []streams.MCPToolSummary{{Name: ciAutomationNeutralOutcomeTool}},
				}}},
			}
			require.NoError(t, f.repo.SetSessionMetadataKey(ctx, "s1", models.SessionMetaKeyMCPAttachmentState, history))
			db := sqlx.NewDb(f.repo.DB(), "sqlite3")
			store, err := github.NewStore(db, db)
			require.NoError(t, err)
			provider := github.NewService(nil, "", nil, store, &recordingEventBus{}, testLogger())
			t.Cleanup(provider.Stop)
			f.service.githubService = provider
			queued := f.reserve(t, map[string]interface{}{
				"origin": ciAutomationOrigin, "automation_kind": ciAutomationKindAutoFix,
				"repository_id": "repo-1", "pr_number": 17, "feedback_signature": "feedback-1",
			})
			require.NoError(t, store.RecordTaskCIFixAttempt(ctx, github.TaskCIFixAttempt{
				TaskID: "t1", RepositoryID: "repo-1", PRNumber: 17, SessionID: "s1",
				QueueEntryID: queued.ID, Signature: "feedback-1", State: github.TaskCIAutoFixAttemptQueued,
				EnqueuedAt: time.Now().UTC(), IncrementRound: true,
			}))
			f.execute(t, queued)
			require.Equal(t, 1, f.calls, "CI lifecycle consumer must reach transport: %v", f.logs.All())
			require.Zero(t, f.count(t, "queued_messages"))
			require.Zero(t, f.count(t, "queue_dispatch_claims"))
			state, err := store.GetTaskCIPRState(ctx, "t1", "repo-1", 17)
			require.NoError(t, err)
			require.NotEmpty(t, state.AutoFixAttemptTurnID)
			require.Equal(t, queued.ID, state.AutoFixAttemptQueueEntryID)
			require.Equal(t, "s1", state.AutoFixAttemptSessionID)
			require.Equal(t, 1, state.AutoFixRoundCount)
		})
	}
}

// @covers AC-UI-CI-PR-AUTOMATION-001.18
func TestQueuedLifecycleDeliveryDurableError(t *testing.T) {
	ctx := context.Background()
	f := newLifecycleDeliveryFixture(t, true)
	session, err := f.repo.GetTaskSession(ctx, "s1")
	require.NoError(t, err)
	require.NoError(t, f.repo.CreateHarnessSessionGeneration(ctx, &models.HarnessSessionGeneration{
		SessionID: session.ID, IncarnationID: session.QueueIncarnationID,
		Generation: 1, NativeSessionID: "native", CreationReason: "initial",
	}))
	queued := f.reserve(t, map[string]interface{}{"origin": githubPRAutomationOrigin})
	f.transportError = errors.New("transport disconnected before its reply")

	f.execute(t, queued)

	require.Equal(t, 1, f.calls)
	require.NotNil(t, f.submission)
	stored, err := f.repo.GetAgentDeliverySubmission(ctx, f.submission.ID)
	require.NoError(t, err)
	require.Equal(t, models.DeliverySubmissionInterruptedUnknown, stored.State)
	require.Equal(t, "s1", stored.SessionID)
	require.Equal(t, f.submission.PayloadHash, stored.PayloadHash)
	block, err := f.service.GetOpenSessionRecoveryBlock(ctx, "s1")
	require.NoError(t, err)
	require.NotNil(t, block)
	require.Equal(t, "s1", block.SessionID)
	require.Equal(t, stored.ID, block.DeliverySubmissionID)
	require.Equal(t, "unknown_prompt_outcome", block.Reason)
	require.Equal(t, 1, f.count(t, "queued_messages"), "uncertainty must not acknowledge the lifecycle row")
	var retainedQueueID string
	require.NoError(t, f.repo.DB().QueryRowContext(ctx, "SELECT id FROM queued_messages WHERE session_id = ?", "s1").Scan(&retainedQueueID))
	require.Equal(t, queued.ID, retainedQueueID)
	require.Zero(t, f.count(t, "queue_dispatch_claims"))

	session, err = f.repo.GetTaskSession(ctx, "s1")
	require.NoError(t, err)
	_, err = f.service.queueAndDrainLifecyclePrompt(
		ctx, session, "t1", queued.Content, queued.Metadata, "durable-error", messagequeue.ErrTaskInactive,
	)
	require.ErrorIs(t, err, ErrSessionRecoveryRequired)
	require.Equal(t, 1, f.calls, "real admission must not resend an uncertain submission")
	require.Equal(t, 1, f.count(t, "queued_messages"))
	retained, err := f.service.GetOpenSessionRecoveryBlock(ctx, "s1")
	require.NoError(t, err)
	require.Equal(t, block.ID, retained.ID)
	require.Equal(t, stored.ID, retained.DeliverySubmissionID)
	stillUnknown, err := f.repo.GetAgentDeliverySubmission(ctx, stored.ID)
	require.NoError(t, err)
	require.Equal(t, models.DeliverySubmissionInterruptedUnknown, stillUnknown.State)
}
