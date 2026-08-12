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
	"github.com/kandev/kandev/internal/auth/authn"
	tasklsp "github.com/kandev/kandev/internal/lsp"
	"github.com/kandev/kandev/internal/task/models"
	usermodels "github.com/kandev/kandev/internal/user/models"
)

type fakeTaskLSPSettingsOwnerSource struct {
	task      *models.Task
	workspace *models.Workspace
}

func (f fakeTaskLSPSettingsOwnerSource) GetTask(context.Context, string) (*models.Task, error) {
	return f.task, nil
}

func (f fakeTaskLSPSettingsOwnerSource) GetWorkspace(context.Context, string) (*models.Workspace, error) {
	return f.workspace, nil
}

type recordingTaskLSPUserSettings struct {
	identity authn.Identity
	settings *usermodels.UserSettings
}

func (r *recordingTaskLSPUserSettings) GetUserSettings(ctx context.Context) (*usermodels.UserSettings, error) {
	r.identity, _ = authn.IdentityFromContext(ctx)
	return r.settings, nil
}

func TestTaskLSPSettingsComeFromTaskWorkspaceOwner(t *testing.T) {
	users := &recordingTaskLSPUserSettings{settings: &usermodels.UserSettings{
		LspAutoStartLanguages: []string{"kotlin"},
	}}
	provider := taskLSPSettingsProvider{
		users: users,
		tasks: fakeTaskLSPSettingsOwnerSource{
			task:      &models.Task{ID: "task-1", WorkspaceID: "workspace-1"},
			workspace: &models.Workspace{ID: "workspace-1", OwnerID: "owner-1"},
		},
	}
	callerCtx := authn.WithIdentity(context.Background(), authn.Identity{
		UserID: "worker-identity", Role: authn.RoleAdmin,
	})

	settings, err := provider.TaskLSPSettings(callerCtx, "task-1")
	if err != nil {
		t.Fatal(err)
	}
	if users.identity.UserID != "owner-1" || users.identity.Role != authn.RoleMember || users.identity.Synthetic {
		t.Fatalf("settings identity = %#v, want real task owner", users.identity)
	}
	if len(settings.AutoStartLanguages) != 1 || settings.AutoStartLanguages[0] != "kotlin" {
		t.Fatalf("settings = %#v", settings)
	}
}

type fakeTaskLSPWorkspaceProvider struct {
	info *taskLSPWorkspace
}

func (p fakeTaskLSPWorkspaceProvider) TaskLSPWorkspace(
	context.Context,
	string,
	string,
) (*taskLSPWorkspace, error) {
	return p.info, nil
}

type fakeTaskLSPTaskHostRuntime struct {
	host        tasklsp.TaskHost
	ensureCalls int
}

func (r *fakeTaskLSPTaskHostRuntime) EnsureTaskHost(context.Context, string, string) (tasklsp.TaskHost, error) {
	r.ensureCalls++
	return r.host, nil
}

func (r *fakeTaskLSPTaskHostRuntime) ExistingTaskHost(context.Context, string, string) (tasklsp.TaskHost, bool, error) {
	return nil, false, nil
}

func (r *fakeTaskLSPTaskHostRuntime) RecoverTaskHost(context.Context, string) (bool, error) {
	return false, nil
}

func (r *fakeTaskLSPTaskHostRuntime) CleanupTaskHost(context.Context, string, string, string) error {
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

	result, err := provider.DiscoverTaskLanguages(context.Background(), "task-1", "env-docker")
	if err != nil {
		t.Fatal(err)
	}
	if runtime.ensureCalls != 1 || host.calls != 1 || !reflect.DeepEqual(result.Languages, []string{"kotlin"}) {
		t.Fatalf("ensure=%d discover=%d result=%#v", runtime.ensureCalls, host.calls, result)
	}
}
