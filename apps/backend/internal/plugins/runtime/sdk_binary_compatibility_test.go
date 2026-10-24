package runtime

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	goruntime "runtime"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/plugins/manifest"
	"github.com/kandev/kandev/internal/plugins/store"
	"github.com/kdlbs/kandev/pluginsdk"
	"github.com/stretchr/testify/require"
)

// v0.97.0, peeled and checked against the public annotated release tag.
const legacySDKRevision = "e43881c7555372897b57ec51c705f1e05da43c40"

// @covers AC-PLUGINS-SDK-DISTRIBUTION-004.1
func TestLegacySDKBinaryHostState(t *testing.T) {
	if testing.Short() {
		t.Skip("legacy subprocess build")
	}
	bin := buildLegacySDKPlugin(t)
	platform := goruntime.GOOS + "-" + goruntime.GOARCH
	rec := &store.Record{Manifest: manifest.Manifest{
		ID: "legacy-sdk-probe", Version: "1.0.0", APIVersion: 1,
		Runtime: manifest.Runtime{Type: "binary", Executables: map[string]string{platform: filepath.Base(bin)}},
	}, InstallPath: filepath.Dir(bin), Status: store.StatusRegistered}
	host := newFakeHost()
	manager := NewManager(t.TempDir(), nil, testLogger(t))
	t.Cleanup(manager.StopAll)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	require.NoError(t, manager.Start(ctx, rec, func(string) pluginsdk.Host { return host }))
	remote, ok := manager.Get(rec.ID)
	require.True(t, ok, "legacy process must complete handshake")
	require.Eventually(t, func() bool {
		if err := remote.DeliverEvent(ctx, &pluginsdk.Event{EventID: "legacy-event", EventType: "task.created"}); err != nil {
			return false
		}
		value, found := host.get("instance", "", "legacy-event")
		return found && value["event_id"] == "legacy-event"
	}, 5*time.Second, 20*time.Millisecond, "legacy binary must call the extracted Host over gRPC")
}

func buildLegacySDKPlugin(t *testing.T) string {
	t.Helper()
	root, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	require.NoError(t, err)
	dir := t.TempDir()
	archive := filepath.Join(dir, "legacy.tar")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	command := exec.CommandContext(ctx, "git", "archive", "--output", archive, legacySDKRevision,
		"apps/backend/go.mod", "apps/backend/go.sum", "apps/backend/pkg/pluginsdk", "apps/backend/proto")
	command.Dir = string(root[:len(root)-1])
	out, err := command.CombinedOutput()
	require.NoError(t, err, "legacy source must be present; use a full checkout: %s", out)
	out, err = exec.CommandContext(ctx, "tar", "-xf", archive, "-C", dir).CombinedOutput()
	require.NoError(t, err, "%s", out)
	backend := filepath.Join(dir, "apps", "backend")
	source := `package main
import ("context"; "github.com/kandev/kandev/pkg/pluginsdk")
type probe struct { pluginsdk.UnimplementedPlugin }
func (p *probe) OnEvent(ctx context.Context, e *pluginsdk.Event) error {
 if p.Host() == nil { return nil }
 return p.Host().SetState(ctx, "instance", "", "legacy-event", map[string]any{"event_id": e.EventID})
}
func main() { pluginsdk.Serve(&probe{}) }
`
	require.NoError(t, os.WriteFile(filepath.Join(backend, "legacy_probe.go"), []byte(source), 0o600))
	binary := filepath.Join(t.TempDir(), "legacy-plugin")
	if goruntime.GOOS == "windows" {
		binary += ".exe"
	}
	command = exec.CommandContext(ctx, "go", "build", "-trimpath", "-o", binary, "legacy_probe.go")
	command.Dir = backend
	command.Env = append(os.Environ(), "GOWORK=off", "GOENV=off", "GOFLAGS=")
	out, err = command.CombinedOutput()
	require.NoError(t, err, "%s", out)
	return binary
}
