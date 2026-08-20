package lifecycle

import (
	"context"
	"errors"
	"testing"

	settingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/secrets"
)

func TestMergeEnvFillMissing(t *testing.T) {
	dst := map[string]string{"KANDEV_TASK_ID": "t1", "FOO": "existing"}
	mergeEnvFillMissing(dst, map[string]string{"FOO": "new", "BAR": "added"})
	if dst["FOO"] != "existing" {
		t.Fatalf("expected FOO to remain existing, got %q", dst["FOO"])
	}
	if dst["BAR"] != "added" {
		t.Fatalf("expected BAR=added, got %q", dst["BAR"])
	}
}

func TestMergeEnvFillMissingComposesIndexedGitConfig(t *testing.T) {
	dst := map[string]string{
		"GIT_CONFIG_COUNT":   "1",
		"GIT_CONFIG_KEY_0":   "credential.https://github.com.helper",
		"GIT_CONFIG_VALUE_0": "!agentctl git-credential",
	}
	src := map[string]string{
		"GIT_CONFIG_COUNT":   "1",
		"GIT_CONFIG_KEY_0":   "core.hooksPath",
		"GIT_CONFIG_VALUE_0": "/opt/locstat/hooks",
	}

	mergeEnvFillMissing(dst, src)

	if got := dst["GIT_CONFIG_COUNT"]; got != "2" {
		t.Fatalf("GIT_CONFIG_COUNT = %q, want 2", got)
	}
	if got := dst["GIT_CONFIG_KEY_0"]; got != "core.hooksPath" {
		t.Fatalf("GIT_CONFIG_KEY_0 = %q, want inherited profile entry", got)
	}
	if got := dst["GIT_CONFIG_KEY_1"]; got != "credential.https://github.com.helper" {
		t.Fatalf("GIT_CONFIG_KEY_1 = %q, want request entry", got)
	}
}

func TestMergeEnvFillMissingComposesIndexedGitConfigMismatchedCount(t *testing.T) {
	dst := map[string]string{
		"GIT_CONFIG_COUNT":   "1",
		"GIT_CONFIG_KEY_0":   "credential.https://github.com.helper",
		"GIT_CONFIG_VALUE_0": "!agentctl git-credential",
	}
	src := map[string]string{
		"GIT_CONFIG_COUNT":   "2",
		"GIT_CONFIG_KEY_0":   "core.hooksPath",
		"GIT_CONFIG_VALUE_0": "/opt/locstat/hooks",
		"GIT_CONFIG_KEY_1":   "core.autocrlf",
		"GIT_CONFIG_VALUE_1": "input",
	}

	mergeEnvFillMissing(dst, src)

	if got := dst["GIT_CONFIG_COUNT"]; got != "3" {
		t.Fatalf("GIT_CONFIG_COUNT = %q, want 3", got)
	}
	if got := dst["GIT_CONFIG_KEY_0"]; got != "core.hooksPath" {
		t.Fatalf("GIT_CONFIG_KEY_0 = %q, want inherited profile entry", got)
	}
	if got := dst["GIT_CONFIG_KEY_2"]; got != "credential.https://github.com.helper" {
		t.Fatalf("GIT_CONFIG_KEY_2 = %q, want request entry", got)
	}
}

func TestResolveAgentProfileEnvVars_SecretAndValue(t *testing.T) {
	store := newInMemorySecretStore()
	_ = store.Create(context.Background(), &secrets.SecretWithValue{
		Secret: secrets.Secret{ID: "sec-1", Name: "test"},
		Value:  "secret-value",
	})

	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	m := &Manager{logger: log, secretStore: store}
	resolved, err := m.resolveAgentProfileEnvVars(context.Background(), []settingsmodels.ProfileEnvVar{
		{Key: "PLAIN", Value: "plain"},
		{Key: "FROM_SECRET", SecretID: "sec-1"},
	})
	if err != nil {
		t.Fatalf("resolveAgentProfileEnvVars: %v", err)
	}
	if resolved["PLAIN"] != "plain" {
		t.Fatalf("PLAIN: got %q", resolved["PLAIN"])
	}
	if resolved["FROM_SECRET"] != "secret-value" {
		t.Fatalf("FROM_SECRET: got %q", resolved["FROM_SECRET"])
	}
}

func TestResolveAgentProfileEnvVars_RejectsWorkspaceSecret(t *testing.T) {
	store := newInMemorySecretStore()
	if err := store.Create(context.Background(), &secrets.SecretWithValue{
		Secret: secrets.Secret{ID: "workspace-secret", Name: "workspace", Scope: secrets.ScopeWorkspace, WorkspaceID: "workspace-1"},
		Value:  "must-not-resolve",
	}); err != nil {
		t.Fatalf("seed workspace secret: %v", err)
	}

	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	m := &Manager{logger: log, secretStore: store}
	resolved, err := m.resolveAgentProfileEnvVars(context.Background(), []settingsmodels.ProfileEnvVar{{
		Key: "WORKSPACE_ONLY", SecretID: "workspace-secret",
	}})
	if err == nil {
		t.Fatal("workspace secret resolved without an error")
	}
	if len(resolved) != 0 {
		t.Fatalf("workspace secret was resolved into profile environment: %#v", resolved)
	}
}

func TestResolveAgentProfileEnvVarsPreservesCancellation(t *testing.T) {
	for name, cause := range map[string]error{
		"canceled":          context.Canceled,
		"deadline exceeded": context.DeadlineExceeded,
	} {
		t.Run(name, func(t *testing.T) {
			store := newInMemorySecretStore()
			store.revealErr = cause
			_ = store.Create(context.Background(), &secrets.SecretWithValue{
				Secret: secrets.Secret{ID: "sec-cancel", Name: "cancel"},
				Value:  "secret-value",
			})

			log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
			m := &Manager{logger: log, secretStore: store}
			resolved, err := m.resolveAgentProfileEnvVars(context.Background(), []settingsmodels.ProfileEnvVar{
				{Key: "FROM_SECRET", SecretID: "sec-cancel"},
			})
			if !errors.Is(err, cause) {
				t.Fatalf("error = %v, want %v", err, cause)
			}
			if errors.Is(err, ErrProfileSecretUnavailable) {
				t.Fatalf("cancellation was sanitized as a profile secret error: %v", err)
			}
			if resolved != nil {
				t.Fatalf("resolved = %#v, want nil", resolved)
			}
		})
	}
}
