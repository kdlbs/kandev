package secrets

import (
	"bytes"
	"context"
	"errors"
	"sync"
	"testing"
)

func TestSessionMCPKey_ReadBeforeEnsureIsNotFound(t *testing.T) {
	store := newTestSQLiteStore(t)
	if _, err := store.SessionMCPKey(context.Background()); !errors.Is(err, ErrNotFound) {
		t.Fatalf("read of absent key = %v, want ErrNotFound", err)
	}
}

func TestSessionMCPKey_EnsureCreatesOnceAndConcurrentFirstUsesAgree(t *testing.T) {
	store := newTestSQLiteStore(t)
	ctx := context.Background()
	const n = 16
	keys := make([][]byte, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			key, replaced, err := store.EnsureSessionMCPKey(ctx)
			if err != nil || replaced {
				t.Errorf("ensure: replaced=%v err=%v", replaced, err)
				return
			}
			keys[i] = key
		}()
	}
	wg.Wait()
	for i := 1; i < n; i++ {
		if len(keys[i]) != SessionMCPKeySize || !bytes.Equal(keys[0], keys[i]) {
			t.Fatalf("callers ended with different keys")
		}
	}
	got, err := store.SessionMCPKey(ctx)
	if err != nil || !bytes.Equal(got, keys[0]) {
		t.Fatalf("read after ensure: %v", err)
	}
}

func TestSessionMCPKey_HiddenFromUserSecretPaths(t *testing.T) {
	store := newTestSQLiteStore(t)
	ctx := context.Background()
	if _, _, err := store.EnsureSessionMCPKey(ctx); err != nil {
		t.Fatal(err)
	}
	list, err := store.List(ctx)
	if err != nil || len(list) != 0 {
		t.Fatalf("List = %v, %v; want empty", list, err)
	}
	scoped, err := store.ListScoped(ctx, SecretListOptions{Scope: ScopeGlobal})
	if err != nil || len(scoped) != 0 {
		t.Fatalf("ListScoped = %v, %v; want empty", scoped, err)
	}
	if _, err := store.Reveal(ctx, SessionMCPKeyID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Reveal = %v, want ErrNotFound", err)
	}
	if err := store.Delete(ctx, SessionMCPKeyID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Delete = %v, want ErrNotFound", err)
	}
}

func TestSessionMCPKey_UndecryptableAndWrongLengthAreReplaced(t *testing.T) {
	for name, corrupt := range map[string]func(*sqliteStore){
		"master key regenerated": func(s *sqliteStore) {
			s.crypto.key = bytes.Repeat([]byte{7}, MasterKeySize)
		},
		"stored key is not 32 bytes": func(s *sqliteStore) {
			ct, nonce, err := Encrypt([]byte("short"), s.crypto.Key())
			if err != nil {
				t.Fatal(err)
			}
			if _, err := s.db.Exec(`UPDATE session_mcp_key SET encrypted_value = ?, nonce = ?`, ct, nonce); err != nil {
				t.Fatal(err)
			}
		},
	} {
		t.Run(name, func(t *testing.T) {
			store := newTestSQLiteStore(t)
			ctx := context.Background()
			old, _, err := store.EnsureSessionMCPKey(ctx)
			if err != nil {
				t.Fatal(err)
			}
			corrupt(store)
			if _, err := store.SessionMCPKey(ctx); !errors.Is(err, ErrSessionKeyUnusable) {
				t.Fatalf("read of unusable key = %v, want ErrSessionKeyUnusable", err)
			}
			fresh, replaced, err := store.EnsureSessionMCPKey(ctx)
			if err != nil || !replaced || len(fresh) != SessionMCPKeySize || bytes.Equal(fresh, old) {
				t.Fatalf("ensure: replaced=%v err=%v", replaced, err)
			}
			again, replaced, err := store.EnsureSessionMCPKey(ctx)
			if err != nil || replaced || !bytes.Equal(again, fresh) {
				t.Fatalf("second ensure replaced=%v err=%v", replaced, err)
			}
		})
	}
}

func TestSessionMCPKey_StoreFailureIsNotMistakenForAbsentOrUnusable(t *testing.T) {
	store := newTestSQLiteStore(t)
	if _, err := store.db.Exec(`DROP TABLE session_mcp_key`); err != nil {
		t.Fatal(err)
	}
	_, err := store.SessionMCPKey(context.Background())
	if err == nil || errors.Is(err, ErrNotFound) || errors.Is(err, ErrSessionKeyUnusable) {
		t.Fatalf("store failure classified as %v", err)
	}
	if _, _, err := store.EnsureSessionMCPKey(context.Background()); err == nil {
		t.Fatal("ensure must fail on a store failure")
	}
}
