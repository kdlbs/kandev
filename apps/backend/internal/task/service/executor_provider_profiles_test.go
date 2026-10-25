package service

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

func testKandevKeyProvider() models.ExecutorProvider {
	return models.ExecutorProvider{ProfileSchema: map[string]any{
		"type":       "object",
		"required":   []any{"region"},
		"properties": map[string]any{"region": map[string]any{"type": "string"}},
	}}
}

func TestPluginExecutorProfilesAcceptKandevCredentialKeys(t *testing.T) {
	provider := testKandevKeyProvider()
	config := map[string]string{
		"region":                  "us-west-2",
		"remote_credentials":      `["agent:opencode-acp:files:0"]`,
		"remote_auth_secrets":     `{"agent:claude-acp:env:ANTHROPIC_API_KEY":"secret-1"}`,
		"remote_auth_target_home": "/root",
	}
	if err := validatePluginExecutorProfileSchema(provider, config); err != nil {
		t.Fatalf("validate: %v", err)
	}
	for key, value := range map[string]string{
		"remote_credentials": "not json", "agent_config_bundles": "123", "remote_auth_secrets": `["a"]`,
	} {
		invalid := map[string]string{"region": "us-west-2", key: value}
		if err := validatePluginExecutorProfileSchema(provider, invalid); !errors.Is(err, ErrInvalidExecutorConfig) {
			t.Fatalf("%s=%s: %v, want ErrInvalidExecutorConfig", key, value, err)
		}
	}
}

func TestPluginExecutorProfileKandevKeysAreClearedAndNeverSentToTheProvider(t *testing.T) {
	provider := testKandevKeyProvider()
	current := map[string]string{"region": "us-west-2", "remote_credentials": `["a"]`, "git_user_name": "Ada"}
	config, _, err := (&Service{}).normalizePluginExecutorProfileConfig(context.Background(), provider, "profile-1", current,
		map[string]string{"remote_credentials": "", "agent_config_bundles": `["bundle"]`})
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	if _, kept := config["remote_credentials"]; kept || config["agent_config_bundles"] != `["bundle"]` || config["git_user_name"] != "Ada" {
		t.Fatalf("normalized config = %v", config)
	}
	public := publicExecutorProviderConfig(provider, config)
	if len(public) != 1 || public["region"] != "us-west-2" {
		t.Fatalf("provider config = %v, want only provider fields", public)
	}
}

func TestProviderSchemasCannotDeclareKandevKeys(t *testing.T) {
	schema := map[string]any{"properties": map[string]any{"remote_credentials": map[string]any{"type": "string"}}}
	if _, _, err := executorProviderSchemaFields(schema); !errors.Is(err, ErrInvalidExecutorConfig) {
		t.Fatalf("reserved field = %v, want ErrInvalidExecutorConfig", err)
	}
}
