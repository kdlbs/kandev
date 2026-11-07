//go:build !windows

package utility

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/managedruntime"
	"go.uber.org/zap"
)

func TestManagedRuntimeCacheNativeCodexProbeCapturesPreInitializeExit(t *testing.T) {
	root := t.TempDir()
	cache := filepath.Join(root, "cache")
	spec := "@openai/codex@0.154.0"
	tree := filepath.Join(cache, "_npx", managedruntime.NpxExecutionCacheKey(spec))
	manifest := filepath.Join(tree, "node_modules", "@openai", "codex", "package.json")
	if err := os.MkdirAll(filepath.Dir(manifest), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifest, []byte(`{"name":"@openai/codex","version":"0.154.0","bin":{"codex":"bin/codex.js"}}`), 0600); err != nil {
		t.Fatal(err)
	}
	writeCodexAppServerFakeNpx(t, filepath.Join(root, "npx"), fmt.Sprintf("#!/bin/sh\nprintf 'npm error code ENOENT\\nnpm error syscall open\\nnpm error path %%s\\n' '%s' >&2\nexit 1\n", filepath.Join(tree, "package.json")))
	writeCodexAppServerFakeNpx(t, filepath.Join(root, "npm"), fmt.Sprintf("#!/bin/sh\nprintf '%%s\\n' '%s'\n", cache))
	t.Setenv("PATH", root+string(os.PathListSeparator)+os.Getenv("PATH"))
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	executor := NewCodexAppServerInferenceExecutor(zap.NewNop())
	response, err := executor.Probe(ctx, &ProbeRequest{AgentID: "codex-app-server", InferenceConfig: &InferenceConfigDTO{Command: agents.NewCodexAppServer(true).ManagedNPMRuntime().RuntimeCommand("0.154.0").Args(), WorkDir: root, Env: map[string]string{"NPM_CONFIG_CACHE": cache}}})
	if err != nil {
		t.Fatal(err)
	}
	if response.FailureCode != ProbeFailureManagedRuntimeCacheIncomplete || response.CacheEvidence == nil || response.CacheEvidence.PackageSpec != spec || response.CacheEvidence.ProbeAttemptID == "" {
		t.Fatalf("native pre-initialize probe = %#v; want bound cache evidence", response)
	}
}
