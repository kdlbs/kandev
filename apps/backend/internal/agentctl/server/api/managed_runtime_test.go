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

func TestManagedRuntimeCacheRepairUsesAgentEnvironmentAndExactTree(t *testing.T) {
	server := newTestServer(t)
	cacheRoot := t.TempDir()
	server.cfg.AgentEnv = []string{"NPM_CONFIG_CACHE=" + cacheRoot}

	packageSpec := "@scope/managed-acp@1.2.3"
	npxRoot := filepath.Join(cacheRoot, "_npx")
	target := filepath.Join(npxRoot, managedruntime.NpxExecutionCacheKey(packageSpec))
	sibling := filepath.Join(npxRoot, "0123456789abcdef")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(sibling, 0o755); err != nil {
		t.Fatal(err)
	}

	body, err := json.Marshal(map[string]string{"package_spec": packageSpec})
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
}
