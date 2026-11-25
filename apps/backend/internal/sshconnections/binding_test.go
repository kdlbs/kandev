package sshconnections

import (
	"context"
	"errors"
	"github.com/jmoiron/sqlx"
	"testing"
)

func TestExecutorBindingIsAtomicAndProtectsConnections(t *testing.T) {
	s, ctx := identityService(t)
	c, err := s.store.SaveConnection(ctx, Connection{ID: "target", Host: "host", Port: 22, User: "user"}, 0)
	if err != nil {
		t.Fatal(err)
	}
	bind := func(ctx context.Context, config map[string]string, rollback bool) error {
		tx, err := s.store.db.BeginTxx(ctx, nil)
		if err != nil {
			return err
		}
		defer func() { _ = tx.Rollback() }()
		if err = s.BindExecutorIn(ctx, tx, "executor", "ssh", config); err != nil {
			return err
		}
		if rollback {
			return nil
		}
		return tx.Commit()
	}
	config := map[string]string{"ssh_connection_id": c.ID}
	if err = bind(ctx, config, false); !errors.Is(err, ErrInvalidRoute) {
		t.Fatalf("unready binding: %v", err)
	}
	if err = s.store.write(ctx, func(tx *sqlx.Tx) error {
		r, e := routeIn(ctx, tx, c.ID)
		if e != nil {
			return e
		}
		c = r.Destination
		c.VerifiedRoute = r.Stamp
		return putConnection(ctx, tx, c)
	}); err != nil {
		t.Fatal(err)
	}
	r, err := s.readRoute(ctx, c.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	config["ssh_connection_stamp"] = r.Stamp
	if err = bind(context.Background(), config, false); !errors.Is(err, ErrForbidden) {
		t.Fatalf("unauthorized binding: %v", err)
	}
	mixed := map[string]string{"ssh_connection_id": c.ID, "ssh_host": "override"}
	if err = bind(ctx, mixed, false); !errors.Is(err, ErrInvalidRoute) {
		t.Fatalf("mixed binding: %v", err)
	}
	if err = bind(ctx, config, true); err != nil {
		t.Fatal(err)
	}
	var count int
	if err = s.store.db.Get(&count, `SELECT COUNT(*) FROM ssh_connection_refs`); err != nil || count != 0 {
		t.Fatalf("rollback leaked reference: %d %v", count, err)
	}
	if err = bind(ctx, config, false); err != nil {
		t.Fatal(err)
	}
	if err = s.store.DeleteConnection(ctx, c.ID, c.Version); !errors.Is(err, ErrInUse) {
		t.Fatalf("bound deletion: %v", err)
	}
	if err = bind(ctx, nil, false); !errors.Is(err, ErrInvalidRoute) {
		t.Fatalf("SSH executor accepted unbind: %v", err)
	}
	if err = s.store.DeleteConnection(ctx, c.ID, c.Version); !errors.Is(err, ErrInUse) {
		t.Fatalf("rejected update removed binding: %v", err)
	}
}
