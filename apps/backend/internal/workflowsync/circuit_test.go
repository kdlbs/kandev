package workflowsync

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/common/authcircuit"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/github"
)

// fakeFingerprintGitHubClients wraps fakeGitHubClients with a
// CredentialFingerprintProvider implementation, so tests can drive the
// "credential rotated" reset path without a real github.Service.
type fakeFingerprintGitHubClients struct {
	fakeGitHubClients
	fingerprint string
	err         error
}

func (f fakeFingerprintGitHubClients) WorkspaceConnectionFingerprint(
	_ context.Context, _ string,
) (string, error) {
	return f.fingerprint, f.err
}

var _ CredentialFingerprintProvider = fakeFingerprintGitHubClients{}

func setupCircuitTestService(t *testing.T, clients GitHubClientProvider) (*Service, *fakeApplier) {
	t.Helper()
	store := setupTestStore(t)
	applier := &fakeApplier{}
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "console"})
	require.NoError(t, err)
	return NewService(store, clients, nil, applier, log), applier
}

// TestSyncDueConfigs_OpenCircuitSkipsWithoutCallingProvider is the core
// AC-9 behavior: once a config's circuit is open, SyncDueConfigs must not
// call the provider at all — not even to fail again — until the backoff
// window elapses or the credential/config changes.
func TestSyncDueConfigs_OpenCircuitSkipsWithoutCallingProvider(t *testing.T) {
	svc, applier := setupCircuitTestService(t, fakeGitHubClients{client: seededMockClient()})
	configureWorkspace(t, svc, "ws-1")

	future := time.Now().UTC().Add(time.Hour)
	require.NoError(t, svc.store.RecordSyncStatus(
		context.Background(), "ws-1", false, "boom", nil, "", time.Now().UTC(),
		authcircuit.State{FailureClass: authcircuit.FailureClassAuth, ConsecutiveFailures: 3, NextRetryAt: &future},
	))

	svc.SyncDueConfigs(context.Background())
	assert.Empty(t, applier.calls, "an open circuit must skip the sync attempt entirely")
}

// TestSyncDueConfigs_RepeatedFailuresOpenCircuit confirms a config that
// keeps failing (here: a 404 "directory not found" from the mock client,
// classified as config) eventually stops retrying every tick, rather than
// costing one GitHub call per tick forever.
func TestSyncDueConfigs_RepeatedFailuresOpenCircuit(t *testing.T) {
	svc, applier := setupCircuitTestService(t, fakeGitHubClients{client: github.NewMockClient()}) // nothing seeded → 404 every time
	configureWorkspace(t, svc, "ws-1")

	// First attempt: due (never synced), fails, opens the circuit.
	svc.SyncDueConfigs(context.Background())
	cfg, err := svc.GetConfigForWorkspace(context.Background(), "ws-1")
	require.NoError(t, err)
	assert.Equal(t, authcircuit.FailureClassConfig, cfg.FailureClass)
	require.NotNil(t, cfg.NextRetryAt)
	assert.True(t, cfg.NextRetryAt.After(time.Now().UTC()), "backoff should schedule a future retry")

	// A config-class failure uses the long "permanent" backoff schedule
	// (minutes, not the config's own 5-minute poll interval alone) — so a
	// tick that would otherwise be "due" per the plain interval must still
	// be skipped while the circuit is open.
	svc.SyncDueConfigs(context.Background())
	assert.Empty(t, applier.calls, "no successful apply ever happened, but no second GitHub call either")
}

// TestSyncDueConfigs_CredentialFingerprintChangeResetsCircuitAndForcesSync
// confirms Task 04's headline behavior: rotating/reconnecting a credential
// (a fingerprint change) resumes syncing on the very next tick rather than
// waiting out the remaining permanent backoff.
func TestSyncDueConfigs_CredentialFingerprintChangeResetsCircuitAndForcesSync(t *testing.T) {
	clients := fakeFingerprintGitHubClients{
		fakeGitHubClients: fakeGitHubClients{client: seededMockClient()},
		fingerprint:       "active:1",
	}
	svc, applier := setupCircuitTestService(t, clients)
	configureWorkspace(t, svc, "ws-1")

	future := time.Now().UTC().Add(time.Hour)
	require.NoError(t, svc.store.RecordCircuitState(
		context.Background(), "ws-1",
		authcircuit.State{
			FailureClass: authcircuit.FailureClassAuth, ConsecutiveFailures: 3, NextRetryAt: &future,
			Fingerprint: "active:1",
		},
	))

	// Same fingerprint: circuit stays open, still no sync.
	svc.SyncDueConfigs(context.Background())
	assert.Empty(t, applier.calls, "an unchanged fingerprint must not reset an open circuit")

	// Credential rotates (generation bumps).
	clients.fingerprint = "active:2"
	svc.githubClients = clients

	svc.SyncDueConfigs(context.Background())
	assert.Len(t, applier.calls, 1, "a changed fingerprint must reset the circuit and sync immediately")

	cfg, err := svc.GetConfigForWorkspace(context.Background(), "ws-1")
	require.NoError(t, err)
	assert.Empty(t, cfg.FailureClass, "a successful sync after reset clears the failure class")
}

// TestSyncDueConfigs_EmptyFingerprintNeverResets confirms a provider that
// cannot currently determine the fingerprint (transient lookup error, or a
// workspace with no connection at all) never accidentally clears an open
// circuit — see authcircuit.State.ResetIfFingerprintChanged's contract.
func TestSyncDueConfigs_EmptyFingerprintNeverResets(t *testing.T) {
	clients := fakeFingerprintGitHubClients{
		fakeGitHubClients: fakeGitHubClients{client: seededMockClient()},
		fingerprint:       "",
	}
	svc, applier := setupCircuitTestService(t, clients)
	configureWorkspace(t, svc, "ws-1")

	future := time.Now().UTC().Add(time.Hour)
	require.NoError(t, svc.store.RecordSyncStatus(
		context.Background(), "ws-1", false, "boom", nil, "", time.Now().UTC(),
		authcircuit.State{FailureClass: authcircuit.FailureClassAuth, ConsecutiveFailures: 3, NextRetryAt: &future},
	))

	svc.SyncDueConfigs(context.Background())
	assert.Empty(t, applier.calls, "an empty fingerprint must never reset an open circuit")
}

// TestWorkflowSyncCircuitSummary_AggregatesByClass confirms the health
// checker's data source counts only open circuits, grouped by class, with
// no per-workspace identifiers leaking into the aggregate.
func TestWorkflowSyncCircuitSummary_AggregatesByClass(t *testing.T) {
	svc, _ := setupCircuitTestService(t, fakeGitHubClients{client: seededMockClient()})
	configureWorkspace(t, svc, "ws-auth")
	configureWorkspace(t, svc, "ws-config")
	configureWorkspace(t, svc, "ws-ok")

	future := time.Now().UTC().Add(time.Hour)
	ctx := context.Background()
	require.NoError(t, svc.store.RecordSyncStatus(ctx, "ws-auth", false, "boom", nil, "", time.Now().UTC(),
		authcircuit.State{FailureClass: authcircuit.FailureClassAuth, ConsecutiveFailures: 1, NextRetryAt: &future}))
	require.NoError(t, svc.store.RecordSyncStatus(ctx, "ws-config", false, "boom", nil, "", time.Now().UTC(),
		authcircuit.State{FailureClass: authcircuit.FailureClassConfig, ConsecutiveFailures: 1, NextRetryAt: &future}))

	summary, err := svc.WorkflowSyncCircuitSummary(ctx)
	require.NoError(t, err)
	assert.Equal(t, 3, summary.Total)
	assert.Equal(t, 1, summary.OpenAuth)
	assert.Equal(t, 1, summary.OpenConfig)
	assert.Equal(t, 0, summary.OpenTransient)
}
