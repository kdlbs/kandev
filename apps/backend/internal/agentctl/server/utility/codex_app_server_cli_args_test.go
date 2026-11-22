//go:build !windows

package utility

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agent/agents"
	"go.uber.org/zap"
)

// @covers AC-AGENTS-MANAGED-RUNTIME-RECOVERY-006.11
// @covers AC-AGENTS-MANAGED-RUNTIME-RECOVERY-006.12
func TestCodexAppServerManagedProbePreservesProfileArguments(t *testing.T) {
	workDir := t.TempDir()
	argsPath := filepath.Join(workDir, "npx-args")
	writeCodexAppServerFakeNpx(t, filepath.Join(workDir, "npx"), `#!/bin/sh
printf '%s\n' "$@" > "$CODEX_APP_SERVER_ARGS.tmp"
mv "$CODEX_APP_SERVER_ARGS.tmp" "$CODEX_APP_SERVER_ARGS"
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

	response, err := NewCodexAppServerInferenceExecutor(zap.NewNop()).Probe(context.Background(), &ProbeRequest{
		AgentID: "codex-app-server", ProfileContext: true,
		InferenceConfig: &InferenceConfigDTO{
			Command:  agents.NewCodexAppServer(true).InferenceConfig().Command.Args(),
			WorkDir:  workDir,
			CLIFlags: []string{"--profile-flag", "ordered-value"},
			Env:      map[string]string{"CODEX_APP_SERVER_ARGS": argsPath},
		},
	})
	if err != nil {
		t.Fatalf("Probe returned error: %v", err)
	}
	if !response.Success {
		t.Fatalf("Probe failed: %s", response.Error)
	}
	argsRaw, err := os.ReadFile(argsPath)
	if err != nil {
		t.Fatalf("read npx arguments: %v", err)
	}
	args := strings.Fields(string(argsRaw))
	if len(args) < 7 || args[5] != "app-server" {
		t.Fatalf("npx arguments = %#v, want managed app-server invocation", args)
	}
	if got, want := args[6:], []string{"--profile-flag", "ordered-value"}; strings.Join(got, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("profile arguments = %#v, want exactly %#v", got, want)
	}
}

func TestCodexAppServerExecuteRejectsProfileArguments(t *testing.T) {
	workDir := t.TempDir()
	argsPath := filepath.Join(workDir, "npx-args")
	writeCodexAppServerFakeNpx(t, filepath.Join(workDir, "npx"), `#!/bin/sh
printf '%s\n' "$@" > "$CODEX_APP_SERVER_ARGS"
exit 1
`)
	t.Setenv("PATH", workDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("CODEX_APP_SERVER_ARGS", argsPath)

	executor := NewCodexAppServerInferenceExecutor(zap.NewNop())
	request := &PromptRequest{
		Prompt: "test prompt",
		InferenceConfig: &InferenceConfigDTO{
			Command:  agents.NewCodexAppServer(true).InferenceConfig().Command.Args(),
			WorkDir:  workDir,
			CLIFlags: []string{"--dangerously-bypass-approvals-and-sandbox"},
		},
	}
	response, err := executor.Execute(t.Context(), request)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if response.Success || response.Error != "codex app-server utility requires the built-in unwrapped command" {
		t.Fatalf("Execute response = %+v, want profile arguments rejected", response)
	}
	if _, err := os.Stat(argsPath); !os.IsNotExist(err) {
		t.Fatalf("rejected prompt started the provider: capture stat = %v", err)
	}

	request.InferenceConfig.CLIFlags = nil
	if _, err := executor.Execute(t.Context(), request); err != nil {
		t.Fatalf("Execute without profile arguments returned error: %v", err)
	}
	if _, err := os.Stat(argsPath); err != nil {
		t.Fatalf("prompt without profile arguments did not start the provider: %v", err)
	}
}
