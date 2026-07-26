package gitlab

import (
	"context"
	"testing"
	"time"
)

func TestPollerStartRecordsHealthForEveryConfiguredWorkspace(t *testing.T) {
	t.Setenv("KANDEV_MOCK_GITLAB", "true")
	t.Setenv(secretNameToken, "test-token")
	store := newTestStore(t)
	for _, workspaceID := range []string{"workspace-a", "workspace-b"} {
		seedWorkspace(t, store, workspaceID)
		if err := store.UpsertConfigForWorkspace(context.Background(), workspaceID, &GitLabConfig{
			Host: DefaultHost, AuthMethod: AuthMethodEnvironment,
		}); err != nil {
			t.Fatalf("seed config %s: %v", workspaceID, err)
		}
	}
	svc := newWorkspaceConfigService(t, store, &configTestSecrets{values: make(map[string]string)})
	poller := NewPoller(svc, nil, newTestLogger(t))
	ctx, cancel := context.WithCancel(context.Background())
	poller.Start(ctx)
	t.Cleanup(func() { cancel(); poller.Stop() })
	deadline := time.Now().Add(time.Second)
	for {
		cfg, err := store.GetConfigForWorkspace(context.Background(), "workspace-a")
		if err != nil {
			t.Fatalf("get config workspace-a: %v", err)
		}
		if cfg.LastCheckedAt != nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for immediate health probe")
		}
		time.Sleep(time.Millisecond)
	}

	for _, workspaceID := range []string{"workspace-a", "workspace-b"} {
		cfg, err := store.GetConfigForWorkspace(context.Background(), workspaceID)
		if err != nil {
			t.Fatalf("get config %s: %v", workspaceID, err)
		}
		if cfg.LastCheckedAt == nil || !cfg.LastOK || cfg.Username != "kandev-tester" {
			t.Fatalf("health for %s = %#v", workspaceID, cfg)
		}
	}
}
