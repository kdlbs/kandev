package sshconnections

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/secrets"
)

func openWireService(t *testing.T, root string) *Service {
	t.Helper()
	connection, err := db.OpenSQLite(filepath.Join(root, "connections.db"))
	if err != nil {
		t.Fatal(err)
	}
	database := sqlx.NewDb(connection, "sqlite3")
	t.Cleanup(func() { _ = database.Close() })
	store, err := NewStore(database)
	if err != nil {
		t.Fatal(err)
	}
	key, err := secrets.NewMasterKeyProvider(filepath.Join(root, "protection"))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = secrets.Provide(database, database, key); err != nil {
		t.Fatal(err)
	}
	return NewService(true, store, secrets.NewSSHIdentityVault(key))
}

func wireRuntimeSnapshot(t *testing.T, s *Service, ctx context.Context, route Route) RuntimeSnapshot {
	t.Helper()
	for _, query := range []string{`CREATE TABLE executors(id TEXT PRIMARY KEY, config TEXT, deleted_at TIMESTAMP,type TEXT DEFAULT 'ssh')`, `CREATE TABLE task_sessions(id TEXT PRIMARY KEY,executor_id TEXT,task_id TEXT NOT NULL DEFAULT 'fixture-task')`, `INSERT INTO task_sessions(id,executor_id) VALUES ('session','executor')`} {
		if _, err := s.store.db.Exec(query); err != nil {
			t.Fatal(err)
		}
	}
	config, err := json.Marshal(map[string]string{ConnectionConfigKey: route.Destination.ID, "ssh_connection_stamp": route.Stamp})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.store.db.Exec(`INSERT INTO executors(id,config) VALUES ('executor',?)`, string(config)); err != nil {
		t.Fatal(err)
	}
	snapshot, err := s.AdmitRuntime(ctx, "session", route.Destination.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	client, err := s.DialRuntime(ctx, snapshot)
	if err != nil {
		t.Fatal(err)
	}
	_ = client.Close()
	return snapshot
}

func TestManagedSSHWireRestartWithoutAgent(t *testing.T) {
	for _, kind := range []string{"direct", "bastion"} {
		t.Run(kind, func(t *testing.T) {
			root := t.TempDir()
			s := openWireService(t, root)
			ctx := authn.WithIdentity(context.Background(), authn.Identity{UserID: "admin", Role: authn.RoleAdmin})
			identity := newWireIdentity(t, s, ctx, "destination")
			target := newWireHost(t, identity.PublicKey)
			var bastionID string
			var bastion *wireHost
			if kind == "bastion" {
				gateway := newWireIdentity(t, s, ctx, "gateway")
				bastion = newWireHost(t, gateway.PublicKey)
				bastion.setDestination(target)
				route := trustWireRoute(t, s, ctx, bastion, gateway, "")
				if _, err := s.TestConnection(ctx, route.Destination.ID, route.Stamp); err != nil {
					t.Fatal(err)
				}
				bastionID = route.Destination.ID
			}
			route := trustWireRoute(t, s, ctx, target, identity, bastionID)
			route, err := s.TestConnection(ctx, route.Destination.ID, route.Stamp)
			if err != nil {
				t.Fatal(err)
			}
			snapshot := wireRuntimeSnapshot(t, s, ctx, route)
			data, err := json.Marshal(snapshot)
			if err != nil {
				t.Fatal(err)
			}
			if err = os.WriteFile(filepath.Join(root, "snapshot.json"), data, 0600); err != nil {
				t.Fatal(err)
			}
			if err = s.store.db.Close(); err != nil {
				t.Fatal(err)
			}
			before := target.keyCalls.Load()
			processCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			command := exec.CommandContext(processCtx, os.Args[0], "-test.run=^TestManagedSSHRestartProcessHelper$", "-test.v")
			for _, entry := range os.Environ() {
				if strings.HasPrefix(entry, "SSH_AUTH_SOCK=") || strings.HasPrefix(entry, "SSH_AGENT_PID=") || strings.HasPrefix(entry, "KANDEV_SSH_TEST_RESTART_ROOT=") {
					continue
				}
				command.Env = append(command.Env, entry)
			}
			command.Env = append(command.Env, "KANDEV_SSH_TEST_RESTART_ROOT="+root)
			if output, err := command.CombinedOutput(); err != nil {
				t.Fatalf("restart process failed: %v\n%s", err, output)
			}
			if target.keyCalls.Load() <= before || target.lastKey() != identity.Fingerprint {
				t.Fatal("restarted process did not authenticate with the persisted identity")
			}
			if target.executions.Load() != 0 || target.passwordCalls.Load() != 0 {
				t.Fatal("restart performed enrollment or bootstrap authentication")
			}
			if bastion != nil && (bastion.executions.Load() != 0 || bastion.passwordCalls.Load() != 0) {
				t.Fatal("restart modified the bastion")
			}
		})
	}
}

func TestManagedSSHRestartProcessHelper(t *testing.T) {
	root := os.Getenv("KANDEV_SSH_TEST_RESTART_ROOT")
	if root == "" {
		t.Skip("subprocess helper")
	}
	if os.Getenv("SSH_AUTH_SOCK") != "" {
		t.Fatal("restart fixture inherited an agent")
	}
	s := openWireService(t, root)
	data, err := os.ReadFile(filepath.Join(root, "snapshot.json"))
	if err != nil {
		t.Fatal(err)
	}
	var snapshot RuntimeSnapshot
	if err = json.Unmarshal(data, &snapshot); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err = s.RecoverEnrollments(ctx); err != nil {
		t.Fatal(err)
	}
	client, err := s.DialRuntime(ctx, snapshot)
	if err != nil {
		t.Fatal(err)
	}
	_ = client.Close()
}

func TestManagedSSHWireProtectionLossDoesNotReplaceIdentity(t *testing.T) {
	for _, failure := range []string{"missing_ciphertext", "wrong_master"} {
		t.Run(failure, func(t *testing.T) {
			root := t.TempDir()
			s := openWireService(t, root)
			ctx := authn.WithIdentity(context.Background(), authn.Identity{UserID: "admin", Role: authn.RoleAdmin})
			identity := newWireIdentity(t, s, ctx, "destination")
			target := newWireHost(t, identity.PublicKey)
			route := trustWireRoute(t, s, ctx, target, identity, "")
			route, err := s.TestConnection(ctx, route.Destination.ID, route.Stamp)
			if err != nil {
				t.Fatal(err)
			}
			snapshot := wireRuntimeSnapshot(t, s, ctx, route)
			if failure == "missing_ciphertext" {
				if _, err = s.store.db.Exec(`DELETE FROM secrets WHERE id=?`, "ssh-identity:"+identity.ID); err != nil {
					t.Fatal(err)
				}
			} else {
				// Only the disposable protection directory is replaced.
				other, err := secrets.NewMasterKeyProvider(t.TempDir())
				if err != nil {
					t.Fatal(err)
				}
				if err = os.WriteFile(filepath.Join(root, "protection", secrets.MasterKeyFile), other.Key(), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if err = s.store.db.Close(); err != nil {
				t.Fatal(err)
			}
			restored := openWireService(t, root)
			before := target.keyCalls.Load()
			if _, err = restored.DialRuntime(ctx, snapshot); !errors.Is(err, ErrCredential) {
				t.Fatalf("unavailable identity dial = %v", err)
			}
			if target.keyCalls.Load() != before {
				t.Fatal("unavailable identity attempted authentication")
			}
			preflight, err := restored.Preflight(ctx, route.Destination.ID, route.Stamp)
			if err != nil {
				t.Fatal(err)
			}
			if preflight.Ready || len(preflight.Checks) != 1 || preflight.Checks[0].Code != "ssh_managed_key_unavailable" {
				t.Fatalf("wrong protection diagnostic: %+v", preflight)
			}
			identities, err := restored.ListIdentities(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if len(identities) != 1 || identities[0].Identity != identity {
				t.Fatal("unavailable identity was replaced or changed")
			}
		})
	}
}
