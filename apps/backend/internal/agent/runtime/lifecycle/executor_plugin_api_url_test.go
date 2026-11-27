package lifecycle

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

type pluginLaunchProfileLoaderFake struct {
	pluginExecutorRecoveryProfileLoaderFake
}

func (f *pluginLaunchProfileLoaderFake) ExecutorProviderProfileForLaunch(context.Context, string, string) (*models.ExecutorProviderLaunchProfile, error) {
	return &f.profile, nil
}

func TestPreparePluginExecutorLaunchSetsTheRuntimeAPIURL(t *testing.T) {
	profile := models.ExecutorProviderLaunchProfile{Provider: testPluginExecutorLaunchProvider(), ProfileID: "profile-1"}
	manager := &Manager{
		logger:                      newTestLogger(),
		pluginExecutorProfileLoader: &pluginLaunchProfileLoaderFake{pluginExecutorRecoveryProfileLoaderFake{profile: profile}},
		pluginRuntimeAPIURL:         "https://kandev.example/api/v1",
	}
	shared := map[string]string{envKeyKandevAPIURL: "http://localhost:38429/api/v1", "OTHER": "kept"}
	req := &ExecutorCreateRequest{Env: shared}
	metadata := map[string]interface{}{MetadataKeyExecutorProfileID: "profile-1"}

	if err := manager.preparePluginExecutorLaunch(context.Background(), req, metadata, "environment-1"); err != nil {
		t.Fatalf("preparePluginExecutorLaunch: %v", err)
	}
	if req.PluginExecutor == nil || req.PluginExecutor.Profile.ProfileID != "profile-1" {
		t.Fatalf("plugin executor = %#v", req.PluginExecutor)
	}
	if got := req.Env[envKeyKandevAPIURL]; got != "https://kandev.example/api/v1" {
		t.Fatalf("%s = %q, want the public runtime API URL", envKeyKandevAPIURL, got)
	}
	if req.Env["OTHER"] != "kept" || shared[envKeyKandevAPIURL] != "http://localhost:38429/api/v1" {
		t.Fatalf("env = %v, shared = %v; the launch env must be a copy", req.Env, shared)
	}

	manager.pluginRuntimeAPIURL = ""
	unset := &ExecutorCreateRequest{Env: map[string]string{}}
	if err := manager.preparePluginExecutorLaunch(context.Background(), unset, metadata, "environment-1"); err != nil {
		t.Fatalf("preparePluginExecutorLaunch without a URL: %v", err)
	}
	if _, set := unset.Env[envKeyKandevAPIURL]; set {
		t.Fatalf("env = %v, want no API URL when none is configured", unset.Env)
	}
}
