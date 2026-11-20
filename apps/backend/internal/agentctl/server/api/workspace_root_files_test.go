package api

import (
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	agentctltypes "github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/common/logger"
)

func TestWorkspaceRootFilesRouteListsMultiRepoRootAndDoesNotRunGit(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"frontend", "backend"} {
		initWorkspaceRootFilesGitRepo(t, filepath.Join(root, name))
	}
	if err := os.WriteFile(filepath.Join(root, "dashboard.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(t.TempDir(), "git-invoked")
	if runtime.GOOS != "windows" {
		realGit, err := exec.LookPath("git")
		if err != nil {
			t.Fatal(err)
		}
		binDir := t.TempDir()
		fakeGit := filepath.Join(binDir, "git")
		shim := "#!/bin/sh\nprintf called >> \"$GIT_MARKER\"\nexec \"$REAL_GIT\" \"$@\"\n"
		if err := os.WriteFile(fakeGit, []byte(shim), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(marker, nil, 0o600); err != nil {
			t.Fatal(err)
		}
		pathParts := append([]string{binDir}, filepath.SplitList(os.Getenv("PATH"))...)
		t.Setenv("PATH", strings.Join(pathParts, string(os.PathListSeparator)))
		t.Setenv("GIT_MARKER", marker)
		t.Setenv("REAL_GIT", realGit)
	}
	server := newWorkspaceRootFilesServer(t, root)
	if runtime.GOOS != "windows" {
		if err := os.WriteFile(marker, nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}

	response := workspaceRequest(t, server, http.MethodGet, "/api/v1/workspace/root-files", nil)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %s)", response.Code, response.Body.String())
	}
	listing := decodeWorkspaceBody[agentctltypes.WorkspaceRootFiles](t, response)
	if !listing.Supported || listing.Truncated || len(listing.Files) != 1 || listing.Files[0].Path != "dashboard.json" {
		t.Fatalf("listing = %+v, want dashboard.json", listing)
	}
	if len(listing.Destinations) != 2 || listing.Destinations[0].RepositoryName != "backend" || listing.Destinations[1].RepositoryName != "frontend" {
		t.Fatalf("destinations = %+v, want backend then frontend", listing.Destinations)
	}
	if runtime.GOOS != "windows" {
		invocations, err := os.ReadFile(marker)
		if err != nil {
			t.Fatal(err)
		}
		if len(invocations) != 0 {
			t.Fatalf("root-file route invoked git: %q", invocations)
		}
	}
}

func TestWorkspaceRootFilesRouteReturnsUnsupportedForGitRoot(t *testing.T) {
	root := t.TempDir()
	initWorkspaceRootFilesGitRepo(t, root)
	if err := os.WriteFile(filepath.Join(root, "visible.txt"), []byte("visible"), 0o644); err != nil {
		t.Fatal(err)
	}
	response := workspaceRequest(t, newWorkspaceRootFilesServer(t, root), http.MethodGet, "/api/v1/workspace/root-files", nil)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %s)", response.Code, response.Body.String())
	}
	listing := decodeWorkspaceBody[agentctltypes.WorkspaceRootFiles](t, response)
	if listing.Supported || len(listing.Files) != 0 || len(listing.Destinations) != 0 {
		t.Fatalf("listing = %+v, want unsupported and empty", listing)
	}
}

func newWorkspaceRootFilesServer(t *testing.T, root string) *Server {
	t.Helper()
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error"})
	if err != nil {
		t.Fatal(err)
	}
	cfg := &config.InstanceConfig{WorkDir: root}
	return NewServer(cfg, process.NewManager(cfg, log), nil, nil, log)
}

func initWorkspaceRootFilesGitRepo(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("git", "init", "--quiet", path)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init %s: %v (%s)", path, err, output)
	}
	if err := os.WriteFile(filepath.Join(path, "tracked.txt"), []byte("tracked"), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd = exec.Command("git", "-C", path, "add", "tracked.txt")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git add in %s: %v (%s)", path, err, output)
	}
}
