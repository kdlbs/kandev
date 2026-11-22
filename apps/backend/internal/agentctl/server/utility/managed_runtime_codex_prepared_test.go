//go:build !windows

package utility

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/managedruntime"
	agentctltypes "github.com/kandev/kandev/internal/agentctl/types"
	"go.uber.org/zap"
)

func TestManagedRuntimePreparationNativeCodexProbeReplacesFailedProcess(t *testing.T) {
	req, argsPath := nativeCodexPreparationFixture(t)
	original := append([]string(nil), req.InferenceConfig.Command...)
	executor := NewCodexAppServerInferenceExecutor(zap.NewNop())
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	failed, err := executor.Probe(ctx, req)
	if err != nil || failed.CacheEvidence == nil {
		t.Fatalf("initial native probe = %#v, %v", failed, err)
	}
	req.Preparation = &agentctltypes.ManagedRuntimePreparationDirective{Condition: failed.CacheEvidence.Condition, PackageSpec: failed.CacheEvidence.PackageSpec, ProbeAttemptID: failed.CacheEvidence.ProbeAttemptID}
	recovered, err := executor.Probe(ctx, req)
	if err != nil || !recovered.Success {
		t.Fatalf("prepared native probe = %#v, %v", recovered, err)
	}
	if len(recovered.Models) != 1 || recovered.Models[0].ID != "gpt-fake" {
		t.Fatalf("native models = %#v", recovered.Models)
	}
	args, err := os.ReadFile(argsPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(args), "--cache\n") || !strings.Contains(string(args), "@openai/codex@0.154.0\napp-server") {
		t.Fatalf("prepared native arguments = %q", args)
	}
	if strings.Join(req.InferenceConfig.Command, " ") != strings.Join(original, " ") {
		t.Fatal("private preparation mutated logical probe identity")
	}
}

func nativeCodexPreparationFixture(t *testing.T) (*ProbeRequest, string) {
	t.Helper()
	root := t.TempDir()
	cache := filepath.Join(root, "cache")
	argsPath := filepath.Join(root, "native-args")
	manifest := filepath.Join(cache, "_npx", managedruntime.NpxExecutionCacheKey("@openai/codex@0.154.0"), "package.json")
	writePrivateRuntimePackageManifest(t, filepath.Join(filepath.Dir(manifest), "node_modules", "@openai", "codex", "package.json"), "@openai/codex", "0.154.0")
	initial := fmt.Sprintf("#!/bin/sh\nif [ \"$5\" != \"--cache\" ]; then\nprintf 'npm error code ENOENT\\nnpm error syscall open\\nnpm error path %%s\\n' '%s' >&2\nexit 1\nfi\n", manifest)
	writeCodexAppServerFakeNpx(t, filepath.Join(root, "npx"), initial+`printf '%s\n' "$@" > "$NATIVE_PREPARED_ARGS"
while IFS= read -r request; do
 id=$(printf '%s\n' "$request" | sed -n 's/.*"id":\([0-9][0-9]*\).*/\1/p')
 case "$request" in
  *'"method":"initialize"'*) printf '{"id":%s,"result":{}}\n' "$id" ;;
  *'"method":"model/list"'*) printf '{"id":%s,"result":{"data":[{"id":"gpt-fake","displayName":"Fake model"}]}}\n' "$id" ;;
 esac
done
`)
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	installer := fmt.Sprintf("#!/bin/sh\nif [ \"$5\" = \"exec\" ]; then\n KANDEV_TEST_NATIVE_INSTALLER=1 exec '%s' -test.run=^TestNativeCodexPrivateInstallerHelper$ -- \"$@\"\nfi\nprintf '%%s\\n' '%s'\n", binary, cache)
	writeCodexAppServerFakeNpx(t, filepath.Join(root, "npm"), installer)
	t.Setenv("PATH", root+string(os.PathListSeparator)+os.Getenv("PATH"))
	return &ProbeRequest{AgentID: "codex-app-server", InferenceConfig: &InferenceConfigDTO{Command: agents.NewCodexAppServer(true).ManagedNPMRuntime().RuntimeCommand("0.154.0").Args(), WorkDir: root, Env: map[string]string{"NATIVE_PREPARED_ARGS": argsPath, "NPM_CONFIG_CACHE": cache}}}, argsPath
}

func TestNativeCodexPrivateInstallerHelper(t *testing.T) {
	if os.Getenv("KANDEV_TEST_NATIVE_INSTALLER") != "1" {
		return
	}
	cache := ""
	for i, arg := range os.Args {
		if arg == "--cache" && i+1 < len(os.Args) {
			cache = os.Args[i+1]
		}
	}
	if cache == "" {
		t.Fatal("private cache argument is required")
	}
	tree := filepath.Join(cache, "_npx", managedruntime.NpxExecutionCacheKey("@openai/codex@0.154.0"))
	pkg := filepath.Join(tree, "node_modules", "@openai", "codex")
	if err := os.MkdirAll(filepath.Join(pkg, "bin"), 0700); err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(map[string]any{"name": "@openai/codex", "version": "0.154.0", "bin": map[string]string{"codex": "bin/codex.js"}})
	for path, data := range map[string][]byte{filepath.Join(tree, "package.json"): []byte(`{}`), filepath.Join(pkg, "package.json"): data, filepath.Join(pkg, "bin", "codex.js"): []byte("#!/bin/sh\nexit 0\n")} {
		if err := os.WriteFile(path, data, 0700); err != nil {
			t.Fatal(err)
		}
	}
	bin := filepath.Join(tree, "node_modules", ".bin")
	if err := os.MkdirAll(bin, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("../@openai/codex/bin/codex.js", filepath.Join(bin, "codex")); err != nil {
		t.Fatal(err)
	}
}
