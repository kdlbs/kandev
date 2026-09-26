package backendapp

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/cursorcloud"
	"github.com/kandev/kandev/internal/secrets"
	"github.com/kandev/kandev/internal/task/models"
)

type cursorCloudDiscoveryReaderStub struct {
	executors    []*models.Executor
	profiles     map[string][]*models.ExecutorProfile
	executorsErr error
	profilesErr  error
}

func (s cursorCloudDiscoveryReaderStub) ListExecutors(context.Context) ([]*models.Executor, error) {
	return s.executors, s.executorsErr
}

func (s cursorCloudDiscoveryReaderStub) ListExecutorProfiles(_ context.Context, executorID string) ([]*models.ExecutorProfile, error) {
	return s.profiles[executorID], s.profilesErr
}

func cursorCloudDiscoveryFixture(t *testing.T) (*backendappSecretStore, *models.Executor, *models.ExecutorProfile) {
	t.Helper()
	store := newBackendappSecretStore()
	if err := store.Create(context.Background(), &secrets.SecretWithValue{
		Secret: secrets.Secret{ID: "cursor-key", Scope: secrets.ScopeGlobal}, Value: "test-key",
	}); err != nil {
		t.Fatal(err)
	}
	executor := &models.Executor{ID: "cloud-executor", Type: models.ExecutorTypeCursorCloud, Status: models.ExecutorStatusActive}
	profile := &models.ExecutorProfile{ID: "cloud-profile", ExecutorID: executor.ID, Config: map[string]string{
		cursorcloud.ExecutorConfigSecretID:    "cursor-key",
		cursorcloud.ExecutorConfigCallbackURL: "https://callback.example.test",
	}}
	return store, executor, profile
}

func TestCursorCloudAgentDiscovery(t *testing.T) {
	store, executor, profile := cursorCloudDiscoveryFixture(t)
	configured := cursorCloudDiscoveryReaderStub{
		executors: []*models.Executor{executor},
		profiles:  map[string][]*models.ExecutorProfile{executor.ID: {profile}},
	}
	cases := []struct {
		name   string
		reader cursorCloudDiscoveryReaderStub
		store  secrets.SecretStore
		want   bool
	}{
		{name: "absent", reader: cursorCloudDiscoveryReaderStub{}, store: store},
		{name: "incomplete profile", reader: cursorCloudDiscoveryReaderStub{
			executors: configured.executors,
			profiles:  map[string][]*models.ExecutorProfile{executor.ID: {{ID: "unsaved", Config: map[string]string{}}}},
		}, store: store},
		{name: "newly saved profile", reader: configured, store: store, want: true},
		{name: "last profile removed", reader: cursorCloudDiscoveryReaderStub{
			executors: configured.executors, profiles: map[string][]*models.ExecutorProfile{executor.ID: {}},
		}, store: store},
		{name: "inaccessible credential", reader: cursorCloudDiscoveryReaderStub{
			executors: configured.executors,
			profiles: map[string][]*models.ExecutorProfile{executor.ID: {{ID: "inaccessible", Config: map[string]string{
				cursorcloud.ExecutorConfigSecretID:    "missing",
				cursorcloud.ExecutorConfigCallbackURL: "https://callback.example.test",
			}}}},
		}, store: store},
		{name: "transient provider outage", reader: configured, store: store, want: true},
		{name: "profile query failure", reader: cursorCloudDiscoveryReaderStub{
			executors: configured.executors, profilesErr: errors.New("store unavailable"),
		}, store: store},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := cursorCloudAgentAvailable(context.Background(), tc.reader, tc.store); got != tc.want {
				t.Fatalf("cursorCloudAgentAvailable() = %v, want %v", got, tc.want)
			}
		})
	}
}
