package routingerr

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/common/managednpm"
)

// fakeBinaryRecordArgs records its argv (one per line) into $ARGS_OUT and
// answers initialize successfully.
const fakeBinaryRecordArgs = `#!/bin/sh
printf '%s\n' "$@" > "$ARGS_OUT"
cat >/dev/null
echo '{"jsonrpc":"2.0","id":1,"result":{"protocolVersion":1,"agentCapabilities":{}}}'
`

func probeRecordingArgs(t *testing.T, extra []string) []string {
	t.Helper()
	dir := t.TempDir()
	out := filepath.Join(dir, "args.txt")
	bin := writeShellBin(t, dir, "fake-npx", fakeBinaryRecordArgs)
	argv := append([]string{bin}, extra...)
	resolver := func(_ context.Context, _ string) ([]string, map[string]string, bool) {
		return argv, map[string]string{"ARGS_OUT": out}, true
	}
	if got := NewACPProbe(resolver, nil).Probe(context.Background(), ProbeInput{ProviderID: "claude-acp"}); got != nil {
		t.Fatalf("probe failed: %+v", got)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read recorded args: %v", err)
	}
	return strings.Split(strings.TrimRight(string(data), "\n"), "\n")
}

func TestACPProbe_ResolvesManagedNPMPrefix(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("TMPDIR", tmp)

	got := probeRecordingArgs(t, []string{"--yes", "--prefix", managednpm.NPMProjectPrefix, "pkg@1.0.0"})

	if len(got) != 4 || got[0] != "--yes" || got[1] != "--prefix" || got[3] != "pkg@1.0.0" {
		t.Fatalf("unexpected argv: %v", got)
	}
	prefix := got[2]
	if prefix == managednpm.NPMProjectPrefix {
		t.Fatalf("prefix marker was not resolved: %v", got)
	}
	if filepath.Dir(prefix) != tmp {
		t.Fatalf("prefix %q not under TMPDIR %q", prefix, tmp)
	}
	info, err := os.Stat(prefix)
	if err != nil || !info.IsDir() {
		t.Fatalf("prefix dir missing: %v", err)
	}
	if info.Mode().Perm() != 0o700 {
		t.Fatalf("prefix mode = %v, want 0700", info.Mode().Perm())
	}
}

func TestACPProbe_LeavesArgvWithoutMarkerUnchanged(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())
	in := []string{"--yes", "--prefix", "/custom/dir", "pkg"}
	got := probeRecordingArgs(t, in)
	if strings.Join(got, "|") != strings.Join(in, "|") {
		t.Fatalf("argv changed: %v", got)
	}
}

func TestACPProbe_DoesNotMutateResolverArgv(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())
	dir := t.TempDir()
	bin := writeShellBin(t, dir, "fake-acp-ok", fakeBinaryOK)
	shared := []string{bin, "--prefix", managednpm.NPMProjectPrefix}
	probe := NewACPProbe(staticResolver(shared), nil)
	_ = probe.Probe(context.Background(), ProbeInput{ProviderID: "claude-acp"})
	if shared[2] != managednpm.NPMProjectPrefix {
		t.Fatalf("resolver slice mutated: %v", shared)
	}
}

func TestACPProbe_PrefixPreparationFailureDoesNotSpawn(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("TMPDIR", "relative-tmp")
	marker := filepath.Join(dir, "spawned")
	bin := writeShellBin(t, dir, "fake-npx", "#!/bin/sh\ntouch "+marker+"\n")
	argv := []string{bin, "--prefix", managednpm.NPMProjectPrefix, "pkg"}

	got := NewACPProbe(staticResolver(argv), nil).Probe(context.Background(), ProbeInput{ProviderID: "claude-acp"})

	if got == nil {
		t.Fatal("expected a classified error when the prefix cannot be prepared")
	}
	if _, err := os.Stat(marker); err == nil {
		t.Fatal("process was spawned despite prefix preparation failure")
	}
}
