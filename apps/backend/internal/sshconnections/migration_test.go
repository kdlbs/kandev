package sshconnections

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/kandev/kandev/internal/db"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

// @covers AC-EXECUTORS-SSH-MIGRATION-001.1, AC-EXECUTORS-SSH-MIGRATION-001.2
func TestMigrationConvergesOffline(t *testing.T) {
	path := filepath.Join(t.TempDir(), "migration.db")
	open := func() *Store {
		t.Helper()
		conn, err := db.OpenSQLite(path)
		if err != nil {
			t.Fatal(err)
		}
		pool := sqlx.NewDb(conn, "sqlite3")
		t.Cleanup(func() { _ = pool.Close() })
		store, err := NewStore(pool)
		if err != nil {
			t.Fatal(err)
		}
		return store
	}
	first := open()
	testMigrationConvergesOffline(t, first, open)
}

func TestPostgresMigrationConvergesOffline(t *testing.T) {
	s, _ := postgresIdentityService(t)
	testMigrationConvergesOffline(t, s.store, func() *Store {
		second, err := NewStore(s.store.db)
		if err != nil {
			t.Fatal(err)
		}
		return second
	})
}

func testMigrationConvergesOffline(t *testing.T, first *Store, open func() *Store) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("SSH_AUTH_SOCK", "")
	for _, q := range []string{
		`CREATE TABLE executors(id TEXT PRIMARY KEY,type TEXT,config TEXT,updated_at TIMESTAMP,deleted_at TIMESTAMP)`,
		`CREATE TABLE executor_profiles(id TEXT PRIMARY KEY,executor_id TEXT)`,
		`INSERT INTO executor_profiles VALUES ('profile-1','executor-1'), ('profile-2','executor-1')`,
		`INSERT INTO executors(id,type,config) VALUES
		  ('executor-1','ssh','{"ssh_host":"offline.invalid","ssh_user":"deploy","ssh_port":"2222","ssh_identity_source":"agent","ssh_host_fingerprint":"SHA256:legacy","ssh_shell":"zsh"}'),
		  ('executor-2','ssh','{"ssh_host":"offline.invalid","ssh_user":"deploy","ssh_identity_source":"file","ssh_identity_file":"/unavailable/key"}'),
		  ('managed','ssh','{"ssh_connection_id":"original"}'),
		  ('local','local','{"ssh_host":"keep"}')`,
	} {
		if _, err := first.db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}

	// Failure after staging graph changes must roll the import back as a unit.
	if _, err := first.db.Exec(`ALTER TABLE ssh_connection_audit RENAME TO migration_audit_fixture`); err != nil {
		t.Fatal(err)
	}
	if err := first.MigrateLegacyExecutors(t.Context()); err == nil {
		t.Fatal("upgrade succeeded without its audit table")
	}
	for _, table := range []string{"ssh_connections", "ssh_identities", "ssh_connection_refs"} {
		var count int
		if err := first.db.Get(&count, "SELECT COUNT(*) FROM "+table); err != nil || count != 0 {
			t.Fatalf("partial upgrade in %s: count=%d err=%v", table, count, err)
		}
	}
	if _, err := first.db.Exec(`ALTER TABLE migration_audit_fixture RENAME TO ssh_connection_audit`); err != nil {
		t.Fatal(err)
	}
	second := open()
	start := make(chan struct{})
	results := make(chan error, 2)
	var done sync.WaitGroup
	for _, store := range []*Store{first, second} {
		done.Add(1)
		go func() {
			defer done.Done()
			<-start
			results <- store.MigrateLegacyExecutors(t.Context())
		}()
	}
	close(start)
	done.Wait()
	close(results)
	for err := range results {
		if err != nil {
			t.Fatal(err)
		}
	}

	config1 := migrationTestConfig(t, first, "executor-1")
	config2 := migrationTestConfig(t, first, "executor-2")
	if config1[ConnectionConfigKey] == "" || config2[ConnectionConfigKey] == "" || config1[ConnectionConfigKey] == config2[ConnectionConfigKey] {
		t.Fatal("connection was missing or shared between executors")
	}
	if config1["ssh_shell"] != "zsh" || config1["ssh_host"] != "" || migrationTestConfig(t, first, "managed")[ConnectionConfigKey] != "original" || migrationTestConfig(t, first, "local")["ssh_host"] != "keep" {
		t.Fatal("upgrade dropped unrelated configuration or changed existing bindings")
	}
	c, err := first.GetConnection(context.Background(), config1[ConnectionConfigKey])
	if err != nil || c.Fingerprint != "SHA256:legacy" || c.VerifiedRoute != "" || c.Port != 2222 {
		t.Fatalf("destination trust or route changed: %+v, %v", c, err)
	}
	if err = first.write(context.Background(), func(tx *sqlx.Tx) error {
		i, err := identityIn(context.Background(), tx, c.IdentityID)
		if err != nil || i.Kind != identityAgent || i.Reference != "" {
			t.Fatalf("default agent selector changed: %+v, %v", i, err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SSH_AUTH_SOCK", filepath.Join(os.TempDir(), "new-backend-agent.sock"))
	if err = open().MigrateLegacyExecutors(t.Context()); err != nil {
		t.Fatal(err)
	}
	if migrationTestConfig(t, first, "executor-1")[ConnectionConfigKey] != c.ID {
		t.Fatal("reopen changed the binding ID")
	}
	for _, table := range []string{"ssh_connections", "ssh_identities", "ssh_connection_refs", "ssh_connection_audit", "executor_profiles"} {
		var count int
		if err = first.db.Get(&count, "SELECT COUNT(*) FROM "+table); err != nil || count != 2 {
			t.Fatalf("%s count=%d err=%v", table, count, err)
		}
	}
}

func migrationTestConfig(t *testing.T, store *Store, id string) map[string]string {
	t.Helper()
	var saved string
	if err := store.db.Get(&saved, store.db.Rebind(`SELECT config FROM executors WHERE id=?`), id); err != nil {
		t.Fatal(err)
	}
	var config map[string]string
	if err := json.Unmarshal([]byte(saved), &config); err != nil {
		t.Fatal(err)
	}
	return config
}

// @covers AC-EXECUTORS-SSH-MIGRATION-001.4
func TestMigrationRequiresRepairAndRetriesMissingAlias(t *testing.T) {
	s, ctx := identityService(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("SSH_AUTH_SOCK", "")
	migrationCreateExecutors(t, s.store, map[string]string{
		"broken":    "invalid stored JSON",
		"malformed": `{"ssh_host":"offline.invalid","ssh_user":"deploy","invalid_non_transport":{"value":1},"ssh_shell":"zsh"}`,
		"alias":     `{"ssh_host_alias":"unconfigured-migration-fixture","ssh_identity_source":"agent","ssh_host_fingerprint":"SHA256:saved"}`,
		"jump":      `{"ssh_host":"offline.invalid","ssh_user":"deploy","ssh_proxy_jump":"first,second","ssh_identity_source":"agent"}`,
		"mixed":     `{"ssh_connection_id":"foreign","ssh_host":"offline.invalid","ssh_identity_file":false,"ssh_host_fingerprint":"SHA256:original"}`,
	})
	if err := s.store.MigrateLegacyExecutors(ctx); err != nil {
		t.Fatal(err)
	}
	cases := map[string]string{"alias": migrationAlias, "jump": migrationRoute, "mixed": migrationMixed, "broken": migrationInvalid, "malformed": migrationInvalid}
	for executor, reason := range cases {
		cfg := migrationTestConfig(t, s.store, executor)
		c, err := s.store.GetConnection(ctx, cfg[ConnectionConfigKey])
		if err != nil || c.RepairReason != reason || !c.ImportedPending || c.VerifiedRoute != "" || hasInlineSSHConfig(cfg) {
			t.Fatalf("%s migration: %+v, %v", executor, c, err)
		}
		if err = s.store.write(ctx, func(tx *sqlx.Tx) error {
			return s.BindExecutorIn(ctx, tx, "other-executor", executorKindSSH, cfg)
		}); !errors.Is(err, ErrInvalidRoute) {
			t.Fatalf("ordinary binding accepted imported repair route: %v", err)
		}
	}
	if config := migrationTestConfig(t, s.store, "malformed"); config["ssh_shell"] != "zsh" {
		t.Fatal("repair lost valid settings along with a malformed field")
	}
	before := migrationTestConfig(t, s.store, "alias")
	dir := filepath.Join(home, ".ssh")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	explicitSocket := filepath.Join(home, "restored-agent.sock")
	aliasConfig := "Host unconfigured-migration-fixture\n HostName restored.invalid\n User restored\n Port 2200\n IdentityAgent " + explicitSocket + "\n"
	if err := os.WriteFile(filepath.Join(dir, "config"), []byte(aliasConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := s.refreshSSHMigration(ctx, "alias"); err != nil {
		t.Fatal(err)
	}
	after := migrationTestConfig(t, s.store, "alias")
	c, err := s.store.GetConnection(ctx, after[ConnectionConfigKey])
	if err != nil || before[ConnectionConfigKey] != c.ID || before["ssh_connection_stamp"] == after["ssh_connection_stamp"] ||
		c.Host != "restored.invalid" || c.Port != 2200 || c.Fingerprint != "SHA256:saved" ||
		c.RepairReason != codeAgentUnavailable || !c.ImportedPending || c.VerifiedRoute != "" {
		t.Fatalf("recovered alias did not update the existing binding: %+v, %v", c, err)
	}
	if err = s.store.write(ctx, func(tx *sqlx.Tx) error {
		identity, err := identityIn(ctx, tx, c.IdentityID)
		if err != nil || identity.Kind != identityAgent || identity.Reference != explicitSocket {
			t.Fatalf("restored explicit agent selector: %+v, %v", identity, err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestMigrationMetadataIsServerOwned(t *testing.T) {
	s, ctx := identityService(t)
	route, err := s.SaveConnection(ctx, SaveConnectionRequest{RequestID: uuid.NewString(), Connection: Connection{
		Name: "forged import", Host: "offline.invalid", Port: 22, User: "deploy", ImportedPending: true, KnownHostsPending: true, RepairReason: migrationMixed,
	}})
	if err != nil {
		t.Fatal(err)
	}
	if route.Destination.ImportedPending || route.Destination.KnownHostsPending || route.Destination.RepairReason != "" {
		t.Fatalf("ordinary connection write accepted migration metadata: %+v", route.Destination)
	}
}

func migrationCreateExecutors(t *testing.T, store *Store, rows map[string]string) {
	t.Helper()
	if _, err := store.db.Exec(`CREATE TABLE executors(id TEXT PRIMARY KEY,type TEXT,config TEXT,updated_at TIMESTAMP,deleted_at TIMESTAMP)`); err != nil {
		t.Fatal(err)
	}
	query := store.db.Rebind(`INSERT INTO executors(id,type,config) VALUES (?,'ssh',?)`)
	for id, config := range rows {
		if _, err := store.db.Exec(query, id, config); err != nil {
			t.Fatal(err)
		}
	}
}
