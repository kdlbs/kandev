package lifecycle

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/kandev/kandev/internal/secrets"
)

// mockSecretStore implements secrets.SecretStore for testing resolveTokenFromMetadata.
type mockSecretStore struct {
	store map[string]string
	err   error
}

var _ secrets.SecretStore = (*mockSecretStore)(nil)

func (m *mockSecretStore) Create(_ context.Context, _ *secrets.SecretWithValue) error { return nil }
func (m *mockSecretStore) Get(_ context.Context, _ string) (*secrets.Secret, error)   { return nil, nil }
func (m *mockSecretStore) Reveal(_ context.Context, id string) (string, error) {
	if m.err != nil {
		return "", m.err
	}
	return m.store[id], nil
}
func (m *mockSecretStore) Update(_ context.Context, _ string, _ *secrets.UpdateSecretRequest) error {
	return nil
}
func (m *mockSecretStore) Delete(_ context.Context, _ string) error                  { return nil }
func (m *mockSecretStore) List(_ context.Context) ([]*secrets.SecretListItem, error) { return nil, nil }
func (m *mockSecretStore) Close() error                                              { return nil }

func newTestSpritesExecutor(store secrets.SecretStore) *SpritesExecutor {
	return &SpritesExecutor{
		secretStore: store,
		logger:      newTestLogger(),
		tokens:      make(map[string]string),
		proxies:     make(map[string]*SpritesProxySession),
		mu:          sync.RWMutex{},
	}
}

func TestSpritesReconnectSetupEnvironmentDoesNotEmitSkippedSetupSteps(t *testing.T) {
	r := newTestSpritesExecutor(nil)
	var reported []PrepareStep

	err := r.stepSetupEnvironment(context.Background(), nil, &ExecutorCreateRequest{}, true, func(_ spritesStepKey, step PrepareStep) {
		reported = append(reported, step)
	})

	if err != nil {
		t.Fatalf("stepSetupEnvironment returned error: %v", err)
	}
	if len(reported) != 0 {
		t.Fatalf("expected reconnect setup to emit no upload/credentials/prepare steps, got %d: %#v", len(reported), reported)
	}
}

func TestSpritesAgentInstanceIDPrefersCurrentLaunchInstance(t *testing.T) {
	req := &ExecutorCreateRequest{
		InstanceID:          "current-session-exec",
		PreviousExecutionID: "previous-session-exec",
	}

	if got := spritesAgentInstanceID(req); got != "current-session-exec" {
		t.Fatalf("spritesAgentInstanceID() = %q, want current session instance", got)
	}
}

func TestSpritesShouldReconnectWhenSpriteNameMetadataExists(t *testing.T) {
	req := &ExecutorCreateRequest{
		InstanceID: "current-session-exec",
		Metadata: map[string]interface{}{
			"sprite_name": "kandev-existing-sprite",
		},
	}

	if !spritesShouldReconnect(req) {
		t.Fatal("expected sprite_name metadata to trigger reconnect")
	}
}

func TestSpritesProgressPlanReconnectOmitsSetupAndNetworkSteps(t *testing.T) {
	plan := newSpritesProgressPlan(true)

	if plan.total() != 3 {
		t.Fatalf("reconnect runtime plan has %d steps, want 3", plan.total())
	}
	for _, key := range []spritesStepKey{
		spriteStepUploadAgentctl,
		spriteStepUploadCredentials,
		spriteStepRunPrepareScript,
		spriteStepApplyNetworkPolicy,
	} {
		if plan.has(key) {
			t.Fatalf("reconnect runtime plan unexpectedly includes %s", key)
		}
	}
	if got := plan.index(spriteStepWaitHealthy); got != 1 {
		t.Fatalf("wait healthy index = %d, want 1", got)
	}
	if got := plan.index(spriteStepAgentInstance); got != 2 {
		t.Fatalf("agent instance index = %d, want 2", got)
	}
}

func TestResolveTokenFromMetadata(t *testing.T) {
	t.Run("nil secret store returns empty", func(t *testing.T) {
		r := &SpritesExecutor{
			tokens: make(map[string]string),
		}
		got := r.resolveTokenFromMetadata(context.Background(), &ExecutorInstance{
			InstanceID: "inst-1",
			Metadata:   map[string]interface{}{"env_secret_id_SPRITES_API_TOKEN": "secret-1"},
		})
		if got != "" {
			t.Fatalf("expected empty, got %q", got)
		}
	})

	t.Run("nil instance returns empty", func(t *testing.T) {
		r := newTestSpritesExecutor(&mockSecretStore{store: map[string]string{"s1": "tok"}})
		got := r.resolveTokenFromMetadata(context.Background(), nil)
		if got != "" {
			t.Fatalf("expected empty, got %q", got)
		}
	})

	t.Run("no secret ID in metadata returns empty", func(t *testing.T) {
		r := newTestSpritesExecutor(&mockSecretStore{store: map[string]string{"s1": "tok"}})
		got := r.resolveTokenFromMetadata(context.Background(), &ExecutorInstance{
			InstanceID: "inst-1",
			Metadata:   map[string]interface{}{},
		})
		if got != "" {
			t.Fatalf("expected empty, got %q", got)
		}
	})

	t.Run("secret store error returns empty", func(t *testing.T) {
		r := newTestSpritesExecutor(&mockSecretStore{err: fmt.Errorf("vault sealed")})
		got := r.resolveTokenFromMetadata(context.Background(), &ExecutorInstance{
			InstanceID: "inst-1",
			Metadata:   map[string]interface{}{"env_secret_id_SPRITES_API_TOKEN": "secret-1"},
		})
		if got != "" {
			t.Fatalf("expected empty, got %q", got)
		}
	})

	t.Run("secret store returns empty value", func(t *testing.T) {
		r := newTestSpritesExecutor(&mockSecretStore{store: map[string]string{"secret-1": ""}})
		got := r.resolveTokenFromMetadata(context.Background(), &ExecutorInstance{
			InstanceID: "inst-1",
			Metadata:   map[string]interface{}{"env_secret_id_SPRITES_API_TOKEN": "secret-1"},
		})
		if got != "" {
			t.Fatalf("expected empty, got %q", got)
		}
	})

	t.Run("valid secret returns token and caches it", func(t *testing.T) {
		r := newTestSpritesExecutor(&mockSecretStore{store: map[string]string{"secret-1": "my-token"}})
		got := r.resolveTokenFromMetadata(context.Background(), &ExecutorInstance{
			InstanceID: "inst-1",
			Metadata:   map[string]interface{}{"env_secret_id_SPRITES_API_TOKEN": "secret-1"},
		})
		if got != "my-token" {
			t.Fatalf("expected my-token, got %q", got)
		}
		// Verify token is cached
		r.mu.RLock()
		cached := r.tokens["inst-1"]
		r.mu.RUnlock()
		if cached != "my-token" {
			t.Fatalf("expected cached token my-token, got %q", cached)
		}
	})
}
