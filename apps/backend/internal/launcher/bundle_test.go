package launcher

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidateRuntimeBundleAcceptsSingleBinaryLayout(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "bin", "kandev"))
	writeFile(t, filepath.Join(dir, "bin", "agentctl"))
	writeFile(t, filepath.Join(dir, "web", "server.js"))

	bundle, err := validateRuntimeBundle(dir, "test")
	if err != nil {
		t.Fatal(err)
	}
	if bundle.Launcher != filepath.Join(dir, "bin", "kandev") {
		t.Fatalf("Launcher = %q", bundle.Launcher)
	}
	if bundle.WebServer != filepath.Join(dir, "web", "server.js") {
		t.Fatalf("WebServer = %q", bundle.WebServer)
	}
}

func TestValidateRuntimeBundleRejectsMissingLauncher(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "bin", "agentctl"))
	writeFile(t, filepath.Join(dir, "web", "server.js"))

	if _, err := validateRuntimeBundle(dir, "test"); err == nil {
		t.Fatal("expected error")
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
