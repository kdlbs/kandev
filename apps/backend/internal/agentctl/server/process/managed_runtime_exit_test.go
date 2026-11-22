package process

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/managedruntime"
	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/pkg/agent"
)

func TestManagedRuntimeCacheProcessExitEvidence(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX process fixture")
	}
	root := t.TempDir()
	cache := filepath.Join(root, "cache")
	spec := "@agentclientprotocol/claude-agent-acp@1.2.3"
	tree := filepath.Join(cache, "_npx", managedruntime.NpxExecutionCacheKey(spec))
	pkg := filepath.Join(tree, "node_modules", "@agentclientprotocol", "claude-agent-acp")
	if err := os.MkdirAll(pkg, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pkg, "package.json"), []byte(`{"name":"@agentclientprotocol/claude-agent-acp","version":"1.2.3","bin":{"claude-agent-acp":"bin/cli.js"}}`), 0600); err != nil {
		t.Fatal(err)
	}
	for name, script := range map[string]string{
		"npx": fmt.Sprintf("#!/bin/sh\nprintf 'npm error code ENOENT\\nnpm error syscall open\\nnpm error path %%s\\n' '%s' >&2\nexit 1\n", filepath.Join(tree, "package.json")),
		"npm": fmt.Sprintf("#!/bin/sh\nprintf '%%s\\n' '%s'\n", cache),
	} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(script), 0700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", root+string(os.PathListSeparator)+os.Getenv("PATH"))
	manager := NewManager(&config.InstanceConfig{AgentType: "claude-acp", AgentArgs: agents.NewClaudeACP().ManagedNPMRuntime().RuntimeCommand("1.2.3").Args(), AgentEnv: os.Environ(), WorkDir: root, Protocol: agent.ProtocolACP}, newTestLogger(t))
	t.Cleanup(func() { _ = manager.StopForTeardown(context.Background()) })
	generation, err := manager.StartWithGeneration(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	evidence := manager.ManagedStartupEvidence(ctx, generation)
	if evidence == nil || evidence.CacheEvidence == nil {
		t.Fatalf("exit evidence = %#v, stderr=%q; want verified incomplete cache", evidence, manager.GetRecentStderr())
	}
}
