package sqlite

import (
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func TestResolveNativeEvidenceAcceptsSettledEarlierIncident(t *testing.T) {
	repo, _, current, attempt, terminal, proof := newSettledEarlierIncidentFixture(t, true)
	proof.Submissions = proof.Submissions[1:]

	changed, err := repo.ResolveSessionRecoveryBlockWithNativeEvidence(t.Context(), current.Block.ID, proof, time.Now().UTC())
	require.NoError(t, err)
	require.True(t, changed)

	block, err := repo.GetSessionRecoveryBlock(t.Context(), current.Block.ID)
	require.NoError(t, err)
	require.Equal(t, models.RecoveryBlockResolved, block.State)
	require.Equal(t, "resume", block.AuthorizedAction)

	storedAttempt, err := repo.GetRestoreAttempt(t.Context(), attempt.ID)
	require.NoError(t, err)
	require.Equal(t, models.SilentRestoreOutcomeBlocked, storedAttempt.Outcome)
	require.Equal(t, historicalTerminalAttemptReason, storedAttempt.Reason)
	require.NotNil(t, storedAttempt.CompletedAt)

	storedCurrent, err := repo.GetAgentDeliverySubmission(t.Context(), current.ID)
	require.NoError(t, err)
	require.Equal(t, models.DeliverySubmissionInterruptedUnknown, storedCurrent.State)
	storedEarlier, err := repo.GetAgentDeliverySubmission(t.Context(), terminal.SubmissionID)
	require.NoError(t, err)
	require.Equal(t, models.DeliverySubmissionFailed, storedEarlier.State)
}

func TestResolveNativeEvidenceRejectsUnsettledEarlierIncident(t *testing.T) {
	repo, _, current, _, _, proof := newSettledEarlierIncidentFixture(t, false)
	proof.Submissions = proof.Submissions[1:]

	changed, err := repo.ResolveSessionRecoveryBlockWithNativeEvidence(t.Context(), current.Block.ID, proof, time.Now().UTC())
	require.NoError(t, err)
	require.False(t, changed)
}

func TestResolveNativeEvidenceRejectsEarlierIncidentMissingTerminalEffect(t *testing.T) {
	repo, _, current, _, terminal, proof := newSettledEarlierIncidentFixture(t, true)
	_, err := repo.DB().ExecContext(t.Context(), repo.db.Rebind(`DELETE FROM agent_delivery_effects WHERE effect_key = ?`), terminalRetentionEffectKey(terminal))
	require.NoError(t, err)
	proof.Submissions = proof.Submissions[1:]

	changed, err := repo.ResolveSessionRecoveryBlockWithNativeEvidence(t.Context(), current.Block.ID, proof, time.Now().UTC())
	require.NoError(t, err)
	require.False(t, changed)
}

func TestResolveNativeEvidenceRejectsMismatchedEarlierIncidentProofRow(t *testing.T) {
	repo, _, current, _, _, proof := newSettledEarlierIncidentFixture(t, true)
	proof.Submissions[0].PayloadHash = "different-payload-hash"

	changed, err := repo.ResolveSessionRecoveryBlockWithNativeEvidence(t.Context(), current.Block.ID, proof, time.Now().UTC())
	require.NoError(t, err)
	require.False(t, changed)
}

func TestResolveNativeEvidenceLogsRefusingStep(t *testing.T) {
	repo := newRepoForSessionTests(t)
	core, logs := observer.New(zapcore.WarnLevel)
	observedLogger, err := logger.NewFromZap(zap.New(core))
	require.NoError(t, err)
	repo.log = observedLogger

	_, fixture, current, _, _, proof := newSettledEarlierIncidentFixtureOnRepo(t, repo, false)
	proof.Submissions = proof.Submissions[1:]
	changed, err := repo.ResolveSessionRecoveryBlockWithNativeEvidence(t.Context(), current.Block.ID, proof, time.Now().UTC())
	require.NoError(t, err)
	require.False(t, changed)

	warnings := logs.FilterMessage("native resume recovery block resolution refused").All()
	require.Len(t, warnings, 1)
	fields := warnings[0].ContextMap()
	require.Equal(t, fixture.session.ID, fields["session_id"])
	require.Equal(t, current.Block.ID, fields["block_id"])
	require.Equal(t, "prepared_attempt", fields["step"])
}

func newSettledEarlierIncidentFixture(t *testing.T, settle bool) (
	*Repository,
	*silentRestoreFixture,
	historicalCurrentSubmission,
	*models.RestoreAttempt,
	models.AgentDeliveryEvent,
	*models.NativeResumeDeliveryEvidence,
) {
	t.Helper()
	repo := newRepoForSessionTests(t)
	return newSettledEarlierIncidentFixtureOnRepo(t, repo, settle)
}

func newSettledEarlierIncidentFixtureOnRepo(t *testing.T, repo *Repository, settle bool) (
	*Repository,
	*silentRestoreFixture,
	historicalCurrentSubmission,
	*models.RestoreAttempt,
	models.AgentDeliveryEvent,
	*models.NativeResumeDeliveryEvidence,
) {
	t.Helper()
	fixture := newSilentRestoreFixture(t, repo, "settled-earlier", models.TaskSessionStateWaitingForInput)
	terminal := settleHistoricalTerminalSource(t, repo, fixture, settle)
	current := createHistoricalCurrentSubmission(t, repo, fixture, "current-settled-earlier")
	attempt := createHistoricalTerminalAttempt(t, repo, fixture, "attempt-settled-earlier", "")
	notice := fixture.recovery
	notice.Phase = models.AgentDeliveryRecoverySettled
	require.NoError(t, repo.SetSessionMetadataKey(t.Context(), fixture.session.ID, models.SessionMetaKeyAgentDeliveryRecovery, notice))
	proof := nativeResumeProofForHistoricalSource(t, repo, fixture, terminal, current, "current-execution-"+current.ID)
	return repo, fixture, current, attempt, terminal, proof
}
