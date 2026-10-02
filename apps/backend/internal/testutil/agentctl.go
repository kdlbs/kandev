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
	name := "agentctl"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	path := filepath.Join(t.TempDir(), name)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	moduleFile, err := exec.CommandContext(ctx, "go", "env", "GOMOD").Output()
	if err != nil {
		t.Fatalf("resolve agentctl test helper module: %v", err)
	}
	modulePath := strings.TrimSpace(string(moduleFile))
	if modulePath == "" || modulePath == os.DevNull {
		t.Fatal("resolve agentctl test helper module: not inside a Go module")
	}
	backendDir := filepath.Dir(modulePath)
	cmd := exec.CommandContext(ctx, "go", "build", "-trimpath", "-buildvcs=false", "-o", path, "./cmd/agentctl")
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
