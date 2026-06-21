package launcher

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestValidateRuntimeBundleAcceptsSingleBinaryLayout(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "bin", "kandev"))
	writeFile(t, filepath.Join(dir, "bin", "agentctl"))
	if requiresAgentctlLinuxAMD64(runtime.GOOS, runtime.GOARCH) {
		writeFile(t, filepath.Join(dir, "bin", "agentctl-linux-amd64"))
	}

	bundle, err := validateRuntimeBundle(dir, "test")
	if err != nil {
		t.Fatal(err)
	}
	if bundle.Launcher != filepath.Join(dir, "bin", "kandev") {
		t.Fatalf("Launcher = %q", bundle.Launcher)
	}
}

func TestValidateRuntimeBundleRejectsMissingLauncher(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "bin", "agentctl"))

	if _, err := validateRuntimeBundle(dir, "test"); err == nil {
		t.Fatal("expected error")
	}
}

func TestRequiresAgentctlLinuxAMD64(t *testing.T) {
	if requiresAgentctlLinuxAMD64("linux", "amd64") {
		t.Fatal("linux/amd64 should use the native agentctl binary")
	}
	if !requiresAgentctlLinuxAMD64("linux", "arm64") {
		t.Fatal("linux/arm64 should require the linux/amd64 helper")
	}
	if !requiresAgentctlLinuxAMD64("darwin", "arm64") {
		t.Fatal("non-linux hosts should require the linux/amd64 helper")
	}
}

func writeFile(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
}
