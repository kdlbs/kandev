package backendapp

import (
	"context"
	"errors"
	"slices"
	"sync/atomic"
	"testing"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/common/config"
	"github.com/kandev/kandev/internal/secrets"
)

type plainSecretStore struct{ secrets.SecretStore }

type keyedSecretStore struct {
	secrets.SecretStore
	ensured atomic.Int32
	err     error
}

func (s *keyedSecretStore) SessionMCPKey(context.Context) ([]byte, error) {
	return make([]byte, secrets.SessionMCPKeySize), nil
}

func (s *keyedSecretStore) EnsureSessionMCPKey(context.Context) ([]byte, bool, error) {
	s.ensured.Add(1)
	return make([]byte, secrets.SessionMCPKeySize), false, s.err
}

func newKeyWiringManager(t *testing.T, store secrets.SecretStore) *lifecycle.Manager {
	t.Helper()
	mgr := lifecycle.NewManager(nil, nil, nil, nil, nil, nil, lifecycle.ExecutorFallbackWarn, t.TempDir(), newTestLogger())
	mgr.SetSecretStore(store)
	mgr.SetSessionMCPPolicy(lifecycle.NewSessionMCPPolicy(true))
	return mgr
}

func TestWireSessionMCPKeyEnsuresTheKeyBeforeRecovery(t *testing.T) {
	store := &keyedSecretStore{}
	exec := lifecycle.NewStandaloneExecutor(nil, "127.0.0.1", 0, newTestLogger())
	if err := wireSessionMCPKey(lifecycle.NewSessionMCPPolicy(true), exec, newKeyWiringManager(t, store), store, newTestLogger()); err != nil {
		t.Fatal(err)
	}
	if got := store.ensured.Load(); got != 1 {
		t.Fatalf("key ensured %d times at startup, want 1", got)
	}
}

func TestWireSessionMCPKeyStoreFailureDoesNotPanicOrBlockStartup(t *testing.T) {
	store := &keyedSecretStore{err: errors.New("disk full")}
	exec := lifecycle.NewStandaloneExecutor(nil, "127.0.0.1", 0, newTestLogger())
	if err := wireSessionMCPKey(lifecycle.NewSessionMCPPolicy(true), exec, newKeyWiringManager(t, store), store, newTestLogger()); err != nil {
		t.Fatal(err)
	}
	if store.ensured.Load() != 1 {
		t.Fatal("the failing ensure must still have been attempted once")
	}
}

func TestWireSessionMCPKeyEnabledWithoutKeyStoreStopsStartup(t *testing.T) {
	store := plainSecretStore{}
	exec := lifecycle.NewStandaloneExecutor(nil, "127.0.0.1", 0, newTestLogger())
	err := wireSessionMCPKey(lifecycle.NewSessionMCPPolicy(true), exec, newKeyWiringManager(t, store), store, newTestLogger())
	if err == nil {
		t.Fatal("an enabled capability with no key store must stop startup")
	}
}

// With the flag off the key is neither created nor read, and a secret store
// that cannot hold it is irrelevant.
func TestWireSessionMCPKeyDisabledDoesNothing(t *testing.T) {
	store := &keyedSecretStore{}
	exec := lifecycle.NewStandaloneExecutor(nil, "127.0.0.1", 0, newTestLogger())
	if err := wireSessionMCPKey(lifecycle.NewSessionMCPPolicy(false), exec, newKeyWiringManager(t, store), store, newTestLogger()); err != nil {
		t.Fatal(err)
	}
	if got := store.ensured.Load(); got != 0 {
		t.Fatalf("key ensured %d times with the flag off, want 0", got)
	}
	plain := plainSecretStore{}
	if err := wireSessionMCPKey(lifecycle.NewSessionMCPPolicy(false), exec, newKeyWiringManager(t, plain), plain, newTestLogger()); err != nil {
		t.Fatalf("plain store with the flag off: %v", err)
	}
}

func TestSessionMCPPolicyFollowsTheFlagAndSelectsAdoptionCapabilities(t *testing.T) {
	cfg := &config.Config{}
	off := sessionMCPPolicyFromConfig(cfg)
	if off.Enabled() || slices.Contains(off.RequiredCapabilities(), "session_mcp_token") {
		t.Fatalf("flag off: enabled=%v caps=%v", off.Enabled(), off.RequiredCapabilities())
	}
	cfg.Features.SessionMCPToken = true
	on := sessionMCPPolicyFromConfig(cfg)
	if !on.Enabled() || !slices.Contains(on.RequiredCapabilities(), "session_mcp_token") {
		t.Fatalf("flag on: enabled=%v caps=%v", on.Enabled(), on.RequiredCapabilities())
	}
}
