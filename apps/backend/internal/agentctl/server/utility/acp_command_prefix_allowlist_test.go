package utility

import "testing"

func TestResolveACPCommandPrefixAllowsFixedKandevGuard(t *testing.T) {
	t.Parallel()

	if got := resolveACPCommandPrefix(kandevAgentGuardPath); got != kandevAgentGuardPath {
		t.Fatalf("resolveACPCommandPrefix(%q) = %q, want fixed guard path", kandevAgentGuardPath, got)
	}
}

func TestResolveACPCommandPrefixRejectsGuardLookalikes(t *testing.T) {
	t.Parallel()

	for _, name := range []string{
		"kandev-agent-guard",
		"/tmp/kandev-agent-guard",
		"/usr/local/bin/kandev-agent-guard-copy",
	} {
		if got := resolveACPCommandPrefix(name); got != "" {
			t.Errorf("resolveACPCommandPrefix(%q) = %q, want empty", name, got)
		}
	}
}

func TestResolveACPCommandPrefixRetainsExistingAllowlist(t *testing.T) {
	t.Parallel()

	if got := resolveACPCommandPrefix("npx"); got != "npx" {
		t.Fatalf("resolveACPCommandPrefix(npx) = %q, want npx", got)
	}
}
