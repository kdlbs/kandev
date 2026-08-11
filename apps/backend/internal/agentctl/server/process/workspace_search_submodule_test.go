package process

import (
	"context"
	"reflect"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/server/config"
)

func TestManagerSearchWorkspaceFileResultsIncludesRootAndSubmodule(t *testing.T) {
	mgr := setupWorkspaceSearchSubmoduleGraph(t)
	root, repositories := mgr.snapshotTrackers()
	root.updateFiles(context.Background())
	for _, tracker := range repositories {
		tracker.updateFiles(context.Background())
	}

	results := mgr.SearchWorkspaceFileResults("search-target", 20)
	got := make([]string, 0, len(results))
	for _, result := range results {
		got = append(got, result.RepositoryName+"|"+result.Path)
	}
	want := []string{
		"|root-search-target.txt",
		"vendor/lib|vendor/lib/child-search-target.txt",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("search results = %v, want %v", got, want)
	}
}

func TestManagerSearchWorkspaceContentIncludesRootAndSubmodule(t *testing.T) {
	mgr := setupWorkspaceSearchSubmoduleGraph(t)

	response, err := mgr.SearchWorkspaceContent(context.Background(), "shared-content-needle", 20)
	if err != nil {
		t.Fatalf("SearchWorkspaceContent: %v", err)
	}
	got := make([]string, 0, len(response.Results))
	for _, result := range response.Results {
		got = append(got, result.RepositoryName+"|"+result.Path)
	}
	want := []string{
		"|root-search-target.txt",
		"vendor/lib|child-search-target.txt",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("content search results = %v, want %v", got, want)
	}
}

func setupWorkspaceSearchSubmoduleGraph(t *testing.T) *Manager {
	t.Helper()
	parent, parentCleanup := setupTestRepo(t)
	t.Cleanup(parentCleanup)
	child, childCleanup := setupTestRepo(t)
	t.Cleanup(childCleanup)

	writeFile(t, parent, "root-search-target.txt", "shared-content-needle\n")
	writeFile(t, child, "child-search-target.txt", "shared-content-needle\n")
	runGit(t, child, "add", ".")
	runGit(t, child, "commit", "-m", "add search fixture")
	runGit(t, parent, "-c", "protocol.file.allow=always", "submodule", "add", child, "vendor/lib")
	runGit(t, parent, "add", ".")
	runGit(t, parent, "commit", "-m", "add submodule search fixture")
	runGit(t, parent, "push", "origin", "main")

	mgr := NewManager(&config.InstanceConfig{WorkDir: parent}, newTestLogger(t))
	t.Cleanup(mgr.stopWorkspaceTrackers)
	return mgr
}
