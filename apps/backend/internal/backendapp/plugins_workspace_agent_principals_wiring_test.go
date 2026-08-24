package backendapp

import (
	"os"
	"strings"
	"testing"
)

func TestRegisterRoutesWiresWorkspaceAgentPrincipalHostSource(t *testing.T) {
	source, err := os.ReadFile("helpers.go")
	if err != nil {
		t.Fatalf("read helpers.go: %v", err)
	}
	if !strings.Contains(string(source), "p.services.Plugins.SetWorkspaceAgentPrincipalSource(pluginsWorkspaceAgentPrincipalSourceAdapter{repo: p.taskRepo})") {
		t.Fatal("registerRoutes must wire durable workspace-agent principal projection into the plugin Host API")
	}
}
