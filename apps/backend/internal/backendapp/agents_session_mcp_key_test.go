package backendapp

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
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
	return mgr
}

func TestWireSessionMCPKeyEnsuresTheKeyBeforeRecovery(t *testing.T) {
	store := &keyedSecretStore{}
	exec := lifecycle.NewStandaloneExecutor(nil, "127.0.0.1", 0, newTestLogger())
	wireSessionMCPKey(exec, newKeyWiringManager(t, store), store, newTestLogger())
	if got := store.ensured.Load(); got != 1 {
		t.Fatalf("key ensured %d times at startup, want 1", got)
	}
}

func TestWireSessionMCPKeyStoreFailureDoesNotPanicOrBlockStartup(t *testing.T) {
	store := &keyedSecretStore{err: errors.New("disk full")}
	exec := lifecycle.NewStandaloneExecutor(nil, "127.0.0.1", 0, newTestLogger())
	wireSessionMCPKey(exec, newKeyWiringManager(t, store), store, newTestLogger())
	if store.ensured.Load() != 1 {
		t.Fatal("the failing ensure must still have been attempted once")
	}
}

func TestWireSessionMCPKeyWithoutKeyStoreLeavesTokensDisabled(t *testing.T) {
	store := plainSecretStore{}
	exec := lifecycle.NewStandaloneExecutor(nil, "127.0.0.1", 0, newTestLogger())
	wireSessionMCPKey(exec, newKeyWiringManager(t, store), store, newTestLogger())
}
