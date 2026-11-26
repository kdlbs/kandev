package backendapp

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/coordinator"
	"github.com/kandev/kandev/internal/github"
	"github.com/kandev/kandev/internal/gitlab"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

type noteFakeTasks map[string]*taskmodels.Task

func (f noteFakeTasks) GetTask(_ context.Context, id string) (*taskmodels.Task, error) {
	if t, ok := f[id]; ok {
		return t, nil
	}
	return nil, repoerrors.ErrTaskNotFound
}

type noteFakeRepos map[string]*taskmodels.Repository

func (f noteFakeRepos) GetRepository(_ context.Context, id string) (*taskmodels.Repository, error) {
	if r, ok := f[id]; ok {
		return r, nil
	}
	return nil, repoerrors.ErrRepositoryNotFound
}

type noteFakeTurns map[string]bool

func (f noteFakeTurns) TurnExists(_ context.Context, _, id string) (bool, error) { return f[id], nil }

func gitIn(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v %s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func TestNoteResolverCitations(t *testing.T) {
	dir := t.TempDir()
	gitIn(t, dir, "init", "-q", "-b", "main")
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("one"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "b.txt"), []byte("same"), 0o600); err != nil {
		t.Fatal(err)
	}
	gitIn(t, dir, "add", ".")
	gitIn(t, dir, "commit", "-q", "-m", "first")
	first := gitIn(t, dir, "rev-parse", "HEAD")
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("two"), 0o600); err != nil {
		t.Fatal(err)
	}
	gitIn(t, dir, "commit", "-q", "-am", "second")

	repos := noteFakeRepos{
		"r1":      {ID: "r1", WorkspaceID: "ws", LocalPath: dir, DefaultBranch: "main"},
		"noclone": {ID: "noclone", WorkspaceID: "ws", DefaultBranch: "main"},
		"foreign": {ID: "foreign", WorkspaceID: "other", LocalPath: dir, DefaultBranch: "main"},
	}
	res := &coordinatorNoteResolver{
		tasks: noteFakeTasks{"t1": {ID: "t1", WorkspaceID: "ws"}, "t2": {ID: "t2", WorkspaceID: "other"}},
		repos: repos, turns: noteFakeTurns{"turn1": true},
	}
	checker := res.Open(context.Background(), "ws", "c1")
	cases := []struct {
		name string
		cite coordinator.NoteCitation
		want coordinator.CitationState
	}{
		{"task", coordinator.NoteCitation{Kind: coordinator.CitationTask, ID: "t1"}, coordinator.CitationPresent},
		{"missing task", coordinator.NoteCitation{Kind: coordinator.CitationTask, ID: "zz"}, coordinator.CitationAbsent},
		{"foreign task", coordinator.NoteCitation{Kind: coordinator.CitationTask, ID: "t2"}, coordinator.CitationAbsent},
		{"turn", coordinator.NoteCitation{Kind: coordinator.CitationTurn, ID: "turn1"}, coordinator.CitationPresent},
		{"missing turn", coordinator.NoteCitation{Kind: coordinator.CitationTurn, ID: "nope"}, coordinator.CitationAbsent},
		{"unchanged file", coordinator.NoteCitation{Kind: coordinator.CitationFile, RepositoryID: "r1", Path: "b.txt", SHA: first}, coordinator.CitationPresent},
		{"changed file", coordinator.NoteCitation{Kind: coordinator.CitationFile, RepositoryID: "r1", Path: "a.txt", SHA: first}, coordinator.CitationAbsent},
		{"missing path", coordinator.NoteCitation{Kind: coordinator.CitationFile, RepositoryID: "r1", Path: "gone.txt", SHA: first}, coordinator.CitationAbsent},
		{"unknown commit", coordinator.NoteCitation{Kind: coordinator.CitationFile, RepositoryID: "r1", Path: "a.txt", SHA: strings.Repeat("a", 40)}, coordinator.CitationAbsent},
		{"no clone", coordinator.NoteCitation{Kind: coordinator.CitationFile, RepositoryID: "noclone", Path: "a.txt", SHA: first}, coordinator.CitationUnavailable},
		{"missing repo", coordinator.NoteCitation{Kind: coordinator.CitationFile, RepositoryID: "zz", Path: "a.txt", SHA: first}, coordinator.CitationAbsent},
		{"foreign repo", coordinator.NoteCitation{Kind: coordinator.CitationFile, RepositoryID: "foreign", Path: "a.txt", SHA: first}, coordinator.CitationAbsent},
		{"pr without provider", coordinator.NoteCitation{Kind: coordinator.CitationPR, RepositoryID: "r1", Number: 1}, coordinator.CitationUnavailable},
	}
	for _, tc := range cases {
		if got := checker.Check(context.Background(), tc.cite); got != tc.want {
			t.Errorf("%s: got %v want %v", tc.name, got, tc.want)
		}
	}
}

func TestChangeRequestMissing(t *testing.T) {
	gl404 := fmt.Errorf("wrapped: %w", &gitlab.APIError{StatusCode: http.StatusNotFound})
	gl500 := &gitlab.APIError{StatusCode: http.StatusInternalServerError}
	gh404 := fmt.Errorf("wrapped: %w", &github.GitHubAPIError{StatusCode: http.StatusNotFound})
	cases := []struct {
		name     string
		provider string
		err      error
		want     bool
	}{
		{"gitlab 404", mergeProviderGitLab, gl404, true},
		{"gitlab 500", mergeProviderGitLab, gl500, false},
		{"github 404", mergeProviderGitHub, gh404, true},
		{"github error on gitlab repo", mergeProviderGitLab, gh404, false},
		{"gitlab error on github repo", mergeProviderGitHub, gl404, false},
		{"plain error", mergeProviderGitLab, fmt.Errorf("boom"), false},
	}
	for _, tc := range cases {
		if got := changeRequestMissing(tc.provider, tc.err); got != tc.want {
			t.Errorf("%s: got %v, want %v", tc.name, got, tc.want)
		}
	}
}
