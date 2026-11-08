package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/server/process"
)

func TestWorkspaceRepositoryLocatorRejections(t *testing.T) {
	for _, locator := range []string{
		"/srv/repos/private.git",
		"file:///srv/repos/private.git",
		"C:/repos/private.git",
		`\\server\share\repo.git`,
		"../relative/repo",
		"https://token@github.com/org/repo.git",
		"ssh://git@github.com/org/repo.git",
		"git://github.com/org/repo.git?ref=main",
		"https://github.com/org/repo.git#main",
		"git@github.com:",
		"git@github.com:/org/repo.git",
		"git@github.com:org//repo.git",
		"git@github.com:org/../repo.git",
		"git@github.com:org/-repo.git",
		"git@github.com:org/repo.git/",
		"git@github.com:org/re po.git",
		"git@github.com:org/repo\t.git",
		"git@github.com:org/repo\n.git",
		"git@github.com:org/repo@2.git",
		"git@github.com:2222:org/repo.git",
		"git@[2001:db8::1]:org/repo.git",
		"git@github..com:org/repo.git",
		"git@-github.com:org/repo.git",
		"git@github.com:org/repo%2f.git",
		"git@github.com:org/repo.git?ref=main",
		"ssh@github.com:org/repo.git",
		"ftp://github.com/org/repo.git",
		"not a locator",
	} {
		t.Run(locator, func(t *testing.T) {
			if err := validateRepositoryLocator(locator); err == nil {
				t.Fatalf("validateRepositoryLocator(%q) accepted an unsupported locator", locator)
			}
		})
	}
}

func TestWorkspaceRepositoryLocatorAcceptsNetworkAndSCP(t *testing.T) {
	for _, locator := range []string{
		"https://github.com/org/repo.git",
		"http://git.internal/team/repo.git",
		"ssh://github.com/org/repo.git",
		"git://github.com/org/repo.git",
		"git@github.com:org/repo.git",
		"git@gitlab.com:group/subgroup/project.git",
		"git@git.internal:team/repo",
	} {
		t.Run(locator, func(t *testing.T) {
			if err := validateRepositoryLocator(locator); err != nil {
				t.Fatalf("validateRepositoryLocator(%q) = %v, want acceptance", locator, err)
			}
		})
	}
}

func TestWorkspaceSCPLocatorMaterializeAndRemove(t *testing.T) {
	origin := createMaterializeOrigin(t)
	locator := "git@scp.test:acme/repository.git"
	workDir := t.TempDir()
	server := newSCPMaterializeTestServer(t, workDir, origin, locator)

	w := materializeRepositoryRequest(t, server, MaterializeRepositoryRequest{
		RepositoryURL: locator,
		Destination:   "repository",
		BaseBranch:    "main",
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("materialize status = %d, want 201 (body: %s)", w.Code, w.Body.String())
	}
	destination := filepath.Join(workDir, "repository")
	if _, err := os.Stat(filepath.Join(destination, ".git")); err != nil {
		t.Fatalf("cloned repository missing: %v", err)
	}
	if got := strings.TrimSpace(materializeGitOutputForTest(t, destination, "remote", "get-url", "origin")); got != locator {
		t.Fatalf("recorded origin = %q, want submitted SCP locator %q", got, locator)
	}

	w = removeMaterializedRepositoryRequest(t, server, RemoveMaterializedRepositoryRequest{
		RepositoryURL: locator,
		Destination:   "repository",
	})
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"removed":true`) {
		t.Fatalf("remove status = %d body = %s, want removed checkout", w.Code, w.Body.String())
	}
	if _, err := os.Stat(destination); !os.IsNotExist(err) {
		t.Fatalf("materialized checkout remains after removal: %v", err)
	}
}

func TestWorkspaceSCPLocatorRejectionDoesNotEchoOrMutate(t *testing.T) {
	workDir := t.TempDir()
	server := newMaterializeTestServer(t, workDir)
	for _, locator := range []string{
		"git@github.com:org/../repo.git",
		"git@github.com:org/repo extra.git",
		"git@github.com:org/repo%2fextra.git",
		"git@github.com:2222:org/repo.git",
		"git@[2001:db8::1]:org/repo.git",
		"git@-github.com:org/repo.git",
	} {
		t.Run(locator, func(t *testing.T) {
			w := materializeRepositoryRequest(t, server, MaterializeRepositoryRequest{
				RepositoryURL: locator,
				Destination:   "repository",
				BaseBranch:    "main",
			})
			if w.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400 (body: %s)", w.Code, w.Body.String())
			}
			if strings.Contains(w.Body.String(), locator) {
				t.Fatalf("response echoed the rejected locator: %s", w.Body.String())
			}
			if _, err := os.Stat(filepath.Join(workDir, "repository")); !os.IsNotExist(err) {
				t.Fatalf("rejected locator mutated the workspace: %v", err)
			}
		})
	}
}

func newSCPMaterializeTestServer(t *testing.T, workDir, origin, locator string) *Server {
	t.Helper()
	configPath := filepath.Join(t.TempDir(), "gitconfig")
	gitConfig := fmt.Sprintf("[url \"file://%s\"]\n\tinsteadOf = %s\n", filepath.ToSlash(origin), locator)
	if err := os.WriteFile(configPath, []byte(gitConfig), 0o600); err != nil {
		t.Fatalf("write git config: %v", err)
	}
	environment := append([]string(nil), os.Environ()...)
	environment = append(environment, "GIT_CONFIG_GLOBAL="+configPath, "GIT_CONFIG_NOSYSTEM=1")
	log := newTestLogger()
	cfg := &config.InstanceConfig{Port: 0, WorkDir: workDir, AuthToken: "test-token", AgentEnv: environment}
	return NewServer(cfg, process.NewManager(cfg, log), nil, nil, log)
}

func materializeRepositoryRequest(t *testing.T, s *Server, request MaterializeRepositoryRequest) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/workspace/materialize-repository", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer test-token")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)
	return w
}
