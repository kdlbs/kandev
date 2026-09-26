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
)

func TestManagedRuntimeCacheRepairUsesProbeEnvironmentAndExactTree(t *testing.T) {
	server := newTestServer(t)
	instanceCacheRoot := t.TempDir()
	probeCacheRoot := t.TempDir()
	server.cfg.AgentEnv = []string{"NPM_CONFIG_CACHE=" + instanceCacheRoot}

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
		"env":          map[string]string{"NPM_CONFIG_CACHE": probeCacheRoot},
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
