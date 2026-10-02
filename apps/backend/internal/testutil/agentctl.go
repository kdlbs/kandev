package testutil

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// BuildAgentctl builds the credential helper into a test-owned temporary directory.
func BuildAgentctl(t testing.TB) string {
	t.Helper()
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve agentctl test helper source path")
	}
	backendDir := filepath.Clean(filepath.Join(filepath.Dir(source), "../.."))
	name := "agentctl"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	path := filepath.Join(t.TempDir(), name)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "go", "build", "-buildvcs=false", "-o", path, "./cmd/agentctl")
	cmd.Dir = backendDir
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build agentctl test helper: %v: %s", err, output)
	}
	return path
}

// ClearGitRepositoryEnvironment removes inherited repository-location variables for hermetic Git tests.
func ClearGitRepositoryEnvironment(t testing.TB) {
	t.Helper()
	for _, name := range []string{"GIT_DIR", "GIT_WORK_TREE", "GIT_INDEX_FILE", "GIT_OBJECT_DIRECTORY"} {
		previous, wasSet := os.LookupEnv(name)
		envName, previousValue := name, previous
		t.Cleanup(func() {
			if wasSet {
				_ = os.Setenv(envName, previousValue)
			} else {
				_ = os.Unsetenv(envName)
			}
		})
		if err := os.Unsetenv(name); err != nil {
			t.Fatalf("unset inherited %s: %v", name, err)
		}
	}
}

// ClearGitConfigEnvironment removes inherited indexed Git config and restores it after the test.
func ClearGitConfigEnvironment(t testing.TB) {
	t.Helper()
	names := make(map[string]struct{})
	for _, entry := range os.Environ() {
		name, _, _ := strings.Cut(entry, "=")
		if name == "GIT_CONFIG_COUNT" || name == "GIT_CONFIG_PARAMETERS" ||
			strings.HasPrefix(name, "GIT_CONFIG_KEY_") || strings.HasPrefix(name, "GIT_CONFIG_VALUE_") {
			names[name] = struct{}{}
		}
	}
	for name := range names {
		previous, wasSet := os.LookupEnv(name)
		envName, previousValue := name, previous
		t.Cleanup(func() {
			if wasSet {
				_ = os.Setenv(envName, previousValue)
			} else {
				_ = os.Unsetenv(envName)
			}
		})
		if err := os.Unsetenv(name); err != nil {
			t.Fatalf("unset inherited %s: %v", name, err)
		}
	}
}
