package sshconnections

import (
	"context"
	"errors"
	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/db"
	"path/filepath"
	"testing"
)

func testStore(t *testing.T) *Store {
	t.Helper()
	conn, err := db.OpenSQLite(filepath.Join(t.TempDir(), "connections.db"))
	if err != nil {
		t.Fatal(err)
	}
	sql := sqlx.NewDb(conn, "sqlite3")
	t.Cleanup(func() { _ = sql.Close() })
	store, err := NewStore(sql)
	if err != nil {
		t.Fatal(err)
	}
	return store
}
func TestConnectionVersionsAndSingleHopGuards(t *testing.T) {
	testConnectionVersions(t, testStore(t))
}
func testConnectionVersions(t *testing.T, s *Store) {
	ctx := context.Background()
	direct, err := s.SaveConnection(ctx, Connection{ID: "b", Name: "bastion", Host: "b.example", Port: 22, User: "user"}, 0)
	if err != nil {
		t.Fatal(err)
	}
	target, err := s.SaveConnection(ctx, Connection{ID: "d", Name: "destination", Host: "d.example", Port: 22, User: "user", BastionID: direct.ID}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if direct.Version != 1 || target.Revision != 1 {
		t.Fatal("missing initial versions")
	}
	direct.Name = "renamed"
	updated, err := s.SaveConnection(ctx, direct, 1)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Revision != direct.Revision || updated.Version != 2 {
		t.Fatal("label edit changed transport or failed to version row")
	}
	if _, err = s.SaveConnection(ctx, direct, 1); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale write = %v", err)
	}
	updated.BastionID = target.ID
	if _, err = s.SaveConnection(ctx, updated, 2); !errors.Is(err, ErrInvalidRoute) {
		t.Fatalf("cycle = %v", err)
	}
	if _, err = s.SaveConnection(ctx, Connection{ID: "chain", BastionID: target.ID}, 0); !errors.Is(err, ErrInvalidRoute) {
		t.Fatalf("chain = %v", err)
	}
	if err = s.DeleteConnection(ctx, direct.ID, 2); !errors.Is(err, ErrInUse) {
		t.Fatalf("referenced deletion = %v", err)
	}
	target.Host = "other.example"
	target, err = s.SaveConnection(ctx, target, 1)
	if err != nil {
		t.Fatal(err)
	}
	if target.Revision != 2 || target.Fingerprint != "" || target.VerifiedRoute != "" {
		t.Fatal("transport edit must invalidate trust and readiness")
	}
	if err = s.DeleteConnection(ctx, target.ID, target.Version); err != nil {
		t.Fatal(err)
	}
	if err = s.DeleteConnection(ctx, direct.ID, 2); err != nil {
		t.Fatal(err)
	}
}

func TestBastionRevisionInvalidatesDestinationRoute(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	b, err := s.SaveConnection(ctx, Connection{ID: "b", Host: "bastion", Port: 22}, 0)
	if err != nil {
		t.Fatal(err)
	}
	d, err := s.SaveConnection(ctx, Connection{ID: "d", Host: "destination", Port: 22, BastionID: b.ID}, 0)
	if err != nil {
		t.Fatal(err)
	}
	var first Route
	err = s.write(ctx, func(tx *sqlx.Tx) error { var err error; first, err = routeIn(ctx, tx, d.ID); return err })
	if err != nil {
		t.Fatal(err)
	}
	b.Host = "new-bastion"
	if _, err = s.SaveConnection(ctx, b, b.Version); err != nil {
		t.Fatal(err)
	}
	err = s.write(ctx, func(tx *sqlx.Tx) error {
		next, err := routeIn(ctx, tx, d.ID)
		if err != nil {
			return err
		}
		if next.Stamp == first.Stamp {
			t.Fatal("bastion edit did not invalidate route stamp")
		}
		_, err = tx.Exec(`INSERT INTO ssh_connection_refs(owner_type,owner_id,connection_id,route_stamp) VALUES ('runtime','run','d','stamp')`)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	b, err = s.GetConnection(ctx, b.ID)
	if err != nil {
		t.Fatal(err)
	}
	b.Host = "third"
	if _, err = s.SaveConnection(ctx, b, b.Version); !errors.Is(err, ErrInUse) {
		t.Fatalf("edited bastion used by active destination: %v", err)
	}
	b.Host = "new-bastion"
	b.Name = "label remains editable"
	if _, err = s.SaveConnection(ctx, b, b.Version); err != nil {
		t.Fatal(err)
	}
}
