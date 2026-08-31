package lifecycle

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/agent/agents"
	settingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/secrets"
)

func providerGatewayManager(t *testing.T, store secrets.SecretStore) *Manager {
	t.Helper()
	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	return &Manager{logger: log, secretStore: store}
}

func TestResolveProviderGatewayAuth_NativeProfileReturnsNil(t *testing.T) {
	m := providerGatewayManager(t, newInMemorySecretStore())
	auth, _, _, err := m.resolveProviderGatewayAuth(context.Background(),
		&AgentProfileInfo{AgentName: "codex-acp"}, agents.NewCodexACP())
	if err != nil || auth != nil {
		t.Fatalf("native profile: auth=%v err=%v", auth, err)
	}
}

func TestResolveProviderGatewayAuth_Success(t *testing.T) {
	store := newInMemorySecretStore()
	_ = store.Create(context.Background(), &secrets.SecretWithValue{
		Secret: secrets.Secret{ID: "sec-key", Name: "router"},
		Value:  "sk-router",
	})
	m := providerGatewayManager(t, store)

	auth, keyEnvVar, key, err := m.resolveProviderGatewayAuth(context.Background(), &AgentProfileInfo{
		AgentName:              "codex-acp",
		ProviderKind:           settingsmodels.ProviderKindOpenAICompatible,
		ProviderBaseURL:        "http://localhost:20128/v1",
		ProviderAPIKeySecretID: "sec-key",
	}, agents.NewCodexACP())
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if auth == nil || auth.MethodID != "gateway" {
		t.Fatalf("auth = %#v", auth)
	}
	if keyEnvVar != "OPENAI_API_KEY" || key != "sk-router" {
		t.Fatalf("keyEnvVar=%q key=%q", keyEnvVar, key)
	}
	gw := auth.Meta["gateway"].(map[string]any)
	if gw["baseUrl"] != "http://localhost:20128/v1" {
		t.Errorf("baseUrl = %v", gw["baseUrl"])
	}
	if gw["headers"].(map[string]any)["Authorization"] != "Bearer sk-router" {
		t.Errorf("headers = %v", gw["headers"])
	}
}

func TestResolveProviderGatewayAuth_FailsClosed(t *testing.T) {
	store := newInMemorySecretStore()
	m := providerGatewayManager(t, store)
	base := &AgentProfileInfo{
		AgentName:       "codex-acp",
		ProviderKind:    settingsmodels.ProviderKindOpenAICompatible,
		ProviderBaseURL: "http://localhost:20128/v1",
	}

	// Agent without provider support.
	if _, _, _, err := m.resolveProviderGatewayAuth(context.Background(),
		&AgentProfileInfo{AgentName: "claude-acp", ProviderKind: settingsmodels.ProviderKindOpenAICompatible, ProviderBaseURL: "http://h/v1"},
		agents.NewClaudeACP()); !errors.Is(err, ErrProviderMisconfigured) {
		t.Errorf("unsupported agent: err = %v", err)
	}

	// Bad base URL.
	bad := *base
	bad.ProviderBaseURL = "not-a-url"
	if _, _, _, err := m.resolveProviderGatewayAuth(context.Background(), &bad, agents.NewCodexACP()); !errors.Is(err, ErrProviderMisconfigured) {
		t.Errorf("bad url: err = %v", err)
	}

	// Missing secret.
	missing := *base
	missing.ProviderAPIKeySecretID = "does-not-exist"
	if _, _, _, err := m.resolveProviderGatewayAuth(context.Background(), &missing, agents.NewCodexACP()); !errors.Is(err, ErrProviderMisconfigured) {
		t.Errorf("missing secret: err = %v", err)
	}
}
