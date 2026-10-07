package backendapp

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/canvas"
	"github.com/kandev/kandev/internal/orchestrator"
	"github.com/kandev/kandev/internal/plugins/instances"
	"github.com/kandev/kandev/internal/plugins/manifest"
	"github.com/kandev/kandev/internal/plugins/webapp"
	"github.com/kandev/kandev/internal/task/models"
	taskservice "github.com/kandev/kandev/internal/task/service"
	"github.com/kandev/kandev/internal/worktree/copyfiles"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

func TestCanvasEditTargetDoesNotAuthorizeAnotherCanvas(t *testing.T) {
	target := canvasEditSessionTarget{
		Origin:    canvasEditOrigin,
		TaskID:    "task-edit",
		CanvasID:  "canvas-a",
		ReleaseID: "release-a",
	}

	if !canvasEditTargetMatches(target, "task-edit", "canvas-a") {
		t.Fatal("expected the trusted target to authorize its canvas")
	}
	if canvasEditTargetMatches(target, "task-edit", "canvas-b") {
		t.Fatal("trusted edit target authorized a different canvas")
	}
	if canvasEditTargetMatches(target, "other-task", "canvas-a") {
		t.Fatal("trusted edit target authorized a different task")
	}
}

func TestTaskCanvasAuthorizationUsesTaskIdentityNotCreatorSession(t *testing.T) {
	task := &models.Task{ID: "task-current", WorkspaceID: "workspace-a"}
	item := &canvas.Canvas{
		ID:                 "canvas-task",
		WorkspaceID:        task.WorkspaceID,
		TaskID:             task.ID,
		ScopeKind:          canvas.ScopeTask,
		CreatedBySessionID: "replacement-session-source",
	}
	if !taskCanvasMatchesTask(item, task) {
		t.Fatal("same-task replacement session was denied by creator-session provenance")
	}
	item.TaskID = "other-task"
	if taskCanvasMatchesTask(item, task) {
		t.Fatal("canvas from another task was authorized")
	}
	item.TaskID = task.ID
	item.WorkspaceID = "other-workspace"
	if taskCanvasMatchesTask(item, task) {
		t.Fatal("canvas from another workspace was authorized")
	}
}

func TestCanvasEditSourceEntriesUseAssignedCanvasRoot(t *testing.T) {
	entries := canvasEditSourceEntries("canvas-a", map[string][]byte{
		"manifest.yaml": []byte("manifest"),
		"index.html":    []byte("index"),
	})

	got := make(map[string][]byte, len(entries))
	for _, entry := range entries {
		got[entry.RelPath] = entry.Content
		if entry.Mode.Perm() != 0o600 {
			t.Errorf("entry %q mode = %o, want 600", entry.RelPath, entry.Mode.Perm())
		}
	}
	if string(got[".kandev/canvases/canvas-a/manifest.yaml"]) != "manifest" {
		t.Fatalf("manifest was not rooted in the assigned canvas directory: %#v", got)
	}
	if string(got[".kandev/canvases/canvas-a/index.html"]) != "index" {
		t.Fatalf("index was not rooted in the assigned canvas directory: %#v", got)
	}
}

func TestRuntimeBindingPermissionsIntersectCurrentGrants(t *testing.T) {
	declared := &manifest.Manifest{Capabilities: manifest.Capabilities{
		APIRead:  []string{"tasks", "workflows"},
		APIWrite: []string{"messages"},
		State:    true,
	}}
	grants := []instances.Grant{
		{PermissionKind: "api_read", Resource: "tasks", ScopeCeiling: instances.ScopeWorkspace},
		{PermissionKind: "api_write", Resource: "messages", ScopeCeiling: instances.ScopeTask},
	}

	got := runtimeGrantedPermissions(declared, instances.ScopeWorkspace, grants)
	if len(got) != 1 || got[0] != "api_read:tasks" {
		t.Fatalf("runtime permissions = %#v, want only declared permissions covered by grants", got)
	}
}

func TestRuntimeNetworkOriginsUseSelectedWebAppDeclaration(t *testing.T) {
	app := manifest.WebApp{NetworkOrigins: []string{
		"https://api.example.com",
		"https://unused.example.com",
	}}
	grants := []instances.Grant{
		{PermissionKind: "network", NetworkOrigin: "https://api.example.com", ScopeCeiling: instances.ScopeTask},
		{PermissionKind: "network", NetworkOrigin: "https://unrelated.example.com", ScopeCeiling: instances.ScopeTask},
	}
	got := runtimeNetworkOrigins(app, instances.ScopeTask, grants)
	if len(got) != 1 || got[0] != "https://api.example.com" {
		t.Fatalf("runtime network origins = %#v, want only the declared granted origin", got)
	}
}

func TestCanvasEditLaunchFailureDeletesEphemeralTask(t *testing.T) {
	tasks, canvasReader, releases, artifacts := newCanvasEditTestDependencies()
	launcher := &fakeCanvasEditLauncher{launchErr: errors.New("agent unavailable")}
	service := newCanvasEditService(
		tasks, canvasReader, releases, artifacts, launcher,
		&fakeCanvasEditSessionStore{}, &fakeCanvasEditExecutionResolver{},
	)

	if _, err := service.start(context.Background(), "canvas-a", "change the title"); err == nil {
		t.Fatal("expected launch failure")
	}
	if len(tasks.deleted) != 1 || tasks.deleted[0] != "task-edit" {
		t.Fatalf("deleted tasks = %#v, want the newly-created ephemeral task", tasks.deleted)
	}
}

func TestCanvasEditMaterializesSourceAndDispatchesPrompt(t *testing.T) {
	tasks, canvasReader, releases, artifacts := newCanvasEditTestDependencies()
	launcher := &fakeCanvasEditLauncher{
		launchResponse: &orchestrator.LaunchSessionResponse{
			Success:          true,
			TaskID:           "task-edit",
			SessionID:        "session-edit",
			AgentExecutionID: "execution-edit",
		},
	}
	sessions := &fakeCanvasEditSessionStore{}
	copier := &fakeCanvasEditAgentCtl{}
	resolver := &fakeCanvasEditExecutionResolver{client: copier}
	service := newCanvasEditService(tasks, canvasReader, releases, artifacts, launcher, sessions, resolver)

	result, err := service.start(context.Background(), "canvas-a", "change the title")
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if result.TaskID != "task-edit" || result.SessionID != "session-edit" || result.CanvasID != "canvas-a" {
		t.Fatalf("result = %#v, want task/session/canvas IDs", result)
	}
	if tasks.createRequest == nil || !tasks.createRequest.IsEphemeral || tasks.createRequest.Origin != canvasEditOrigin {
		t.Fatalf("create request = %#v, want ephemeral canvas-edit task", tasks.createRequest)
	}
	if tasks.createRequest.Metadata["origin"] != canvasEditOrigin || tasks.createRequest.Metadata["canvas_id"] != "canvas-a" {
		t.Fatalf("task metadata = %#v, want canvas edit origin and canvas", tasks.createRequest.Metadata)
	}
	if launcher.launchRequest == nil {
		t.Fatal("launch request was not recorded")
	}
	if launcher.launchRequest.AgentProfileID != "profile-default" || launcher.launchRequest.ExecutorID != "executor-default" {
		t.Fatalf("launch defaults = profile %q executor %q", launcher.launchRequest.AgentProfileID, launcher.launchRequest.ExecutorID)
	}
	if launcher.prompt == "" || !containsCanvasEditText(launcher.prompt, "change the title") {
		t.Fatalf("prompt = %q, want requested edit", launcher.prompt)
	}
	if !containsCanvasEditText(launcher.prompt, "api_read:tasks") || !containsCanvasEditText(launcher.prompt, "scope workspace") {
		t.Fatalf("prompt = %q, want the effective grant projection", launcher.prompt)
	}
	target, ok := sessions.values[canvasEditSessionTargetMetadataKey].(canvasEditSessionTarget)
	if !ok || !canvasEditTargetMatches(target, "task-edit", "canvas-a") {
		t.Fatalf("trusted target metadata = %#v", sessions.values[canvasEditSessionTargetMetadataKey])
	}
	if len(copier.entries) != 2 {
		t.Fatalf("copied entries = %#v, want active release source", copier.entries)
	}
	for _, entry := range copier.entries {
		if entry.RelPath != ".kandev/canvases/canvas-a/index.html" && entry.RelPath != ".kandev/canvases/canvas-a/manifest.yaml" {
			t.Fatalf("copied entry %q escaped assigned canvas root", entry.RelPath)
		}
	}
}

func TestCanvasEditMaterializationFailureDeletesEphemeralTask(t *testing.T) {
	tasks, canvasReader, releases, artifacts := newCanvasEditTestDependencies()
	launcher := &fakeCanvasEditLauncher{
		launchResponse: &orchestrator.LaunchSessionResponse{
			Success:          true,
			TaskID:           "task-edit",
			SessionID:        "session-edit",
			AgentExecutionID: "execution-edit",
		},
	}
	resolver := &fakeCanvasEditExecutionResolver{client: &fakeCanvasEditAgentCtl{err: errors.New("copy failed")}}
	service := newCanvasEditService(
		tasks, canvasReader, releases, artifacts, launcher,
		&fakeCanvasEditSessionStore{}, resolver,
	)

	if _, err := service.start(context.Background(), "canvas-a", "change the title"); err == nil {
		t.Fatal("expected source materialization failure")
	}
	if len(tasks.deleted) != 1 || tasks.deleted[0] != "task-edit" {
		t.Fatalf("deleted tasks = %#v, want the newly-created ephemeral task", tasks.deleted)
	}
}

func containsCanvasEditText(value, want string) bool {
	return len(value) >= len(want) && (value == want || len(value) > len(want) && stringContains(value, want))
}

func stringContains(value, want string) bool {
	for i := 0; i+len(want) <= len(value); i++ {
		if value[i:i+len(want)] == want {
			return true
		}
	}
	return false
}

func newCanvasEditTestDependencies() (*fakeCanvasEditTaskStore, *fakeCanvasEditCanvasStore, *fakeCanvasEditReleaseStore, *fakeCanvasEditArtifactStore) {
	return &fakeCanvasEditTaskStore{
			workspace: &models.Workspace{
				ID:                    "workspace-a",
				DefaultAgentProfileID: stringPointer("profile-default"),
				DefaultExecutorID:     stringPointer("executor-default"),
			},
		}, &fakeCanvasEditCanvasStore{canvas: &canvas.Canvas{
			ID:                  "canvas-a",
			PluginInstanceID:    "instance-a",
			WorkspaceID:         "workspace-a",
			ScopeKind:           canvas.ScopeWorkspace,
			Status:              instances.StatusActive,
			ActiveReleaseID:     "release-a",
			ActiveReleaseStatus: instances.ValidationValid,
			EffectiveGrants: []canvas.GrantProjection{{
				PermissionKind: "api_read", Resource: "tasks", ScopeCeiling: instances.ScopeWorkspace,
			}},
		}}, &fakeCanvasEditReleaseStore{release: instances.Release{
			ID:               "release-a",
			InstanceID:       "instance-a",
			PackageDigest:    "digest-a",
			ArtifactPath:     "releases/digest-a",
			ValidationStatus: instances.ValidationValid,
		}}, &fakeCanvasEditArtifactStore{files: map[string][]byte{
			"manifest.yaml": []byte("manifest"),
			"index.html":    []byte("index"),
		}}
}

func stringPointer(value string) *string { return &value }

type fakeCanvasEditTaskStore struct {
	workspace     *models.Workspace
	createRequest *taskservice.CreateTaskRequest
	deleted       []string
}

func (f *fakeCanvasEditTaskStore) AuthorizeWorkspaceAccess(context.Context, string) error { return nil }

func (f *fakeCanvasEditTaskStore) GetWorkspace(context.Context, string) (*models.Workspace, error) {
	return f.workspace, nil
}

func (f *fakeCanvasEditTaskStore) CreateTask(_ context.Context, request *taskservice.CreateTaskRequest) (taskservice.CreateTaskResult, error) {
	f.createRequest = request
	return taskservice.CreateTaskResult{
		Task:    &models.Task{ID: "task-edit", WorkspaceID: "workspace-a"},
		Outcome: taskservice.CreateTaskOutcomeCreated,
	}, nil
}

func (f *fakeCanvasEditTaskStore) DeleteTask(_ context.Context, taskID string) error {
	f.deleted = append(f.deleted, taskID)
	return nil
}

type fakeCanvasEditCanvasStore struct{ canvas *canvas.Canvas }

func (f *fakeCanvasEditCanvasStore) Get(context.Context, string) (*canvas.Canvas, error) {
	return f.canvas, nil
}

type fakeCanvasEditReleaseStore struct{ release instances.Release }

func (f *fakeCanvasEditReleaseStore) GetRelease(context.Context, string) (instances.Release, error) {
	return f.release, nil
}

type fakeCanvasEditArtifactStore struct{ files map[string][]byte }

func (f *fakeCanvasEditArtifactStore) ReadFiles(webapp.Artifact) (map[string][]byte, error) {
	return f.files, nil
}

type fakeCanvasEditLauncher struct {
	launchErr      error
	launchResponse *orchestrator.LaunchSessionResponse
	launchRequest  *orchestrator.LaunchSessionRequest
	prompt         string
}

func (f *fakeCanvasEditLauncher) LaunchSession(_ context.Context, request *orchestrator.LaunchSessionRequest) (*orchestrator.LaunchSessionResponse, error) {
	f.launchRequest = request
	return f.launchResponse, f.launchErr
}

func (f *fakeCanvasEditLauncher) PromptTask(_ context.Context, _, _, prompt, _ string, _ bool, _ []v1.MessageAttachment, _ bool) (*orchestrator.PromptResult, error) {
	f.prompt = prompt
	return &orchestrator.PromptResult{}, nil
}

type fakeCanvasEditSessionStore struct{ values map[string]interface{} }

func (f *fakeCanvasEditSessionStore) SetSessionMetadataKey(_ context.Context, _, key string, value interface{}) error {
	if f.values == nil {
		f.values = make(map[string]interface{})
	}
	f.values[key] = value
	return nil
}

type fakeCanvasEditExecutionResolver struct{ client canvasEditAgentCtl }

func (f *fakeCanvasEditExecutionResolver) ResolveAgentCtl(string) (canvasEditAgentCtl, error) {
	if f.client == nil {
		return nil, errors.New("agentctl unavailable")
	}
	return f.client, nil
}

type fakeCanvasEditAgentCtl struct {
	entries []copyfiles.Entry
	err     error
}

func (f *fakeCanvasEditAgentCtl) CopyFiles(_ context.Context, _ string, entries []copyfiles.Entry) (canvasCopyFilesResult, error) {
	f.entries = append([]copyfiles.Entry(nil), entries...)
	if f.err != nil {
		return canvasCopyFilesResult{}, f.err
	}
	return canvasCopyFilesResult{Present: true}, nil
}
