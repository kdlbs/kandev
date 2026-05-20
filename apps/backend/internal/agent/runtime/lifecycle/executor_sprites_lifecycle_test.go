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

// TestSpritesStopInstancePreservesSandboxOnSessionStop locks in the resume-fast
// invariant: a plain agent stop must not destroy the upstream sandbox.
// Otherwise the next resume always falls into the missing-sandbox fallback,
// re-runs prepare, and loses the working tree the user had inside the sandbox.
func TestSpritesStopInstancePreservesSandboxOnSessionStop(t *testing.T) {
	r := newTestSpritesExecutor(nil)
	r.tokens["inst-1"] = "tok"

	preserveReasons := []string{
		"",
		"stopped via API",
		"agent crashed",
		"user requested",
	}

	for _, reason := range preserveReasons {
		t.Run(reason, func(t *testing.T) {
			err := r.StopInstance(context.Background(), &ExecutorInstance{
				InstanceID: "inst-1",
				Metadata:   map[string]interface{}{"sprite_name": "kandev-abc"},
				StopReason: reason,
			}, false)
			if err != nil {
				t.Fatalf("StopInstance: %v", err)
			}
			// Token cache is NOT cleared on a preserving stop; we'll need it
			// to talk to the same sandbox on resume.
			r.mu.RLock()
			tok := r.tokens["inst-1"]
			r.mu.RUnlock()
			if tok != "tok" {
				t.Fatalf("token cache cleared on preserving stop (reason=%q)", reason)
			}
		})
	}
}

// TestSpritesStopThenResumeReconnects proves the end-to-end integration of
// the resume-fast path: a preserving StopInstance leaves enough state on the
// instance metadata for the next launch to (a) decide it's a reconnect and
// (b) build the 3-step reconnect plan instead of the full 7-step bootstrap.
// The pieces are unit-tested individually elsewhere; this test guards
// against the wiring drifting (e.g. StopInstance accidentally stripping
// sprite_name, or the reconnect predicate forgetting to look at metadata).
func TestSpritesStopThenResumeReconnects(t *testing.T) {
	r := newTestSpritesExecutor(nil)

	stopped := &ExecutorInstance{
		InstanceID: "inst-stop-resume",
		Metadata:   map[string]interface{}{"sprite_name": "kandev-keep-me"},
		StopReason: "stopped via API",
	}
	if err := r.StopInstance(context.Background(), stopped, false); err != nil {
		t.Fatalf("StopInstance: %v", err)
	}

	// Mirror what executor_resume.go does: it carries forward the previous
	// execution's sprite_name into the next launch request's Metadata. If
	// StopInstance had cleared sprite_name, this map would be empty.
	resume := &ExecutorCreateRequest{
		Metadata:            map[string]interface{}{"sprite_name": stopped.Metadata["sprite_name"]},
		PreviousExecutionID: "previous-exec",
	}

	if !spritesShouldReconnect(resume) {
		t.Fatal("resume after preserving stop must be detected as a reconnect")
	}

	plan := newSpritesProgressPlan(spritesShouldReconnect(resume))
	if plan.total() != 3 {
		t.Fatalf("reconnect plan total = %d, want 3 (got: %v)", plan.total(), plan.steps)
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
	// Assert ordering rather than absolute indexes — the plan can grow new
	// steps without invalidating the contract this test guards (create →
	// wait healthy → agent instance must run in that order).
	for _, key := range []spritesStepKey{spriteStepCreateSprite, spriteStepWaitHealthy, spriteStepAgentInstance} {
		if !plan.has(key) {
			t.Fatalf("reconnect plan missing required step %s", key)
		}
	}
	assertPlanOrder(t, plan, spriteStepCreateSprite, spriteStepWaitHealthy)
	assertPlanOrder(t, plan, spriteStepWaitHealthy, spriteStepAgentInstance)
}

func assertPlanOrder(t *testing.T, plan *spritesProgressPlan, before, after spritesStepKey) {
	t.Helper()
	if plan.index(before) >= plan.index(after) {
		t.Fatalf("expected %s to run before %s, got indexes %d / %d",
			before, after, plan.index(before), plan.index(after))
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
