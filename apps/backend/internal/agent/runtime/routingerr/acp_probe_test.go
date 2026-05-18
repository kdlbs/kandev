package routingerr

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// writeShellBin writes a /bin/sh script at path and chmods it 0755.
// Returns the absolute path. Cleanup is the test's responsibility via
// t.TempDir() — the directory itself is removed on test end.
func writeShellBin(t *testing.T, dir, name, body string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("acp probe test relies on POSIX shell scripts")
	}
	full := filepath.Join(dir, name)
	if err := os.WriteFile(full, []byte(body), 0o755); err != nil {
		t.Fatalf("write fake bin: %v", err)
	}
	return full
}

// staticResolver returns a CommandResolver that always yields argv for
// every provider id. Used to exercise the probe loop without wiring the
// full agent registry into a unit test.
func staticResolver(argv []string) CommandResolver {
	return func(_ string) ([]string, map[string]string, bool) {
		return argv, nil, true
	}
}

// A fake binary that prints a well-formed initialize response and
// exits 0. Mirrors the minimum shape ACPProbe.hasInitializeResult
// looks for: id + non-empty result + no error.
const fakeBinaryOK = `#!/bin/sh
# Drain stdin so the parent's Write does not block on a full pipe.
cat >/dev/null &
DRAIN=$!
echo '{"jsonrpc":"2.0","id":1,"result":{"protocolVersion":1,"agentCapabilities":{}}}'
# Keep stdin open briefly so the parent flushes the initialize frame.
wait "$DRAIN" 2>/dev/null
exit 0
`

// A fake binary that exits non-zero without speaking ACP. Used to
// assert the probe surfaces a *Error rather than nil.
const fakeBinaryFail = `#!/bin/sh
echo "Anthropic API key not set" >&2
exit 1
`

func TestACPProbe_HappyPath(t *testing.T) {
	dir := t.TempDir()
	bin := writeShellBin(t, dir, "fake-acp-ok", fakeBinaryOK)

	probe := NewACPProbe(staticResolver([]string{bin}), nil)
	got := probe.Probe(context.Background(), ProbeInput{ProviderID: "claude-acp"})
	if got != nil {
		t.Fatalf("expected nil error on successful probe, got %+v", got)
	}
}

func TestACPProbe_ClassifiesNonZeroExit(t *testing.T) {
	dir := t.TempDir()
	bin := writeShellBin(t, dir, "fake-acp-fail", fakeBinaryFail)

	probe := NewACPProbe(staticResolver([]string{bin}), nil)
	got := probe.Probe(context.Background(), ProbeInput{ProviderID: "claude-acp"})
	if got == nil {
		t.Fatal("expected non-nil error on failing probe")
	}
	// Phase must be auth_check (the probe IS the auth check). The
	// exact Code is provider-rules-dependent; we only assert the
	// classifier produced *something* with the right phase.
	if got.Phase != PhaseAuthCheck {
		t.Errorf("phase = %q, want %q", got.Phase, PhaseAuthCheck)
	}
}

func TestACPProbe_MissingResolver(t *testing.T) {
	probe := NewACPProbe(nil, nil)
	got := probe.Probe(context.Background(), ProbeInput{ProviderID: "claude-acp"})
	if got == nil {
		t.Fatal("expected error when resolver is nil")
	}
}

func TestACPProbe_ResolverReturnsNotOK(t *testing.T) {
	probe := NewACPProbe(func(_ string) ([]string, map[string]string, bool) {
		return nil, nil, false
	}, nil)
	got := probe.Probe(context.Background(), ProbeInput{ProviderID: "unknown"})
	if got == nil {
		t.Fatal("expected error when resolver yields no command")
	}
}
