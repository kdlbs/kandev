package lifecycle

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/agent/agents"
	settingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	"github.com/kandev/kandev/internal/agentruntime"
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
		&AgentProfileInfo{AgentName: "codex-acp"}, agents.NewCodexACP(), agentruntime.RuntimeStandalone)
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
	}, agents.NewCodexACP(), agentruntime.RuntimeStandalone)
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
		agents.NewClaudeACP(), agentruntime.RuntimeStandalone); !errors.Is(err, ErrProviderMisconfigured) {
		t.Errorf("unsupported agent: err = %v", err)
	}

	// Bad base URL.
	bad := *base
	bad.ProviderBaseURL = "not-a-url"
	if _, _, _, err := m.resolveProviderGatewayAuth(context.Background(), &bad, agents.NewCodexACP(), agentruntime.RuntimeStandalone); !errors.Is(err, ErrProviderMisconfigured) {
		t.Errorf("bad url: err = %v", err)
	}

	// Missing secret.
	missing := *base
	missing.ProviderAPIKeySecretID = "does-not-exist"
	if _, _, _, err := m.resolveProviderGatewayAuth(context.Background(), &missing, agents.NewCodexACP(), agentruntime.RuntimeStandalone); !errors.Is(err, ErrProviderMisconfigured) {
		t.Errorf("missing secret: err = %v", err)
	}

	// Loopback base URL on a remote containerized runtime is rejected with guidance.
	remote := *base
	if _, _, _, err := m.resolveProviderGatewayAuth(context.Background(), &remote, agents.NewCodexACP(), agentruntime.RuntimeRemoteDocker); !errors.Is(err, ErrProviderMisconfigured) {
		t.Errorf("remote loopback: err = %v", err)
	}

	// Cleartext http to a non-loopback host while carrying a credential is rejected.
	insecure := *base
	insecure.ProviderBaseURL = "http://router.example/v1"
	insecure.ProviderAPIKeySecretID = "sec-key"
	if _, _, _, err := m.resolveProviderGatewayAuth(context.Background(), &insecure, agents.NewCodexACP(), agentruntime.RuntimeStandalone); !errors.Is(err, ErrProviderMisconfigured) {
		t.Errorf("credentialed cleartext: err = %v", err)
	}
}

func TestResolveProviderGatewayAuth_LocalDockerRewritesLoopback(t *testing.T) {
	m := providerGatewayManager(t, newInMemorySecretStore())
	auth, _, _, err := m.resolveProviderGatewayAuth(context.Background(), &AgentProfileInfo{
		AgentName:       "codex-acp",
		ProviderKind:    settingsmodels.ProviderKindOpenAICompatible,
		ProviderBaseURL: "http://127.0.0.1:20128/v1",
	}, agents.NewCodexACP(), agentruntime.RuntimeDocker)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	gw := auth.Meta["gateway"].(map[string]any)
	if gw["baseUrl"] != "http://host.docker.internal:20128/v1" {
		t.Errorf("baseUrl = %v", gw["baseUrl"])
	}
}
