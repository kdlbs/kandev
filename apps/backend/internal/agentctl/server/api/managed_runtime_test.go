package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/kandev/kandev/internal/agent/managedruntime"
	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/server/process"
)

func TestManagedRuntimeCacheRepairUsesProbeEnvironmentAndExactTree(t *testing.T) {
	log := newTestLogger()
	instanceCacheRoot, _ := filepath.EvalSymlinks(t.TempDir())
	probeCacheRoot, _ := filepath.EvalSymlinks(t.TempDir())
	binDir, _ := filepath.EvalSymlinks(t.TempDir())
	npmPath := filepath.Join(binDir, "npm")
	fixture := "#!/bin/sh\nprintf '%s\\n' \"$NPM_CONFIG_CACHE\"\n"
	if err := os.WriteFile(npmPath, []byte(fixture), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	cfg := &config.InstanceConfig{
		Port:     45321,
		WorkDir:  t.TempDir(),
		AgentEnv: []string{"NPM_CONFIG_CACHE=" + instanceCacheRoot, "PATH=" + binDir + string(os.PathListSeparator) + os.Getenv("PATH")},
	}
	procMgr := process.NewManager(cfg, log)
	server := NewServer(cfg, procMgr, nil, nil, log)

	packageSpec := "@scope/managed-acp@1.2.3"
	probeNpxRoot := filepath.Join(probeCacheRoot, "_npx")
	target := filepath.Join(probeNpxRoot, managedruntime.NpxExecutionCacheKey(packageSpec))
	sibling := filepath.Join(probeNpxRoot, "0123456789abcdef")
	instanceTarget := filepath.Join(
		instanceCacheRoot, "_npx", managedruntime.NpxExecutionCacheKey(packageSpec),
	)
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(sibling, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(instanceTarget, 0o755); err != nil {
		t.Fatal(err)
	}

	body, err := json.Marshal(map[string]any{
		"package_spec": packageSpec,
		"env":          map[string]string{"NPM_CONFIG_CACHE": probeCacheRoot, "PATH": binDir + string(os.PathListSeparator) + os.Getenv("PATH")},
	})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/agent/managed-runtime/cache-repair", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	server.router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("cache repair status = %d, want %d: %s", resp.Code, http.StatusOK, resp.Body.String())
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("target stat error = %v, want not-exist", err)
	}
	if _, err := os.Stat(sibling); err != nil {
		t.Fatalf("unrelated tree was removed: %v", err)
	}
	if _, err := os.Stat(instanceTarget); err != nil {
		t.Fatalf("instance-environment tree was removed instead of probe tree: %v", err)
	}
}
