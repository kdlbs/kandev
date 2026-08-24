package process

import (
	"context"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/server/config"
)

func TestNpmCacheRootFromOutputUsesAbsolutePath(t *testing.T) {
	cacheRoot := t.TempDir()
	output := []byte(cacheRoot + "\n")

	got, err := npmCacheRootFromOutput(output)
	if err != nil {
		t.Fatalf("npmCacheRootFromOutput: %v", err)
	}
	if got != cacheRoot {
		t.Fatalf("cache root = %q, want %q", got, cacheRoot)
	}
}

func TestNpmCacheRootFromOutputRejectsMixedStreams(t *testing.T) {
	cacheRoot := t.TempDir()
	output := []byte("npm warning using configured registry\n" + cacheRoot + "\n")
	if _, err := npmCacheRootFromOutput(output); err == nil {
		t.Fatal("npmCacheRootFromOutput() = nil error, want mixed-stream output rejected")
	}
}

func TestNpmCacheRootFromOutputRejectsNonPathOutput(t *testing.T) {
	if _, err := npmCacheRootFromOutput([]byte("npm cache unavailable\nrelative/cache\n")); err == nil {
		t.Fatal("npmCacheRootFromOutput() = nil error, want rejection")
	}
}

func TestRepairManagedRuntimeCacheRejectsUnversionedSpecBeforeCommand(t *testing.T) {
	mgr := NewManager(&config.InstanceConfig{WorkDir: t.TempDir()}, newTestLogger(t))
	t.Cleanup(func() { _ = mgr.StopForTeardown(context.Background()) })

	err := mgr.RepairManagedRuntimeCache(context.Background(), "managed-acp")
	if err == nil {
		t.Fatal("RepairManagedRuntimeCache(unversioned) = nil, want rejection")
	}
	if strings.Contains(err.Error(), "npm") {
		t.Fatalf("validation error = %q, should not start npm", err)
	}
}

func TestRepairManagedRuntimeCacheClearsPreviousStderr(t *testing.T) {
	cacheRoot := t.TempDir()
	packageSpec := "managed-acp@1.2.3"
	mgr := NewManager(&config.InstanceConfig{
		WorkDir:  t.TempDir(),
		AgentEnv: []string{"NPM_CONFIG_CACHE=" + cacheRoot},
	}, newTestLogger(t))
	t.Cleanup(func() { _ = mgr.StopForTeardown(context.Background()) })
	mgr.appendStderr("stale first-attempt error")

	if err := mgr.RepairManagedRuntimeCache(context.Background(), packageSpec); err != nil {
		t.Fatalf("RepairManagedRuntimeCache: %v", err)
	}
	if got := mgr.GetRecentStderr(); len(got) != 0 {
		t.Fatalf("stderr after repair = %#v, want empty", got)
	}
}
