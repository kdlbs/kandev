package backendapp

import (
	"context"
	"net/http"
	"path/filepath"
	"reflect"
	"sort"
	"testing"

	"github.com/gorilla/websocket"
	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	tasklsp "github.com/kandev/kandev/internal/lsp"
	"github.com/kandev/kandev/internal/task/models"
)

type fakeTaskLSPWorkspaceProvider struct {
	info *taskLSPWorkspace
}

func (p fakeTaskLSPWorkspaceProvider) TaskLSPWorkspace(
	context.Context,
	string,
) (*taskLSPWorkspace, error) {
	return p.info, nil
}

type fakeTaskLSPTaskHostRuntime struct {
	host        tasklsp.TaskHost
	ensureCalls int
}

func (r *fakeTaskLSPTaskHostRuntime) EnsureTaskHost(context.Context, string) (tasklsp.TaskHost, error) {
	r.ensureCalls++
	return r.host, nil
}

func (r *fakeTaskLSPTaskHostRuntime) ExistingTaskHost(context.Context, string) (tasklsp.TaskHost, bool, error) {
	return nil, false, nil
}

func (r *fakeTaskLSPTaskHostRuntime) RecoverTaskHost(context.Context, string) (bool, error) {
	return false, nil
}

func (r *fakeTaskLSPTaskHostRuntime) CleanupTaskHost(context.Context, string, string) error {
	return nil
}

type fakeTaskLSPDiscoveryHost struct {
	result *tasklsp.DiscoveryResult
	calls  int
}

func (h *fakeTaskLSPDiscoveryHost) DiscoverLSP(context.Context) (*tasklsp.DiscoveryResult, error) {
	h.calls++
	return h.result, nil
}

func (*fakeTaskLSPDiscoveryHost) RefreshTaskLSPWorkspace(context.Context) (*tasklsp.WorkspaceUpdateResult, error) {
	return nil, nil
}
func (*fakeTaskLSPDiscoveryHost) StartTaskLSP(context.Context, tasklsp.TaskHostStartRequest) (*tasklsp.RuntimeSnapshot, error) {
	return nil, nil
}
func (*fakeTaskLSPDiscoveryHost) RestartTaskLSP(context.Context, tasklsp.TaskHostStartRequest) (*tasklsp.RuntimeSnapshot, error) {
	return nil, nil
}
func (*fakeTaskLSPDiscoveryHost) UpdateTaskLSPConfiguration(context.Context, tasklsp.TaskHostConfigurationRequest) (*tasklsp.RuntimeSnapshot, error) {
	return nil, nil
}
func (*fakeTaskLSPDiscoveryHost) StopTaskLSP(context.Context, tasklsp.TaskHostStopRequest) (*tasklsp.RuntimeSnapshot, error) {
	return nil, nil
}
func (*fakeTaskLSPDiscoveryHost) TaskLSPSnapshot(context.Context, string) (*tasklsp.RuntimeSnapshot, error) {
	return nil, nil
}
func (*fakeTaskLSPDiscoveryHost) WatchTaskLSP(context.Context, string, func(tasklsp.RuntimeSnapshot) error) error {
	return nil
}
func (*fakeTaskLSPDiscoveryHost) DialTaskLSPAttach(context.Context, string, uint64) (*websocket.Conn, *http.Response, error) {
	return nil, nil, nil
}

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

func TestDockerTaskLSPDiscoveryRunsInsideTaskHost(t *testing.T) {
	host := &fakeTaskLSPDiscoveryHost{result: &tasklsp.DiscoveryResult{
		Languages: []string{"kotlin"}, State: tasklsp.DetectionComplete,
	}}
	runtime := &fakeTaskLSPTaskHostRuntime{host: host}
	provider := taskLSPRuntimeProvider{
		taskHosts: runtime,
		tasks: fakeTaskLSPWorkspaceProvider{info: &taskLSPWorkspace{
			executorType: models.ExecutorTypeLocalDocker,
		}},
	}

	result, err := provider.DiscoverTaskLanguages(context.Background(), "env-docker")
	if err != nil {
		t.Fatal(err)
	}
	if runtime.ensureCalls != 1 || host.calls != 1 || !reflect.DeepEqual(result.Languages, []string{"kotlin"}) {
		t.Fatalf("ensure=%d discover=%d result=%#v", runtime.ensureCalls, host.calls, result)
	}
}
