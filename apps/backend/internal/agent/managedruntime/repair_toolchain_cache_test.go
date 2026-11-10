package managedruntime

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

// @covers AC-AGENTS-MANAGED-RUNTIME-RECOVERY-006.7
// @covers AC-AGENTS-MANAGED-RUNTIME-RECOVERY-007.19
func TestRepairNPMVersionMemoization(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("npm memoization fixture uses a POSIX shell executable")
	}
	bin := t.TempDir()
	counter := filepath.Join(bin, "counter")
	script := "#!/bin/sh\n" +
		"count=0\n" +
		"if [ -f \"$COUNTER\" ]; then count=$(cat \"$COUNTER\"); fi\n" +
		"echo $((count + 1)) > \"$COUNTER\"\n" +
		"echo 11.0.0\n"
	if err := os.WriteFile(filepath.Join(bin, "npm"), []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	node := "#!/bin/sh\necho 24.0.0\n"
	if err := os.WriteFile(filepath.Join(bin, "node"), []byte(node), 0o700); err != nil {
		t.Fatal(err)
	}
	env := []string{"PATH=" + bin + string(os.PathListSeparator) + os.Getenv("PATH"), "COUNTER=" + counter}
	npm := filepath.Join(bin, "npm")
	for attempt := 0; attempt < 2; attempt++ {
		version, err := repairNPMVersion(context.Background(), npm, env)
		if err != nil || version != "11.0.0" {
			t.Fatalf("version %d = (%q, %v)", attempt, version, err)
		}
	}
	if got := readCounter(t, counter); got != 1 {
		t.Fatalf("npm version command calls = %d, want one memoized call", got)
	}
	otherTaskEnv := append(append([]string(nil), env...),
		"KANDEV_TASK_ID=another-task", "CLAUDE_CONFIG_DIR=/another/task", "NPM_CONFIG_REGISTRY=https://changed.example.test/")
	if _, err := repairNPMVersion(context.Background(), npm, otherTaskEnv); err != nil {
		t.Fatal(err)
	}
	if got := readCounter(t, counter); got != 1 {
		t.Fatalf("npm version command calls across tasks = %d, want the existing version fact", got)
	}

	firstKey, err := repairNPMVersionFactKey(npm, env)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(npm)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(npm, []byte(script+"# replaced\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	replacementTime := info.ModTime().Add(2 * time.Second)
	if err := os.Chtimes(npm, replacementTime, replacementTime); err != nil {
		t.Fatal(err)
	}
	secondKey, err := repairNPMVersionFactKey(npm, env)
	if err != nil {
		t.Fatal(err)
	}
	if firstKey == secondKey {
		t.Fatalf("executable change did not change the version fact key: %s", firstKey)
	}
	if _, err := repairNPMVersion(context.Background(), npm, env); err != nil {
		t.Fatal(err)
	}
	if got := readCounter(t, counter); got != 2 {
		t.Fatalf("changed executable calls = %d, want a fresh lookup", got)
	}

	for index, setting := range []string{"NODE_OPTIONS=--no-warnings", "NODE_PATH=" + t.TempDir()} {
		changedEnv := append(append([]string(nil), env...), setting)
		if _, err := repairNPMVersion(context.Background(), npm, changedEnv); err != nil {
			t.Fatal(err)
		}
		if got, want := readCounter(t, counter), index+3; got != want {
			t.Fatalf("changed %s calls = %d, want %d for a fresh lookup", setting, got, want)
		}
	}

	cancelEnv := append(append([]string(nil), env...), "NODE_OPTIONS=--trace-warnings")
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := repairNPMVersion(cancelled, npm, cancelEnv); err == nil {
		t.Fatal("cancelled lookup succeeded")
	}
	if _, err := repairNPMVersion(context.Background(), npm, cancelEnv); err != nil {
		t.Fatal(err)
	}
	if got := readCounter(t, counter); got != 5 {
		t.Fatalf("cancelled lookup poisoned the cache: calls = %d, want a fresh successful lookup", got)
	}
}

func readCounter(t *testing.T, path string) int {
	t.Helper()
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	count, err := strconv.Atoi(strings.TrimSpace(string(contents)))
	if err != nil {
		t.Fatal(err)
	}
	return count
}
