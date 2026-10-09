package orchestrator

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/stretchr/testify/require"
)

func TestInterruptedResumeRetriesPreparedCheckpointAfterRestoreFailure(t *testing.T) {
	ctx := context.Background()
	svc, request, launches := interruptedResumeFixture(t)
	manager := svc.agentManager.(*interruptedRecoveryManager).mockAgentManager
	restore := manager.launchAgentFunc
	manager.launchAgentFunc = func(context.Context, *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
		return nil, errors.New("temporary native restore failure")
	}
	first, err := svc.ResumeInterruptedSession(ctx, "t1", "s1", request)
	require.NoError(t, err)
	require.Equal(t, SessionDeliveryRecoveryRestoredBlocked, first.Outcome)
	require.Empty(t, manager.capturedPrompts)
	manager.launchAgentFunc = restore
	result, err := svc.ResumeInterruptedSession(ctx, "t1", "s1", request)
	require.NoError(t, err)
	require.Equal(t, SessionDeliveryRecoveryContinued, result.Outcome)
	require.Equal(t, 1, *launches)
	require.Equal(t, []string{request.Instruction}, manager.capturedPrompts)
}

type interruptedAcceptanceFailureRepository struct {
	*sqliterepo.Repository
	failSnapshot bool
}

func (r *interruptedAcceptanceFailureRepository) CompleteContinuationSnapshot(ctx context.Context, id, status string, at time.Time) error {
	if r.failSnapshot {
		r.failSnapshot = false
		return errors.New("temporary acceptance bookkeeping failure")
	}
	return r.Repository.CompleteContinuationSnapshot(ctx, id, status, at)
}

func TestInterruptedResumeRepairsAcceptanceCheckpointWithoutDispatch(t *testing.T) {
	ctx := context.Background()
	svc, request, launches := interruptedResumeFixture(t)
	svc.repo = &interruptedAcceptanceFailureRepository{Repository: svc.repo.(*sqliterepo.Repository), failSnapshot: true}
	first, err := svc.ResumeInterruptedSession(ctx, "t1", "s1", request)
	require.NoError(t, err)
	require.Equal(t, SessionDeliveryRecoveryRestoredBlocked, first.Outcome)
	result, err := svc.ResumeInterruptedSession(ctx, "t1", "s1", request)
	require.NoError(t, err)
	require.Equal(t, SessionDeliveryRecoveryContinued, result.Outcome)
	require.Equal(t, 1, *launches)
	manager := svc.agentManager.(*interruptedRecoveryManager).mockAgentManager
	require.Equal(t, []string{request.Instruction}, manager.capturedPrompts)
	messages, err := svc.repo.ListMessages(ctx, "s1")
	require.NoError(t, err)
	require.Len(t, messages, 2)
	require.Equal(t, models.MessageAuthorUser, messages[1].AuthorType)
}

func TestInterruptedResumeWithoutAcceptanceEvidenceNeverRedispatches(t *testing.T) {
	ctx := context.Background()
	svc, request, launches := interruptedResumeFixture(t)
	creator := &interruptedInstructionFailureCreator{MessageCreator: svc.messageCreator, instruction: request.Instruction, fail: true}
	svc.messageCreator = creator
	first, err := svc.ResumeInterruptedSession(ctx, "t1", "s1", request)
	require.NoError(t, err)
	require.Equal(t, SessionDeliveryRecoveryRestoredBlocked, first.Outcome)
	creator.fail = false
	result, err := svc.ResumeInterruptedSession(ctx, "t1", "s1", request)
	require.NoError(t, err)
	require.Equal(t, SessionDeliveryRecoveryRestoredBlocked, result.Outcome)
	require.Equal(t, continuationOutcomeUnknown, result.Reason)
	require.Equal(t, 1, *launches)
	manager := svc.agentManager.(*interruptedRecoveryManager).mockAgentManager
	require.Equal(t, []string{request.Instruction}, manager.capturedPrompts)
	messages, err := svc.repo.ListMessages(ctx, "s1")
	require.NoError(t, err)
	require.Len(t, messages, 1)
}

type interruptedInstructionFailureCreator struct {
	MessageCreator
	instruction string
	fail        bool
}

func (m *interruptedInstructionFailureCreator) CreateUserMessageIdempotent(ctx context.Context, messageID, taskID, content, sessionID, turnID string, metadata map[string]interface{}) error {
	if m.fail && content == m.instruction {
		return errors.New("acceptance receipt write failed")
	}
	return m.MessageCreator.CreateUserMessageIdempotent(ctx, messageID, taskID, content, sessionID, turnID, metadata)
}
