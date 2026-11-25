package sshconnections

import (
	"errors"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"testing"
)

func TestRuntimeAdmissionPinsRouteAndRequiresSavedBinding(t *testing.T) {
	s, ctx := identityService(t)
	for _, q := range []string{`CREATE TABLE executors(id TEXT PRIMARY KEY, config TEXT, deleted_at TIMESTAMP,type TEXT DEFAULT 'ssh')`, `CREATE TABLE task_sessions(id TEXT PRIMARY KEY,executor_id TEXT)`, `INSERT INTO executors(id,config) VALUES ('executor','{"ssh_connection_id":"target"}')`, `INSERT INTO task_sessions(id,executor_id) VALUES ('session','executor')`} {
		if _, err := s.store.db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	identity, err := s.CreateIdentity(ctx, CreateIdentityRequest{RequestID: uuid.NewString(), Label: "key", Kind: "generate"})
	if err != nil {
		t.Fatal(err)
	}
	c, err := s.store.SaveConnection(ctx, Connection{ID: "target", Host: "host", Port: 22, User: "user", IdentityID: identity.ID}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.AdmitRuntime(ctx, "session", "target", ""); !errors.Is(err, ErrInvalidRoute) {
		t.Fatalf("unready admission: %v", err)
	}
	if err = s.store.write(ctx, func(tx *sqlx.Tx) error {
		r, e := routeIn(ctx, tx, c.ID)
		if e != nil {
			return e
		}
		c = r.Destination
		c.VerifiedRoute = r.Stamp
		if _, e = tx.Exec(`UPDATE executors SET config=? WHERE id='executor'`, `{"ssh_connection_id":"target","ssh_connection_stamp":"`+r.Stamp+`"}`); e != nil {
			return e
		}
		return putConnection(ctx, tx, c)
	}); err != nil {
		t.Fatal(err)
	}
	if _, err = s.AdmitRuntime(ctx, "missing", "target", ""); err == nil {
		t.Fatal("unknown session admitted")
	}
	if _, err = s.AdmitRuntime(ctx, "session", "other", ""); err == nil {
		t.Fatal("override admitted")
	}
	snapshot, err := s.AdmitRuntime(ctx, "session", "target", "")
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.IdentityGeneration != identity.Generation || snapshot.Stamp == "" {
		t.Fatal("missing identity or route snapshot")
	}
	changed := c
	changed.Host = "different"
	if _, err = s.store.SaveConnection(ctx, changed, c.Version); !errors.Is(err, ErrInUse) {
		t.Fatalf("active route changed: %v", err)
	}
	if _, err = s.AdmitRuntime(ctx, "session", "target", "stale"); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale resume: %v", err)
	}
	if _, err = s.AdmitRuntime(ctx, "session", "target", snapshot.Stamp); err != nil {
		t.Fatal(err)
	}
}

func TestRuntimeReferenceReleaseRollsBackWithRecordDeletion(t *testing.T) {
	s, ctx := identityService(t)
	if _, err := s.store.db.Exec(`INSERT INTO ssh_connection_refs(owner_type,owner_id,connection_id,route_stamp) VALUES ('runtime','session','target','stamp')`); err != nil {
		t.Fatal(err)
	}
	tx, err := s.store.db.BeginTxx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.ReleaseRuntimeIn(ctx, tx, "session"); err != nil {
		t.Fatal(err)
	}
	if err = tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	var count int
	if err = s.store.db.Get(&count, `SELECT COUNT(*) FROM ssh_connection_refs`); err != nil || count != 1 {
		t.Fatalf("rolled back deletion lost protection: %d %v", count, err)
	}
	tx, err = s.store.db.BeginTxx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.ReleaseRuntimeIn(ctx, tx, "session"); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if err = s.store.db.Get(&count, `SELECT COUNT(*) FROM ssh_connection_refs`); err != nil || count != 0 {
		t.Fatalf("deleted record retained reference: %d %v", count, err)
	}
}
