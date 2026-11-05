package main

import (
	"os"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/acpcompat"
)

func TestAntigravityBrowserHelper(t *testing.T) {
	value := acpcompat.BrowserEnvValue(`/opt/Kandev Agent/agentctl`)
	if !strings.Contains(value, acpcompat.AntigravityBrowserHelperCommand) || !strings.Contains(value, "%s") {
		t.Fatalf("BROWSER value = %q", value)
	}
	previous := os.Args
	os.Args = []string{"/opt/kandev/agentctl", acpcompat.AntigravityBrowserHelperCommand, "https://example.invalid"}
	defer func() { os.Args = previous }()
	if code := runMain(); code != 0 {
		t.Fatalf("helper exit code = %d, want 0", code)
	}
}
