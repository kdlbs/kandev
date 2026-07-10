package lifecycle

import (
	"context"
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

func TestResolveAgentProfileEnvVars_SecretAndValue(t *testing.T) {
	store := newInMemorySecretStore()
	_ = store.Create(context.Background(), &secrets.SecretWithValue{
		Secret: secrets.Secret{ID: "sec-1", Name: "test"},
		Value:  "secret-value",
	})

	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	m := &Manager{logger: log, secretStore: store}
	resolved := m.resolveAgentProfileEnvVars(context.Background(), []settingsmodels.ProfileEnvVar{
		{Key: "PLAIN", Value: "plain"},
		{Key: "FROM_SECRET", SecretID: "sec-1"},
	})
	if resolved["PLAIN"] != "plain" {
		t.Fatalf("PLAIN: got %q", resolved["PLAIN"])
	}
	if resolved["FROM_SECRET"] != "secret-value" {
		t.Fatalf("FROM_SECRET: got %q", resolved["FROM_SECRET"])
	}
}
