package utility

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agent/antigravityruntime"
	"github.com/kandev/kandev/internal/agentctl/acpcompat"
)

func TestAntigravityManagedCommandAdmission(t *testing.T) {
	home := t.TempDir()
	t.Setenv("KANDEV_HOME_DIR", home)
	artifact, err := antigravityruntime.ArtifactFor(runtime.GOOS, runtime.GOARCH)
	if err != nil {
		t.Fatal(err)
	}
	dest := antigravityruntime.ManagedDestination(home, artifact)
	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatal(err)
	}
	server := filepath.Join(dest, artifact.ServerName)
	harness := filepath.Join(dest, artifact.HarnessNames[0])
	if err := os.WriteFile(server, []byte("server"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(harness, []byte("harness"), 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := antigravityruntime.Manifest{
		Version: artifact.Version, OS: artifact.OS, Arch: artifact.Arch,
		Digest: artifact.SHA256, Server: artifact.ServerName, Harness: artifact.HarnessNames[0],
	}
	raw, _ := json.Marshal(manifest)
	if err := os.WriteFile(filepath.Join(dest, antigravityruntime.ManifestName), raw, 0o644); err != nil {
		t.Fatal(err)
	}
	conflictDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(conflictDir, artifact.ServerName), []byte("conflict"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(conflictDir, artifact.HarnessNames[0]), []byte("conflict"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", conflictDir)
	cfg := &InferenceConfigDTO{Command: []string{server}}
	got, errText := resolveSpawnCommandContext(context.Background(), cfg)
	if errText != "" {
		t.Fatalf("resolveSpawnCommandContext: %s", errText)
	}
	if got != server {
		t.Fatalf("admitted command = %q, want managed %q", got, server)
	}
	untrusted := &InferenceConfigDTO{Command: []string{filepath.Join(conflictDir, artifact.ServerName)}}
	if _, errText := resolveSpawnCommandContext(context.Background(), untrusted); errText == "" {
		t.Fatal("untrusted absolute runtime path was admitted")
	}
}

func TestAntigravityUtilityEnvironmentPolicy(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if runtime.GOOS == "windows" {
		t.Setenv("USERPROFILE", home)
	}
	t.Setenv("KANDEV_HOME_DIR", t.TempDir())
	cfg := &InferenceConfigDTO{
		AuthPolicy: acpcompat.AntigravityAuthPolicy(false),
	}
	harness := filepath.Join(t.TempDir(), "harness")
	if err := os.WriteFile(harness, []byte("harness"), 0o755); err != nil {
		t.Fatal(err)
	}
	env := []string{
		"HOME=" + home,
		"ANTIGRAVITY_HARNESS_PATH=" + harness,
		"GEMINI_API_KEY=secret",
		"GOOGLE_API_KEY=secret",
		"BROWSER=real-browser",
		"UNRELATED=kept",
	}
	prepared, err := prepareAntigravityUtilityEnv(context.Background(), acpcompat.AntigravityAgentID, cfg, env)
	if err != nil {
		t.Fatalf("prepare env: %v", err)
	}
	joined := strings.Join(prepared, "\n")
	if strings.Contains(joined, "GEMINI_API_KEY") || strings.Contains(joined, "GOOGLE_API_KEY") {
		t.Fatalf("credential input survived: %s", joined)
	}
	if !strings.Contains(joined, "UNRELATED=kept") || !strings.Contains(joined, acpcompat.AntigravityBrowserHelperCommand) {
		t.Fatalf("prepared env = %s", joined)
	}
	conflict := append([]string{"GEMINI_HOME=/tmp/other"}, env...)
	if _, err := prepareAntigravityUtilityEnv(context.Background(), acpcompat.AntigravityAgentID, cfg, conflict); err == nil {
		t.Fatal("ambient GEMINI_HOME conflict was accepted")
	}
}

func TestAntigravityUtilityStderrRedaction(t *testing.T) {
	var stderr stderrBuffer
	consent := stderr.consent()
	line := "Open https://accounts.google.com/o/oauth2/v2/auth?client_id=x&state=secret&code_challenge=c&redirect_uri=http%3A%2F%2F127.0.0.1%3A5555%2Fcb\n"
	half := len(line) / 2
	_, _ = stderr.Write([]byte(line[:half]))
	_, _ = stderr.Write([]byte(line[half:]))
	select {
	case got := <-consent:
		if !strings.Contains(got, "accounts.google.com") {
			t.Fatalf("consent URL = %q", got)
		}
	default:
		t.Fatal("consent URL was not observed")
	}
	tail := stderr.tail()
	if strings.Contains(tail, "accounts.google.com") || strings.Contains(tail, "state=secret") {
		t.Fatalf("retained stderr leaked consent URL: %q", tail)
	}
	_, _ = stderr.Write([]byte("https://accounts.google.com/o/oauth2/malformed?state=leak\n"))
	if strings.Contains(stderr.tail(), "state=leak") {
		t.Fatalf("malformed consent URL leaked: %q", stderr.tail())
	}
}
