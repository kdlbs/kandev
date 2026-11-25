package sshconnections

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/secrets"
	"github.com/kandev/kandev/internal/testutil"
)

func postgresIdentityService(t *testing.T) (*Service, context.Context) {
	t.Helper()
	database := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	store, err := NewStore(database)
	if err != nil {
		t.Fatal(err)
	}
	key, err := secrets.NewMasterKeyProvider(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = secrets.Provide(database, database, key); err != nil {
		t.Fatal(err)
	}
	return NewService(true, store, secrets.NewSSHIdentityVault(key)), authn.WithIdentity(context.Background(), authn.Identity{UserID: "admin", Role: authn.RoleAdmin})
}

// @covers AC-EXECUTORS-SSH-CONNECTIONS-001.3, AC-EXECUTORS-SSH-CONNECTIONS-001.6
func TestPostgresIdentityTransactions(t *testing.T) {
	for _, operation := range []string{"create", "delete"} {
		t.Run(operation, func(t *testing.T) {
			s, ctx := postgresIdentityService(t)
			req := CreateIdentityRequest{RequestID: uuid.NewString(), Label: "fixture", Kind: "generate"}
			var identity Identity
			var err error
			if operation == "delete" {
				identity, err = s.CreateIdentity(ctx, req)
				if err != nil {
					t.Fatal(err)
				}
			}
			if _, err = s.store.db.Exec(`DROP TABLE ssh_connection_audit`); err != nil {
				t.Fatal(err)
			}
			if operation == "create" {
				_, err = s.CreateIdentity(ctx, req)
			} else {
				err = s.DeleteIdentity(ctx, identity.ID, identity.Version)
			}
			if err == nil {
				t.Fatal("mutation succeeded without its audit transaction")
			}
			want := 0
			if operation == "delete" {
				want = 1
			}
			for _, table := range []string{"ssh_identities", "ssh_identity_requests", "secrets"} {
				var count int
				if err = s.store.db.Get(&count, "SELECT COUNT(*) FROM "+table); err != nil || count != want {
					t.Fatalf("%s count=%d want=%d error=%v", table, count, want, err)
				}
			}
			if operation == "delete" {
				tx, err := s.store.db.BeginTxx(ctx, nil)
				if err != nil {
					t.Fatal(err)
				}
				defer func() { _ = tx.Rollback() }()
				if _, err = s.vault.Reveal(ctx, tx, "ssh-identity:"+identity.ID); err != nil {
					t.Fatal("rollback lost encrypted key")
				}
			}
		})
	}
}

func TestPostgresConcurrentIdentityIdempotency(t *testing.T) {
	s, ctx := postgresIdentityService(t)
	req := CreateIdentityRequest{RequestID: uuid.NewString(), Label: "fixture", Kind: "generate"}
	var results [2]Identity
	var errs [2]error
	start := make(chan struct{})
	var done sync.WaitGroup
	for i := range 2 {
		done.Add(1)
		go func() { defer done.Done(); <-start; results[i], errs[i] = s.CreateIdentity(ctx, req) }()
	}
	close(start)
	done.Wait()
	for _, err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if results[0].ID == "" || results[0] != results[1] {
		t.Fatal("concurrent retry produced different keys")
	}
	for _, table := range []string{"ssh_identities", "ssh_identity_requests", "secrets", "ssh_connection_audit"} {
		var count int
		if err := s.store.db.Get(&count, "SELECT COUNT(*) FROM "+table); err != nil || count != 1 {
			t.Fatalf("%s count=%d error=%v", table, count, err)
		}
	}
}

// @covers AC-EXECUTORS-SSH-CONNECTIONS-002.8, AC-EXECUTORS-SSH-CONNECTIONS-005.1
func TestPostgresBindingAndRuntimeTransactions(t *testing.T) {
	s, ctx := postgresIdentityService(t)
	identity, err := s.CreateIdentity(ctx, CreateIdentityRequest{RequestID: uuid.NewString(), Label: "fixture", Kind: "generate"})
	if err != nil {
		t.Fatal(err)
	}
	c, err := s.store.SaveConnection(ctx, Connection{ID: "target", Host: "fixture.invalid", User: "fixture", Port: 22, IdentityID: identity.ID}, 0)
	if err != nil {
		t.Fatal(err)
	}
	var route Route
	if err = s.store.write(ctx, func(tx *sqlx.Tx) error {
		var err error
		route, err = routeIn(ctx, tx, c.ID)
		if err != nil {
			return err
		}
		c = route.Destination
		c.VerifiedRoute = route.Stamp
		return putConnection(ctx, tx, c)
	}); err != nil {
		t.Fatal(err)
	}
	config := map[string]string{ConnectionConfigKey: c.ID, "ssh_connection_stamp": route.Stamp}
	for _, commit := range []bool{false, true} {
		tx, err := s.store.db.BeginTxx(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		if err = s.BindExecutorIn(ctx, tx, "executor", "ssh", config); err != nil {
			_ = tx.Rollback()
			t.Fatal(err)
		}
		if commit {
			err = tx.Commit()
		} else {
			err = tx.Rollback()
		}
		if err != nil {
			t.Fatal(err)
		}
		var count int
		want := 0
		if commit {
			want = 1
		}
		if err = s.store.db.Get(&count, `SELECT COUNT(*) FROM ssh_connection_refs`); err != nil || count != want {
			t.Fatalf("binding rollback/commit: %d %v", count, err)
		}
	}
	for _, q := range []string{
		`CREATE TABLE executors(id TEXT PRIMARY KEY,config TEXT,deleted_at TIMESTAMP,type TEXT DEFAULT 'ssh')`,
		`CREATE TABLE task_sessions(id TEXT PRIMARY KEY,executor_id TEXT,task_id TEXT)`,
		`INSERT INTO executors (id,config,deleted_at) VALUES ('executor','{"ssh_connection_id":"target","ssh_connection_stamp":"` + route.Stamp + `"}',NULL)`,
		`INSERT INTO task_sessions VALUES ('session','executor','task')`,
	} {
		if _, err = s.store.db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	snapshot, err := s.AdmitRuntime(ctx, "session", c.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	changed := c
	changed.User = "different"
	if _, err = s.store.SaveConnection(ctx, changed, c.Version); !errors.Is(err, ErrInUse) {
		t.Fatalf("admitted route changed: %v", err)
	}
	if _, err = s.AdmitRuntime(ctx, "session", c.ID, "stale"); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale resume: %v", err)
	}
	for _, commit := range []bool{false, true} {
		tx, err := s.store.db.BeginTxx(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		if err = s.ReleaseRuntimeIn(ctx, tx, snapshot.SessionID); err != nil {
			_ = tx.Rollback()
			t.Fatal(err)
		}
		if commit {
			err = tx.Commit()
		} else {
			err = tx.Rollback()
		}
		if err != nil {
			t.Fatal(err)
		}
		var count int
		want := 1
		if commit {
			want = 0
		}
		if err = s.store.db.Get(&count, `SELECT COUNT(*) FROM ssh_connection_refs WHERE owner_type='runtime_pending'`); err != nil || count != want {
			t.Fatalf("runtime rollback/commit: %d %v", count, err)
		}
	}
	if _, err = s.store.SaveConnection(ctx, changed, c.Version); err != nil {
		t.Fatal(err)
	}
}

func TestPostgresEnrollmentJournal(t *testing.T) {
	s, ctx := postgresIdentityService(t)
	testEnrollmentJournal(t, s, ctx)
}
func TestPostgresEnrollmentRecovery(t *testing.T) {
	s, ctx := postgresIdentityService(t)
	testEnrollmentRecovery(t, s, ctx)
}
func TestPostgresIdentityDependencies(t *testing.T) {
	s, ctx := postgresIdentityService(t)
	testIdentityDependencies(t, s, ctx)
}

func TestPostgresConcurrentBastionEditsRejectCycle(t *testing.T) {
	s, ctx := postgresIdentityService(t)
	a, err := s.store.SaveConnection(ctx, Connection{ID: "a"}, 0)
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.store.SaveConnection(ctx, Connection{ID: "b"}, 0)
	if err != nil {
		t.Fatal(err)
	}
	a.BastionID = b.ID
	b.BastionID = a.ID
	var errs [2]error
	start := make(chan struct{})
	var done sync.WaitGroup
	for i, c := range []Connection{a, b} {
		done.Add(1)
		go func() { defer done.Done(); <-start; _, errs[i] = s.store.SaveConnection(ctx, c, c.Version) }()
	}
	close(start)
	done.Wait()
	successes := 0
	for _, err := range errs {
		if err == nil {
			successes++
		} else if !errors.Is(err, ErrInvalidRoute) {
			t.Fatal(err)
		}
	}
	if successes != 1 {
		t.Fatalf("concurrent graph updates accepted=%d", successes)
	}
	for _, id := range []string{a.ID, b.ID} {
		if _, err = s.GetConnection(ctx, id); err != nil {
			t.Fatalf("committed invalid route: %v", err)
		}
	}
}
