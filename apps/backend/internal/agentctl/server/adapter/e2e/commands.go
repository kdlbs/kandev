//go:build e2e

package e2e

import (
	"strings"
	"testing"
	"unicode"

	"github.com/kandev/kandev/internal/agent/agents"
)

// acpCommand derives the standalone ACP launch command for an agent from its
// production BuildCommand, instead of restating it as a literal in the test.
// This keeps the e2e test in sync with production if a managed npm version
// pin or an agent's ACP args changes.
func acpCommand(t *testing.T, a agents.Agent) string {
	t.Helper()

	argv := a.BuildCommand(agents.CommandOptions{}).Args()
	if len(argv) == 0 {
		t.Fatalf("BuildCommand returned an empty argv")
	}
	for i, tok := range argv {
		if tok == "" {
			t.Fatalf("BuildCommand argv token %d is empty; it cannot round-trip through the string-shaped AgentSpec.Command", i)
		}
		if hasUnicodeWhitespace(tok) {
			t.Fatalf("BuildCommand argv token %q contains whitespace; it cannot round-trip through the string-shaped AgentSpec.Command", tok)
		}
	}

	return strings.Join(argv, " ")
}

// hasUnicodeWhitespace matches the full unicode.IsSpace set, mirroring the
// splitter (strings.Fields) that ParseCommand and requireBinary use in
// harness.go so this guard cannot miss a token that would still round-trip
// incorrectly.
func hasUnicodeWhitespace(s string) bool {
	for _, r := range s {
		if unicode.IsSpace(r) {
			return true
		}
	}
	return false
}
