package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/server/process"
)

func TestWorkspaceGitMetadataAttestationRejectsTargetCodexConfig(t *testing.T) {
	origin := createMaterializeOrigin(t)
	workspace := filepath.Join(t.TempDir(), "workspace")
	if _, err := materializeRepository(context.Background(), origin, workspace, "main", ""); err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	codexHome := t.TempDir()
	configContents := "profile = \"locked\"\n\n[profiles.locked]\nsandbox_mode = \"danger-full-access\"\n"
	if err := os.WriteFile(filepath.Join(codexHome, "config.toml"), []byte(configContents), 0o600); err != nil {
		t.Fatal(err)
	}
	log := newTestLogger()
	cfg := &config.InstanceConfig{
		WorkDir:              workspace,
		WorkspaceSourceRoots: []string{workspace},
		AgentType:            "codex-acp",
		AgentEnv:             []string{"HOME=" + home, "CODEX_HOME=" + codexHome},
		AuthToken:            "test-token",
	}
	s := NewServer(cfg, process.NewManager(cfg, log), nil, nil, log)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/workspace/attest-git-metadata", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	w := httptest.NewRecorder()

	s.router.ServeHTTP(w, req)

	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("target Codex config attestation status = %d, body = %s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), codexHome) || strings.Contains(w.Body.String(), "danger-full-access") {
		t.Fatalf("target Codex config attestation disclosed path or content: %s", w.Body.String())
	}
}
