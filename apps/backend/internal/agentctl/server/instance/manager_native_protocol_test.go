package instance

import (
	"context"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/pkg/agent"
)

func TestCreateInstanceRejectsCodexAppServerWhenDisabled(t *testing.T) {
	manager := &Manager{config: &config.Config{Defaults: config.InstanceDefaults{Protocol: agent.ProtocolACP}}, logger: logger.Default()}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := manager.CreateInstance(ctx, &CreateRequest{
		WorkspacePath: t.TempDir(),
		Protocol:      string(agent.ProtocolCodexAppServer),
	})
	if err == nil || !strings.Contains(err.Error(), "codex app-server feature is disabled") {
		t.Fatalf("CreateInstance error = %v", err)
	}
}

func TestCreateInstanceAcceptsCodexAppServerWithBackendGate(t *testing.T) {
	manager := &Manager{config: &config.Config{Defaults: config.InstanceDefaults{Protocol: agent.ProtocolACP}}, logger: logger.Default()}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := manager.CreateInstance(ctx, &CreateRequest{
		WorkspacePath:         t.TempDir(),
		Protocol:              string(agent.ProtocolCodexAppServer),
		CodexAppServerEnabled: true,
	})
	if err == nil || strings.Contains(err.Error(), "feature is disabled") {
		t.Fatalf("CreateInstance should proceed past the feature gate, got %v", err)
	}
}
