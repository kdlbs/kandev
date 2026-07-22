package gitlab

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type failingHostStore struct{}

func (failingHostStore) GetHost(context.Context) (string, error) {
	return "", errors.New("settings unavailable")
}

type staticHostStore struct{ host string }

func (s *staticHostStore) GetHost(context.Context) (string, error) { return s.host, nil }
func (s *staticHostStore) SetHost(_ context.Context, host string) error {
	s.host = host
	return nil
}

func (failingHostStore) SetHost(context.Context, string) error {
	return nil
}

func TestMigrateLegacyConnectionKeepsPATWhenWorkspaceSecretStoreIsUnavailable(t *testing.T) {
	store := newTestStore(t)
	seedWorkspace(t, store, "workspace-a")
	legacy := newFakeSecretManager()
	legacy.items = []*SecretListItem{{ID: "legacy-token", Name: secretNameToken, HasValue: true}}
	legacy.values["legacy-token"] = "glpat-legacy"
	hosts := &staticHostStore{host: "http://gitlab.internal"}

	err := MigrateLegacyConnection(
		context.Background(), store, nil, legacy, legacy, hosts, newTestLogger(t),
	)
	if err == nil {
		t.Fatal("expected migration to require workspace secret storage")
	}
	cfg, getErr := store.GetConfigForWorkspace(context.Background(), "workspace-a")
	if getErr != nil {
		t.Fatalf("get config: %v", getErr)
	}
	if cfg != nil {
		t.Fatalf("config was persisted without its PAT: %#v", cfg)
	}
	if legacy.deleteCalls != 0 || legacy.values["legacy-token"] != "glpat-legacy" {
		t.Fatal("legacy PAT was deleted when migration could not persist the workspace secret")
	}
}

func TestMigrateLegacyConnectionRestoresWorkspaceSecretWhenConfigWriteFails(t *testing.T) {
	store := newTestStore(t)
	seedWorkspace(t, store, "workspace-a")
	if _, err := store.db.Exec(`CREATE TRIGGER fail_gitlab_migration_config
		BEFORE INSERT ON gitlab_configs
		BEGIN SELECT RAISE(FAIL, 'injected migration config failure'); END`); err != nil {
		t.Fatalf("install migration failure trigger: %v", err)
	}
	legacy := newFakeSecretManager()
	legacy.items = []*SecretListItem{{ID: "legacy-token", Name: secretNameToken, HasValue: true}}
	legacy.values["legacy-token"] = "glpat-legacy"
	workspaceSecrets := &configTestSecrets{values: map[string]string{
		SecretKeyForWorkspace("workspace-a"): "previous-workspace-token",
	}}
	hosts := &staticHostStore{host: "http://gitlab.internal"}

	err := MigrateLegacyConnection(
		context.Background(), store, workspaceSecrets, legacy, legacy, hosts, newTestLogger(t),
	)
	if err == nil {
		t.Fatal("expected migration config failure")
	}
	if got := workspaceSecrets.values[SecretKeyForWorkspace("workspace-a")]; got != "previous-workspace-token" {
		t.Fatalf("workspace secret = %q, want previous-workspace-token", got)
	}
	if legacy.deleteCalls != 0 || legacy.values["legacy-token"] != "glpat-legacy" {
		t.Fatal("legacy PAT was deleted before the workspace config became durable")
	}
	if hosts.host != "http://gitlab.internal" {
		t.Fatalf("legacy host = %q, want unchanged", hosts.host)
	}
}

func TestProvideFailsClosedWhenHostStoreCannotBeRead(t *testing.T) {
	t.Setenv("KANDEV_MOCK_GITLAB", "true")
	t.Setenv("GITLAB_TOKEN", "token-for-self-managed-host")

	svc, cleanup, err := Provide(context.Background(), nil, failingHostStore{}, newTestLogger(t))
	if err == nil {
		t.Fatal("expected host store read error")
	}
	if !strings.Contains(err.Error(), "load GitLab host") {
		t.Fatalf("err = %v, want load GitLab host context", err)
	}
	if svc != nil {
		t.Fatalf("service = %#v, want nil", svc)
	}
	if cleanup != nil {
		t.Fatalf("cleanup = %T, want nil", cleanup)
	}
}
