package copilotplugins_test

import (
	"testing"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/copilotplugins"
)

func TestCopilotPluginRuntime105AdapterContract(t *testing.T) {
	spec := agents.NewCopilotACP().ManagedNPMRuntime()
	if spec.Package != "@github/copilot" || spec.PackageSpec("1.0.105") != "@github/copilot@1.0.105" {
		t.Fatalf("managed package contract = %q %q", spec.Package, spec.PackageSpec("1.0.105"))
	}
	if version, ok := copilotplugins.RuntimeVersionFromCommand([]string{
		"npx", "--yes", "--prefer-offline", "@github/copilot@1.0.105", "--acp",
	}); !ok || version != "1.0.105" {
		t.Fatalf("runtime version from command = %q, ok=%t", version, ok)
	}
	if !copilotplugins.RuntimeSupported("1.0.105") || !copilotplugins.MountsPermitted("1.0.105") {
		t.Fatal("1.0.105 must share discovery and mount admission")
	}
}
