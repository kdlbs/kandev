package sshconnections

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/jmoiron/sqlx"
	"testing"
)

func TestExecutorProbeRequiresEnabledSavedBinding(t *testing.T) {
	if _, _, _, err := NewService(false, nil, nil).OpenExecutor(context.Background(), "executor"); !errors.Is(err, ErrDisabled) {
		t.Fatalf("disabled probe: %v", err)
	}
	s, ctx := identityService(t)
	if _, err := s.store.db.Exec(`CREATE TABLE executors(id TEXT PRIMARY KEY,type TEXT,config TEXT,deleted_at TIMESTAMP)`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.store.db.Exec(`INSERT INTO executors(id,type,config) VALUES ('legacy','ssh','{"ssh_host":"arbitrary"}')`); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := s.OpenExecutor(ctx, "legacy"); !errors.Is(err, ErrInvalidRoute) {
		t.Fatalf("inline config accepted: %v", err)
	}
	if _, _, _, err := s.OpenExecutor(ctx, "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown executor: %v", err)
	}
}

// @covers AC-EXECUTORS-SSH-CONNECTIONS-002.8, AC-EXECUTORS-SSH-CONNECTIONS-005.3
func TestExecutorConnectionHoldsRouteUntilCleanup(t *testing.T) {
	t.Setenv("SSH_AUTH_SOCK", "")
	s, ctx := identityService(t)
	identity := newWireIdentity(t, s, ctx, "destination")
	host := newWireHost(t, identity.PublicKey)
	route := trustWireRoute(t, s, ctx, host, identity, "")
	route, err := s.TestConnection(ctx, route.Destination.ID, route.Stamp)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.store.db.Exec(`CREATE TABLE executors(id TEXT PRIMARY KEY,type TEXT,config TEXT,deleted_at TIMESTAMP)`); err != nil {
		t.Fatal(err)
	}
	config := map[string]string{ConnectionConfigKey: route.Destination.ID, "ssh_connection_stamp": route.Stamp}
	data, err := json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.store.db.Exec(`INSERT INTO executors(id,type,config) VALUES ('executor','ssh',?)`, string(data)); err != nil {
		t.Fatal(err)
	}
	if err = s.store.write(ctx, func(tx *sqlx.Tx) error { return s.BindExecutorIn(ctx, tx, "executor", "ssh", config) }); err != nil {
		t.Fatal(err)
	}
	client, target, cleanup, err := s.OpenExecutor(ctx, "executor")
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	if client == nil || target.PinnedFingerprint != host.fingerprint {
		t.Fatal("missing authenticated pinned transport")
	}
	changed := route.Destination
	changed.User = "changed"
	if _, err = s.store.SaveConnection(ctx, changed, changed.Version); !errors.Is(err, ErrInUse) {
		t.Fatalf("active cleanup route changed: %v", err)
	}
	cleanup()
	cleanup()
	if _, err = s.store.SaveConnection(ctx, changed, changed.Version); err != nil {
		t.Fatal(err)
	}
	before := host.keyCalls.Load()
	if _, _, _, err = s.OpenExecutor(ctx, "executor"); !errors.Is(err, ErrInvalidRoute) {
		t.Fatalf("stale route accepted: %v", err)
	}
	if host.keyCalls.Load() != before {
		t.Fatal("stale binding attempted authentication")
	}
	var refs int
	if err = s.store.db.Get(&refs, `SELECT COUNT(*) FROM ssh_connection_refs WHERE owner_type='probe'`); err != nil || refs != 0 {
		t.Fatalf("cleanup left probe refs: %d %v", refs, err)
	}
}
