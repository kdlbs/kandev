package launcher

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateRuntimeBundleAcceptsSingleBinaryLayout(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "bin", "kandev"))
	writeFile(t, filepath.Join(dir, "bin", "agentctl"))
	writeRemoteAgentctlHelpers(t, dir)

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

func TestValidateRuntimeBundleRejectsMissingRemoteHelper(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "bin", "kandev"))
	writeFile(t, filepath.Join(dir, "bin", "agentctl"))
	writeFile(t, filepath.Join(dir, "bin", "agentctl-linux-amd64"))
	writeFile(t, filepath.Join(dir, "bin", "agentctl-darwin-amd64"))

	_, err := validateRuntimeBundle(dir, "test")
	if err == nil {
		t.Fatal("expected error")
	}
	if got, want := err.Error(), "agentctl darwin/arm64 helper not found"; !strings.Contains(got, want) {
		t.Fatalf("error = %q, want substring %q", got, want)
	}
}

func TestRequiredAgentctlRemoteHelpers(t *testing.T) {
	got := make([]string, 0, len(requiredAgentctlRemoteHelpers))
	for _, helper := range requiredAgentctlRemoteHelpers {
		got = append(got, helper.Name)
	}
	want := []string{
		"agentctl-linux-amd64",
		"agentctl-darwin-arm64",
		"agentctl-darwin-amd64",
	}
	if len(got) != len(want) {
		t.Fatalf("helpers = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("helpers = %v, want %v", got, want)
		}
	}
}

func writeRemoteAgentctlHelpers(t *testing.T, dir string) {
	t.Helper()
	for _, helper := range requiredAgentctlRemoteHelpers {
		writeFile(t, filepath.Join(dir, "bin", helper.Name))
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
