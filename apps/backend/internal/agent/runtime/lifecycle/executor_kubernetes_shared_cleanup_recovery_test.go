package lifecycle

import (
	"context"
	"errors"
	"github.com/kandev/kandev/internal/secrets"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestKubernetesSharedCleanupAfterRestartBeforeRefresh(t *testing.T) {
	f := newTaskPodFixture(t)
	a := f.launch(t, 1)
	b := f.launch(t, 2)
	f.control.reboot()
	before, _, _ := f.control.snapshot()
	require.NoError(t, f.runtime.StopInstance(context.Background(), a, true))
	require.Nil(t, f.runtime.currentKubernetesSession(a.InstanceID))
	require.NotNil(t, f.runtime.currentKubernetesSession(b.InstanceID))
	after, active, handshakes := f.control.snapshot()
	require.Len(t, after, len(before), "stop must not recreate an agent")
	require.Empty(t, active)
	require.Equal(t, 1, handshakes)
	require.Empty(t, f.resources.deletedPods)
	require.Empty(t, f.resources.deletedPVCs)
	require.NoError(t, f.runtime.StopInstance(context.Background(), b, true))
	_, _, handshakes = f.control.snapshot()
	require.Equal(t, 1, handshakes, "sibling stop must reuse recovered credentials")
}

func TestKubernetesSharedCleanupIdentityFailureRetainsRecovery(t *testing.T) {
	f := newTaskPodFixture(t)
	a := f.launch(t, 1)
	original := f.runtime.currentKubernetesSession(a.InstanceID)
	_, _, before := f.control.snapshot()
	f.resources.mu.Lock()
	f.resources.pod.UID = "foreign-pod"
	f.resources.mu.Unlock()
	require.Error(t, f.runtime.StopInstance(context.Background(), a, true))
	require.Same(t, original, f.runtime.currentKubernetesSession(a.InstanceID))
	_, active, handshakes := f.control.snapshot()
	require.Contains(t, active, a.InstanceID)
	require.Equal(t, before, handshakes)
}

func TestKubernetesSharedCleanupPersistenceFailureSurvivesBackendRestart(t *testing.T) {
	f := newTaskPodFixture(t)
	a := f.launch(t, 1)
	id := getMetadataString(a.Metadata, MetadataKeyAuthTokenSecret)
	store := &failingUpdateSecretStore{inMemorySecretStore: f.secretStore.(*inMemorySecretStore), failUpdateFor: id}
	f.secretStore = store
	f.runtime.secretStore = store
	f.control.reboot()
	require.ErrorContains(t, f.runtime.StopInstance(context.Background(), a, true), "injected secret update failure")
	require.NotNil(t, f.runtime.currentKubernetesSession(a.InstanceID))
	token, err := store.Reveal(context.Background(), kubernetesControlRecoverySecretID(id))
	require.NoError(t, err)
	require.NotEmpty(t, token)
	store.failUpdateFor = ""
	f.restartBackend(t)
	require.NoError(t, f.runtime.StopInstance(context.Background(), a, true))
	_, _, handshakes := f.control.snapshot()
	require.Equal(t, 1, handshakes)
}

func TestKubernetesSharedCleanupDeletionFailureKeepsTracking(t *testing.T) {
	for _, status := range []int{http.StatusForbidden, http.StatusInternalServerError} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			f := newTaskPodFixture(t)
			a := f.launch(t, 1)
			original := f.runtime.currentKubernetesSession(a.InstanceID)
			_, _, before := f.control.snapshot()
			f.control.mu.Lock()
			f.control.deleteStatus = status
			f.control.mu.Unlock()
			require.Error(t, f.runtime.StopInstance(context.Background(), a, true))
			require.Same(t, original, f.runtime.currentKubernetesSession(a.InstanceID))
			_, active, handshakes := f.control.snapshot()
			require.Contains(t, active, a.InstanceID)
			require.Equal(t, before, handshakes, "403 mentioning auth must not consume a handshake")
			f.control.mu.Lock()
			f.control.deleteStatus = 0
			f.control.mu.Unlock()
			require.NoError(t, f.runtime.StopInstance(context.Background(), a, true))
		})
	}
}

func TestKubernetesSharedCleanupConcurrentSiblingRecovery(t *testing.T) {
	f := newTaskPodFixture(t)
	a := f.launch(t, 1)
	f.launch(t, 2)
	f.control.reboot()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	results := make(chan error, 2)
	go func() { results <- f.runtime.StopInstance(ctx, a, true) }()
	go func() { _, err := f.runtime.CreateInstance(ctx, taskPodRequest(3)); results <- err }()
	require.NoError(t, <-results)
	require.NoError(t, <-results)
	_, active, handshakes := f.control.snapshot()
	require.Equal(t, 1, handshakes)
	require.NotContains(t, active, a.InstanceID)
	require.Contains(t, active, "instance-3")
}

func TestKubernetesSharedCleanupMissingNonceRetainsRecovery(t *testing.T) {
	f := newTaskPodFixture(t)
	a := f.launch(t, 1)
	original := f.runtime.currentKubernetesSession(a.InstanceID)
	require.NoError(t, f.secretStore.Delete(context.Background(), getMetadataString(a.Metadata, MetadataKeyBootstrapNonceSecret)))
	f.control.reboot()
	require.Error(t, f.runtime.StopInstance(context.Background(), a, true))
	require.Same(t, original, f.runtime.currentKubernetesSession(a.InstanceID))
	_, _, handshakes := f.control.snapshot()
	require.Zero(t, handshakes)
}

func TestKubernetesSharedCleanupMissingNonceReferenceRetainsRecovery(t *testing.T) {
	f := newTaskPodFixture(t)
	a := f.launch(t, 1)
	original := f.runtime.currentKubernetesSession(a.InstanceID)
	delete(a.Metadata, MetadataKeyBootstrapNonceSecret)
	_, err := f.database.Exec(`UPDATE task_environment_kubernetes SET bootstrap_secret_id = '' WHERE environment_id = 'environment-1'`)
	require.NoError(t, err)
	f.control.reboot()
	require.ErrorContains(t, f.runtime.StopInstance(context.Background(), a, true), "bootstrap nonce is unavailable")
	require.Same(t, original, f.runtime.currentKubernetesSession(a.InstanceID))
	_, _, handshakes := f.control.snapshot()
	require.Zero(t, handshakes)
	require.Empty(t, f.resources.deletedPods)
	require.Empty(t, f.resources.deletedPVCs)
}

type unavailableRecoverySecretStore struct{ *failingUpdateSecretStore }

func (s *unavailableRecoverySecretStore) Create(context.Context, *secrets.SecretWithValue) error {
	return errors.New("injected recovery write failure")
}

func TestKubernetesSharedCleanupAllWritesFailKeepsPendingToken(t *testing.T) {
	f := newTaskPodFixture(t)
	a := f.launch(t, 1)
	id := getMetadataString(a.Metadata, MetadataKeyAuthTokenSecret)
	failing := &failingUpdateSecretStore{inMemorySecretStore: f.secretStore.(*inMemorySecretStore), failUpdateFor: id}
	f.runtime.secretStore = &unavailableRecoverySecretStore{failing}
	f.control.reboot()
	require.ErrorContains(t, f.runtime.StopInstance(context.Background(), a, true), "injected recovery write failure")
	require.NotNil(t, f.runtime.currentKubernetesSession(a.InstanceID))
	failing.failUpdateFor = ""
	require.NoError(t, f.runtime.StopInstance(context.Background(), a, true))
	_, _, handshakes := f.control.snapshot()
	require.Equal(t, 1, handshakes)
}

func TestKubernetesSharedCleanupCompletionCannotRetireReplacement(t *testing.T) {
	f := newTaskPodFixture(t)
	a := f.launch(t, 1)
	original := f.runtime.currentKubernetesSession(a.InstanceID)
	entered, release := make(chan struct{}), make(chan struct{})
	f.control.beforeDelete = func() { close(entered); <-release }
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	stopped := make(chan error, 1)
	go func() { stopped <- f.runtime.StopInstance(ctx, a, true) }()
	select {
	case <-entered:
	case <-ctx.Done():
		close(release)
		t.Fatal("stop did not reach deletion")
	}
	replacement := &kubernetesSession{runtime: original.runtime, request: original.request}
	f.runtime.mu.Lock()
	f.runtime.sessions[a.InstanceID] = replacement
	f.runtime.mu.Unlock()
	close(release)
	require.NoError(t, <-stopped)
	require.Same(t, replacement, f.runtime.currentKubernetesSession(a.InstanceID))
	require.NoError(t, closeKubernetesSessionResources(original))
}
