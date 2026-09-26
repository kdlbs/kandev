//go:build !windows

package utility

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/managedruntime"
	"go.uber.org/zap"
)

// @covers AC-AGENTS-CODEX-NATIVE-001.1
func TestCodexAppServerProbeDiscoversModelsFromGeneratedManagedCommand(t *testing.T) {
	workDir := t.TempDir()
	argsPath := filepath.Join(workDir, "npx-args")
	writeCodexAppServerFakeNpx(t, filepath.Join(workDir, "npx"), `#!/bin/sh
printf '%s\n' "$@" > "$CODEX_APP_SERVER_ARGS"
while IFS= read -r request; do
  id=$(printf '%s\n' "$request" | sed -n 's/.*"id":\([0-9][0-9]*\).*/\1/p')
  case "$request" in
    *'"method":"initialize"'*)
      printf '{"id":%s,"result":{}}\n' "$id"
      ;;
    *'"method":"model/list"'*)
      printf '{"id":%s,"result":{"data":[{"id":"gpt-fake","displayName":"Fake model"}]}}\n' "$id"
      ;;
  esac
done
`)
	t.Setenv("PATH", workDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("CODEX_APP_SERVER_ARGS", argsPath)

	command := agents.NewCodexAppServer(true).InferenceConfig().Command.Args()
	originalCommand := slices.Clone(command)
	response, err := NewCodexAppServerInferenceExecutor(zap.NewNop()).Probe(context.Background(), &ProbeRequest{
		InferenceConfig: &InferenceConfigDTO{Command: command, WorkDir: workDir},
	})
	if err != nil {
		t.Fatalf("Probe returned error: %v", err)
	}
	if !response.Success {
		t.Fatalf("Probe failed: %s", response.Error)
	}
	if len(response.Models) != 1 || response.Models[0].ID != "gpt-fake" || response.Models[0].Name != "Fake model" {
		t.Fatalf("models = %#v, want fake model", response.Models)
	}
	if !slices.Equal(command, originalCommand) {
		t.Fatalf("Probe mutated generated command: got %#v, want %#v", command, originalCommand)
	}

	argsRaw, err := os.ReadFile(argsPath)
	if err != nil {
		t.Fatalf("read npx arguments: %v", err)
	}
	args := strings.Fields(string(argsRaw))
	if len(args) != 6 || args[0] != "--yes" || args[1] != "--prefer-offline" || args[2] != "--prefix" {
		t.Fatalf("npx arguments = %#v, want managed app-server invocation", args)
	}
	if !isPreparedNPMPrefix(args[3], workDir) {
		t.Fatalf("npx prefix = %q, want prepared absolute private prefix", args[3])
	}
	if args[4] != "@openai/codex@0.154.0" || args[5] != "app-server" {
		t.Fatalf("npx package and args = %#v", args[4:])
	}
}

// @covers AC-AGENTS-CODEX-NATIVE-002.1
func TestCodexAppServerStartPreparesManagedPrefixForSharedUtilityLaunch(t *testing.T) {
	workDir := t.TempDir()
	argsPath := filepath.Join(workDir, "npx-args")
	writeCodexAppServerFakeNpx(t, filepath.Join(workDir, "npx"), `#!/bin/sh
printf '%s\n' "$@" > "$CODEX_APP_SERVER_ARGS"
cat >/dev/null
`)
	t.Setenv("PATH", workDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("CODEX_APP_SERVER_ARGS", argsPath)

	command := agents.NewCodexAppServer(true).InferenceConfig().Command.Args()
	originalCommand := slices.Clone(command)
	cfg := &InferenceConfigDTO{Command: command, WorkDir: workDir}
	executor := NewCodexAppServerInferenceExecutor(zap.NewNop())
	resolved, args, err := resolveCodexAppServerCommand(cfg)
	if err != nil {
		t.Fatalf("resolve command: %v", err)
	}
	_, cleanup, _, err := executor.start(context.Background(), resolved, args, cfg)
	if err != nil {
		t.Fatalf("start utility process: %v", err)
	}
	t.Cleanup(cleanup)
	waitForCodexAppServerArgs(t, argsPath)
	if !slices.Equal(command, originalCommand) {
		t.Fatalf("utility launch mutated generated command: got %#v, want %#v", command, originalCommand)
	}

	argsRaw, err := os.ReadFile(argsPath)
	if err != nil {
		t.Fatalf("read npx arguments: %v", err)
	}
	args = strings.Fields(string(argsRaw))
	if len(args) < 4 || args[0] != "--yes" || args[1] != "--prefer-offline" || args[2] != "--prefix" {
		t.Fatalf("npx arguments = %#v, want prepared managed prefix", args)
	}
	if !isPreparedNPMPrefix(args[3], workDir) {
		t.Fatalf("npx prefix = %q, want prepared absolute private prefix", args[3])
	}
}

func TestCodexAppServerStartRejectsUnpreparableManagedPrefixBeforeSpawn(t *testing.T) {
	workDir := t.TempDir()
	argsPath := filepath.Join(workDir, "npx-args")
	writeCodexAppServerFakeNpx(t, filepath.Join(workDir, "npx"), `#!/bin/sh
printf '%s\n' "$@" > "$CODEX_APP_SERVER_ARGS"
`)
	t.Setenv("PATH", workDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("CODEX_APP_SERVER_ARGS", argsPath)
	invalidTempRoot := filepath.Join(workDir, "not-a-directory")
	if err := os.WriteFile(invalidTempRoot, []byte("file"), 0o600); err != nil {
		t.Fatalf("create invalid temporary root: %v", err)
	}
	t.Setenv("TMPDIR", invalidTempRoot)

	cfg := &InferenceConfigDTO{
		Command: agents.NewCodexAppServer(true).InferenceConfig().Command.Args(),
		WorkDir: workDir,
	}
	command, args, err := resolveCodexAppServerCommand(cfg)
	if err != nil {
		t.Fatalf("resolve command: %v", err)
	}
	_, _, _, err = NewCodexAppServerInferenceExecutor(zap.NewNop()).start(context.Background(), command, args, cfg)
	if err == nil || !strings.Contains(err.Error(), "prepare managed npm project prefix") {
		t.Fatalf("start error = %v, want managed npm prefix preparation error", err)
	}
	if _, err := os.Stat(argsPath); !os.IsNotExist(err) {
		t.Fatalf("fake npx started despite preparation failure; stat error = %v", err)
	}
}

func isPreparedNPMPrefix(prefix, workDir string) bool {
	return filepath.IsAbs(prefix) && prefix != managedruntime.NPMProjectPrefix && prefix != workDir &&
		filepath.Dir(filepath.Clean(prefix)) == filepath.Clean(os.TempDir()) &&
		strings.HasPrefix(filepath.Base(prefix), "kandev-managed-npm-runtime-")
}

func waitForCodexAppServerArgs(t *testing.T, path string) {
	t.Helper()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	for {
		if _, err := os.Stat(path); err == nil {
			return
		}
		select {
		case <-ticker.C:
		case <-timer.C:
			t.Fatalf("fake npx did not capture its arguments at %s", path)
		}
	}
}

func writeCodexAppServerFakeNpx(t *testing.T, path, script string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake npx executable: %v", err)
	}
}
