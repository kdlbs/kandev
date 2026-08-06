package backendapp

import (
	"path/filepath"
	"reflect"
	"sort"
	"testing"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/task/models"
)

func TestTaskLSPDiscoveryRootsUseTrustedHostPaths(t *testing.T) {
	host := filepath.Join(string(filepath.Separator), "host")
	info := &lifecycle.WorkspaceInfo{
		ExecutorType:  string(models.ExecutorTypeLocalDocker),
		WorkspacePath: filepath.Join(string(filepath.Separator), "workspace"),
		WorkspaceFolders: []lifecycle.WorkspaceFolderSpec{
			{Name: "folder", LocalPath: filepath.Join(host, "folder")},
			{Name: "relative", LocalPath: "relative-folder"},
		},
		WorkspaceRepositories: []lifecycle.WorkspaceRepositorySpec{
			{RepoName: "b", RepositoryPath: filepath.Join(host, "repo-b")},
			{RepoName: "a", RepositoryPath: filepath.Join(host, "repo-a")},
			{RepoName: "duplicate", RepositoryPath: filepath.Join(host, "repo-b")},
		},
	}

	got := taskLSPDiscoveryRoots(info)
	want := []string{filepath.Join(host, "folder"), filepath.Join(host, "repo-a"), filepath.Join(host, "repo-b")}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Docker taskLSPDiscoveryRoots() = %v, want %v", got, want)
	}

	info.ExecutorType = string(models.ExecutorTypeLocal)
	got = taskLSPDiscoveryRoots(info)
	want = append(want, filepath.Join(string(filepath.Separator), "workspace"))
	sort.Strings(want)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Local PC taskLSPDiscoveryRoots() = %v, want %v", got, want)
	}
}
