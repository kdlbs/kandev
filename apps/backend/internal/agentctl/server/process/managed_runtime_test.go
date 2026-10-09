package process

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agent/managedruntime"
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

func TestRepairManagedRuntimeCacheUsesIsolatedNpmPrefix(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test uses a POSIX npm fixture")
	}
	home := t.TempDir()
	workDir := t.TempDir()
	cacheRoot := t.TempDir()
	binDir := t.TempDir()
	argsFile := filepath.Join(t.TempDir(), "npm-args")
	cwdFile := filepath.Join(t.TempDir(), "npm-cwd")
	npmPath := filepath.Join(binDir, "npm")
	fixture := "#!/bin/sh\n" +
		"printf '%s\\n' \"$@\" > \"$MANAGED_NPM_ARGS_FILE\"\n" +
		"printf '%s' \"$PWD\" > \"$MANAGED_NPM_CWD_FILE\"\n" +
		"printf '%s\\n' \"$NPM_CONFIG_CACHE\"\n"
	if err := os.WriteFile(npmPath, []byte(fixture), 0o755); err != nil {
		t.Fatalf("write npm fixture: %v", err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	packageSpec := "@scope/managed-acp@1.2.3"
	target := filepath.Join(cacheRoot, "_npx", managedruntime.NpxExecutionCacheKey(packageSpec))
	if err := os.MkdirAll(target, 0o700); err != nil {
		t.Fatalf("create execution cache: %v", err)
	}

	manager := NewManager(&config.InstanceConfig{
		WorkDir: workDir,
		AgentEnv: []string{
			"HOME=" + home,
			"PATH=" + binDir + string(os.PathListSeparator) + "/usr/bin",
			"NPM_CONFIG_CACHE=" + cacheRoot,
			"MANAGED_NPM_ARGS_FILE=" + argsFile,
			"MANAGED_NPM_CWD_FILE=" + cwdFile,
		},
	}, newTestLogger(t))
	t.Cleanup(func() { _ = manager.StopForTeardown(context.Background()) })

	if err := manager.RepairManagedRuntimeCache(context.Background(), packageSpec); err != nil {
		t.Fatalf("RepairManagedRuntimeCache: %v", err)
	}
	args, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatalf("read npm arguments: %v", err)
	}
	argLines := strings.Split(strings.TrimSpace(string(args)), "\n")
	if len(argLines) != 5 {
		t.Fatalf("npm args = %q, want five arguments", args)
	}
	prefix := argLines[1]
	if !filepath.IsAbs(prefix) || !strings.HasPrefix(filepath.Clean(prefix), filepath.Clean(os.TempDir())+string(filepath.Separator)) {
		t.Fatalf("npm project prefix = %q, want an absolute path under %q", prefix, os.TempDir())
	}
	wantArgs := "--prefix\n" + prefix + "\nconfig\nget\ncache\n"
	if string(args) != wantArgs {
		t.Fatalf("npm args = %q, want %q", args, wantArgs)
	}
	cwd, err := os.ReadFile(cwdFile)
	if err != nil || string(cwd) != workDir {
		t.Fatalf("npm working directory = %q, error = %v, want workspace %q", cwd, err, workDir)
	}
	if info, err := os.Stat(prefix); err != nil || !info.IsDir() {
		t.Fatalf("managed npm prefix was not provisioned: info=%v err=%v", info, err)
	}
	if _, err := os.Stat(filepath.Join(home, ".kandev", "managed-npm-runtime")); !os.IsNotExist(err) {
		t.Fatalf("managed npm prefix was created under child HOME, stat error = %v", err)
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("execution tree stat error = %v, want tree removed", err)
	}
}
