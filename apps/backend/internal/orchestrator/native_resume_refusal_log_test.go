package orchestrator

import (
	"context"
	"errors"
	"testing"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/agentctl/journal"
	commonlogger "github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

type refusingLoadedNativeManager struct {
	*nativeResumeTestManager
	refusal error
}

type reasonedDeliveryRefusal struct {
	reason string
	cause  error
}

func (e reasonedDeliveryRefusal) Error() string { return e.cause.Error() }

func (e reasonedDeliveryRefusal) Unwrap() error { return e.cause }

func (e reasonedDeliveryRefusal) DeliveryRecoveryRefusalReason() string { return e.reason }

func (m *refusingLoadedNativeManager) ReconcileLoadedNativeDelivery(context.Context, string) (*models.NativeResumeDeliveryEvidence, error) {
	return nil, m.refusal
}

func (m *refusingLoadedNativeManager) DurableDeliveryCapabilityForExecution(context.Context, string) (agentruntime.DurableDeliveryCapability, bool) {
	return agentruntime.DurableDeliveryCapability{Version: journal.CurrentVersion, Durable: true, Unresolved: true}, true
}

func observeNativeResumeRefusalLogs(t *testing.T, service *Service) *observer.ObservedLogs {
	t.Helper()
	core, logs := observer.New(zapcore.DebugLevel)
	log, err := commonlogger.NewFromZap(zap.New(core))
	require.NoError(t, err)
	service.logger = log
	return logs
}

func assertOneDeliveryRefusalWarning(t *testing.T, logs *observer.ObservedLogs, sessionID, blockID, reason string) {
	t.Helper()
	var warnings []observer.LoggedEntry
	for _, entry := range logs.All() {
		if entry.Level == zapcore.WarnLevel && entry.Message == "agent_delivery recovery remains required" {
			warnings = append(warnings, entry)
		}
	}
	require.Len(t, warnings, 1)
	fields := warnings[0].ContextMap()
	require.Equal(t, sessionID, fields["session_id"])
	require.Equal(t, blockID, fields["block_id"])
	require.Equal(t, reason, fields["reason"])
	for key := range fields {
		require.NotContains(t, key, "payload")
		require.NotContains(t, key, "token")
		require.NotContains(t, key, "path")
	}
}

func TestPrepareBootDeliveryReadinessLogsRefusalReason(t *testing.T) {
	tests := []struct {
		name   string
		err    error
		reason string
	}{
		{name: "delivery blocked", err: agentruntime.ErrDeliveryRecoveryBlocked, reason: "journal_unresolved"},
		{name: "peer evidence unavailable sentinel", err: lifecycle.ErrDurableAdoptionEvidenceUnavailable, reason: "unclassified"},
		{name: "durable adoption blocked sentinel", err: lifecycle.ErrDurableAdoptionBlocked, reason: "unclassified"},
		{name: "execution unavailable", err: reasonedDeliveryRefusal{reason: agentruntime.DeliveryRecoveryRefusalExecutionUnavailable, cause: errors.New("execution unavailable")}, reason: "execution_unavailable"},
		{name: "evidence unavailable", err: reasonedDeliveryRefusal{reason: agentruntime.DeliveryRecoveryRefusalEvidenceUnavailable, cause: errors.New("evidence unavailable")}, reason: "evidence_unavailable"},
		{name: "office owned", err: reasonedDeliveryRefusal{reason: agentruntime.DeliveryRecoveryRefusalOfficeOwned, cause: errors.New("office owns recovery")}, reason: "office_owned"},
		{name: "unclassified", err: errors.New("proof unavailable"), reason: "unclassified"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service, manager, block, _, _, _ := nativeResumeFixture(t, durableDeliveryUnresolvedReason, false, false)
			service.agentManager = &refusingLoadedNativeManager{nativeResumeTestManager: manager, refusal: tt.err}
			logs := observeNativeResumeRefusalLogs(t, service)

			require.False(t, service.prepareBootDeliveryReadiness(context.Background(), block.SessionID, "old-execution"))
			assertOneDeliveryRefusalWarning(t, logs, block.SessionID, block.ID, tt.reason)
		})
	}
}

func TestCheckSessionRecoveryBlockAndNativeResumeReadinessHideInternalCause(t *testing.T) {
	service, manager, block, _, _, _ := nativeResumeFixture(t, durableDeliveryUnresolvedReason, false, false)
	cause := errors.New("internal SQL journal diagnostic")
	service.agentManager = &refusingLoadedNativeManager{nativeResumeTestManager: manager, refusal: cause}
	logs := observeNativeResumeRefusalLogs(t, service)

	checkErr := service.checkSessionRecoveryBlock(context.Background(), block.SessionID)
	require.EqualError(t, checkErr, "session recovery required: unresolved_durable_work")
	require.ErrorIs(t, checkErr, ErrSessionRecoveryRequired)
	require.NotContains(t, checkErr.Error(), cause.Error())
	assertOneDeliveryRefusalWarning(t, logs, block.SessionID, block.ID, "unclassified")
	entries := logs.FilterMessage("agent_delivery recovery remains required").All()
	require.Len(t, entries, 1)
	require.Equal(t, cause.Error(), entries[0].ContextMap()["cause"])

	_, readinessErr := service.reconcileUnresolvedDeliveryProtocol(context.Background(), block.SessionID, "old-execution")
	require.EqualError(t, readinessErr, "session recovery required: unresolved_durable_work")
	require.NotContains(t, readinessErr.Error(), cause.Error())
}

func TestPrepareBootDeliveryReadinessLogsInternalCauseOnce(t *testing.T) {
	service, manager, block, _, _, _ := nativeResumeFixture(t, durableDeliveryUnresolvedReason, false, false)
	cause := errors.New("internal SQL journal diagnostic")
	service.agentManager = &refusingLoadedNativeManager{nativeResumeTestManager: manager, refusal: cause}
	logs := observeNativeResumeRefusalLogs(t, service)

	require.False(t, service.prepareBootDeliveryReadiness(context.Background(), block.SessionID, "old-execution"))
	assertOneDeliveryRefusalWarning(t, logs, block.SessionID, block.ID, "unclassified")
	var warnings []observer.LoggedEntry
	for _, entry := range logs.FilterMessage("agent_delivery recovery remains required").All() {
		if entry.Level == zapcore.WarnLevel {
			warnings = append(warnings, entry)
		}
	}
	require.Len(t, warnings, 1)
	require.Equal(t, cause.Error(), warnings[0].ContextMap()["cause"])
}

func TestNativeResumeBlockRefusalReasonsAreSpecific(t *testing.T) {
	t.Run("office owned", func(t *testing.T) {
		service, _, block, _, _, _ := nativeResumeFixture(t, durableDeliveryUnresolvedReason, false, false)
		task, err := service.repo.GetTask(context.Background(), "task1")
		require.NoError(t, err)
		task.ProjectID = "project-office"
		require.NoError(t, service.repo.UpdateTask(context.Background(), task))
		require.Equal(t, agentruntime.DeliveryRecoveryRefusalOfficeOwned,
			deliveryRecoveryRefusalReason(service.checkNativeResumeDeliveryBlock(context.Background(), "task1", block.SessionID)))
	})

	t.Run("execution capability unavailable", func(t *testing.T) {
		service, manager, block, _, _, _ := nativeResumeFixture(t, durableDeliveryUnresolvedReason, false, false)
		service.agentManager = manager.sessionUpdatingAgentManager
		require.Equal(t, agentruntime.DeliveryRecoveryRefusalExecutionUnavailable,
			deliveryRecoveryRefusalReason(service.checkNativeResumeDeliveryBlock(context.Background(), "task1", block.SessionID)))
	})

	t.Run("evidence reader unavailable", func(t *testing.T) {
		service, _, block, _, _, _ := nativeResumeFixture(t, durableDeliveryUnresolvedReason, false, false)
		service.repo = repositoryWithoutNativeResumeSubmissionReader{
			sessionExecutorStore:      service.repo,
			sessionRecoveryBlockStore: service.repo.(sessionRecoveryBlockStore),
			sessionContinuityStore:    service.repo.(sessionContinuityStore),
		}
		require.Equal(t, agentruntime.DeliveryRecoveryRefusalEvidenceUnavailable,
			deliveryRecoveryRefusalReason(service.checkNativeResumeDeliveryBlock(context.Background(), "task1", block.SessionID)))
	})
}

type repositoryWithoutNativeResumeSubmissionReader struct {
	sessionExecutorStore
	sessionRecoveryBlockStore
	sessionContinuityStore
}

func TestBootDeliveryRefusalDoesNotDuplicateExplicitResumeWarning(t *testing.T) {
	service, manager, block, submission, _, _ := nativeResumeFixture(t, durableDeliveryUnresolvedReason, false, false)
	repo := service.repo.(nativeResumeTestRepository)
	live := *submission
	live.ID = "prompt:live"
	live.State = models.DeliverySubmissionAccepted
	_, err := repo.PrepareAgentDeliverySubmission(context.Background(), &live)
	require.NoError(t, err)
	logs := observeNativeResumeRefusalLogs(t, service)

	_, err = service.RecoverSession(context.Background(), "task1", submission.SessionID, "resume")
	require.ErrorIs(t, err, ErrSessionRecoveryRequired)

	attempt, owner := service.resumeAttemptStore().begin(context.Background(), "task1", submission.SessionID)
	require.True(t, owner)
	defer attempt.finish(service.resumeAttemptStore())
	service.agentManager = &refusingLoadedNativeManager{
		nativeResumeTestManager: manager,
		refusal:                 reasonedDeliveryRefusal{reason: agentruntime.DeliveryRecoveryRefusalLiveWork, cause: errors.New("live work")},
	}
	require.False(t, service.prepareBootDeliveryReadiness(context.Background(), submission.SessionID, "old-execution"))

	warnings := 0
	debugRefusal := false
	var debugFields map[string]interface{}
	for _, entry := range logs.All() {
		if entry.Message == "agent_delivery recovery remains required" {
			if entry.Level == zapcore.WarnLevel {
				warnings++
			}
			if entry.Level == zapcore.DebugLevel {
				debugRefusal = true
				debugFields = entry.ContextMap()
			}
		}
	}
	require.Equal(t, 1, warnings)
	require.True(t, debugRefusal)
	require.Equal(t, "live_work", debugFields["reason"])
	require.Equal(t, "live work", debugFields["cause"])
	assertOneDeliveryRefusalWarning(t, logs, submission.SessionID, block.ID, "live_work")
}

func TestDeliveryRecoveryRefusalReasonSetIsClosed(t *testing.T) {
	reasons := []string{
		agentruntime.DeliveryRecoveryRefusalLiveWork,
		agentruntime.DeliveryRecoveryRefusalPeerRecordMissing,
		agentruntime.DeliveryRecoveryRefusalPeerRecordMismatch,
		agentruntime.DeliveryRecoveryRefusalGenerationSuperseded,
		agentruntime.DeliveryRecoveryRefusalNativeIdentityMismatch,
		agentruntime.DeliveryRecoveryRefusalJournalUnresolved,
		agentruntime.DeliveryRecoveryRefusalProjectionPending,
		agentruntime.DeliveryRecoveryRefusalExecutionUnavailable,
		agentruntime.DeliveryRecoveryRefusalEvidenceUnavailable,
		agentruntime.DeliveryRecoveryRefusalOfficeOwned,
		agentruntime.DeliveryRecoveryRefusalUnclassified,
	}
	for _, reason := range reasons {
		require.True(t, agentruntime.IsDeliveryRecoveryRefusalReason(reason), reason)
	}
	require.False(t, agentruntime.IsDeliveryRecoveryRefusalReason("unknown_reason"))
}

func TestExplicitResumeLogsLiveDeliveryRefusal(t *testing.T) {
	service, _, block, submission, launched, _ := nativeResumeFixture(t, durableDeliveryUnresolvedReason, false, false)
	repo := service.repo.(nativeResumeTestRepository)
	live := *submission
	live.ID = "prompt:live"
	live.State = models.DeliverySubmissionAccepted
	_, err := repo.PrepareAgentDeliverySubmission(context.Background(), &live)
	require.NoError(t, err)
	logs := observeNativeResumeRefusalLogs(t, service)

	_, err = service.RecoverSession(context.Background(), "task1", submission.SessionID, "resume")
	require.ErrorIs(t, err, ErrSessionRecoveryRequired)
	require.Nil(t, *launched)
	assertOneDeliveryRefusalWarning(t, logs, submission.SessionID, block.ID, "live_work")
}

func TestExplicitResumeLogsLifecycleRefusalReason(t *testing.T) {
	tests := []struct {
		name   string
		err    error
		reason string
	}{
		{name: "delivery blocked", err: agentruntime.ErrDeliveryRecoveryBlocked, reason: "journal_unresolved"},
		{name: "peer evidence unavailable", err: lifecycle.ErrDurableAdoptionEvidenceUnavailable, reason: "peer_record_missing"},
		{name: "durable adoption blocked", err: lifecycle.ErrDurableAdoptionBlocked, reason: "peer_record_mismatch"},
		{name: "unclassified", err: errors.New("journal acknowledgement failed"), reason: "unclassified"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service, manager, block, submission, _, _ := nativeResumeFixture(t, durableDeliveryUnresolvedReason, false, false)
			manager.acknowledge = func(context.Context, string) error { return tt.err }
			logs := observeNativeResumeRefusalLogs(t, service)

			_, err := service.RecoverSession(context.Background(), "task1", submission.SessionID, "resume")
			require.ErrorIs(t, err, tt.err)
			assertOneDeliveryRefusalWarning(t, logs, submission.SessionID, block.ID, tt.reason)
		})
	}
}

func TestSuccessfulBootDeliveryReadinessDoesNotLogRefusal(t *testing.T) {
	service, _, submission := nativeReadinessFixture(t, false)
	logs := observeNativeResumeRefusalLogs(t, service)

	require.True(t, service.prepareBootDeliveryReadiness(context.Background(), submission.SessionID, "old-execution"))
	for _, entry := range logs.All() {
		require.NotEqual(t, zapcore.WarnLevel, entry.Level)
		require.NotEqual(t, "agent_delivery recovery remains required", entry.Message)
	}
}
