package plugins

import (
	"context"
	"errors"
	"testing"

	agentsettingsdto "github.com/kandev/kandev/internal/agent/settings/dto"
	"github.com/kandev/kandev/internal/plugins/manifest"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	"github.com/kandev/kandev/pkg/pluginsdk"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ── Write gating: denied without api_write:<resource> ───────────────────

func TestPluginHost_Tasks_CreateDeniedWithoutWriteCapability(t *testing.T) {
	// api_read:tasks is granted but api_write:tasks is NOT — proving the two
	// capabilities gate independently: a read-only plugin cannot write.
	d := newTestDataHost(manifest.Capabilities{APIRead: []string{"tasks"}})

	_, err := d.host.Tasks().Create(context.Background(), pluginsdk.CreateTaskInput{Title: "x", WorkspaceID: "ws-1", WorkflowID: "wf-1"})
	assertPermissionDenied(t, err, "api_write:tasks")

	_, err = d.host.Tasks().Update(context.Background(), pluginsdk.UpdateTaskInput{ID: "task-1"})
	assertPermissionDenied(t, err, "api_write:tasks")

	if d.taskWriter.createCalls != 0 || d.taskWriter.updateCalls != 0 {
		t.Fatalf("task writer called despite missing api_write:tasks (create=%d update=%d)", d.taskWriter.createCalls, d.taskWriter.updateCalls)
	}
}

func TestPluginHost_Messages_SendDeniedWithoutWriteCapability(t *testing.T) {
	// api_read:messages granted but api_write:messages is NOT — the read and
	// write capabilities on the messages resource gate independently.
	d := newTestDataHost(manifest.Capabilities{APIRead: []string{"messages"}})

	_, err := d.host.Messages().Send(context.Background(), "task-1", "", "hello")
	assertPermissionDenied(t, err, "api_write:messages")
	if d.messenger.calls != 0 {
		t.Fatalf("messenger called %d times despite missing api_write:messages", d.messenger.calls)
	}
}

// TestPluginHost_Tasks_WriteOnlyCannotRead is the mirror of the read-only case:
// a plugin with only api_write:tasks can Create but cannot List/Get — the two
// capabilities are enforced per-method on the same accessor.
func TestPluginHost_Tasks_WriteOnlyCannotRead(t *testing.T) {
	d := newTestDataHost(manifest.Capabilities{APIWrite: []string{"tasks"}})

	_, _, err := d.host.Tasks().List(context.Background(), pluginsdk.TaskFilter{}, pluginsdk.Page{})
	assertPermissionDenied(t, err, "api_read:tasks")

	_, err = d.host.Tasks().Create(context.Background(), pluginsdk.CreateTaskInput{Title: "x", WorkspaceID: "ws-1", WorkflowID: "wf-1"})
	if err != nil {
		t.Fatalf("Create() with api_write:tasks unexpected error: %v", err)
	}
}

// ── Task create ─────────────────────────────────────────────────────────

func TestPluginHost_Tasks_CreateSucceedsAndStampsSource(t *testing.T) {
	d := newTestDataHost(manifest.Capabilities{APIWrite: []string{"tasks"}})
	d.taskWriter.created = &taskmodels.Task{ID: "task-9", WorkspaceID: "ws-1", WorkflowID: "wf-1", Title: "Investigate"}

	task, err := d.host.Tasks().Create(context.Background(), pluginsdk.CreateTaskInput{
		WorkspaceID: "ws-1", WorkflowID: "wf-1", Title: "Investigate", Description: "details",
	})
	if err != nil {
		t.Fatalf("Create() unexpected error: %v", err)
	}
	if task == nil || task.ID != "task-9" {
		t.Fatalf("Create() = %+v, want task-9", task)
	}
	if d.taskWriter.createCalls != 1 {
		t.Fatalf("task writer create calls = %d, want 1", d.taskWriter.createCalls)
	}
	if d.taskWriter.lastCreate.Source != "plugin:p1" {
		t.Fatalf("create source = %q, want plugin:p1 (server-stamped provenance)", d.taskWriter.lastCreate.Source)
	}
	if d.starter.calls != 0 {
		t.Fatalf("start_agent not requested but starter called %d times", d.starter.calls)
	}
}

func TestPluginHost_Tasks_CreateRejectsReservedSourceMetadata(t *testing.T) {
	d := newTestDataHost(manifest.Capabilities{APIWrite: []string{"tasks"}})

	_, err := d.host.Tasks().Create(context.Background(), pluginsdk.CreateTaskInput{
		WorkspaceID: "ws-1",
		WorkflowID:  "wf-1",
		Title:       "Investigate",
		Metadata:    map[string]any{taskSourceMetadataKey: "plugin:someone-else"},
	})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("Create() error = %v, want InvalidArgument for source overwrite", err)
	}
	if d.taskWriter.createCalls != 0 {
		t.Fatal("task writer called despite reserved source metadata")
	}
}

func TestPluginHost_Tasks_CreateRejectsUndeclaredRemoteProvider(t *testing.T) {
	d := newTestDataHost(manifest.Capabilities{APIWrite: []string{"tasks"}})

	_, err := d.host.Tasks().Create(context.Background(), pluginsdk.CreateTaskInput{
		WorkspaceID: "ws-1",
		WorkflowID:  "wf-1",
		Title:       "Investigate",
		Repositories: []pluginsdk.PluginTaskRepository{{
			Remote: &pluginsdk.RemoteRepositoryDescriptor{
				ProviderID:           "bitbucket",
				ProviderHost:         "https://bitbucket.example.test",
				OwnerOrProject:       "PROJ",
				ProviderRepositoryID: "99",
				Name:                 "widgets",
				CloneURL:             "https://bitbucket.example.test/scm/PROJ/widgets.git",
			},
		}},
	})
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("Create() error = %v, want PermissionDenied for undeclared provider", err)
	}
	if d.taskWriter.createCalls != 0 {
		t.Fatal("task writer called for an undeclared repository provider")
	}
}

func TestPluginHost_Tasks_CreateRejectsCredentialBearingCloneURL(t *testing.T) {
	d := newTestDataHost(manifest.Capabilities{APIWrite: []string{"tasks"}})
	d.host.repositoryProviders = []string{"bitbucket"}

	_, err := d.host.Tasks().Create(context.Background(), pluginsdk.CreateTaskInput{
		WorkspaceID: "ws-1", WorkflowID: "wf-1", Title: "Investigate",
		Repositories: []pluginsdk.PluginTaskRepository{{
			Remote: &pluginsdk.RemoteRepositoryDescriptor{
				ProviderID: "bitbucket", ProviderHost: "https://bitbucket.example.test",
				OwnerOrProject: "PROJ", ProviderRepositoryID: "99", Name: "widgets",
				CloneURL: "https://token@bitbucket.example.test/scm/PROJ/widgets.git",
			},
		}},
	})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("Create() error = %v, want InvalidArgument for credential-bearing clone URL", err)
	}
	if d.taskWriter.createCalls != 0 {
		t.Fatal("task writer called for credential-bearing clone URL")
	}
}

func TestPluginHost_Tasks_CreateRejectsCloneURLOnDifferentProviderOrigin(t *testing.T) {
	d := newTestDataHost(manifest.Capabilities{APIWrite: []string{"tasks"}})
	d.host.repositoryProviders = []string{"bitbucket"}

	_, err := d.host.Tasks().Create(context.Background(), pluginsdk.CreateTaskInput{
		WorkspaceID: "ws-1", WorkflowID: "wf-1", Title: "Investigate",
		Repositories: []pluginsdk.PluginTaskRepository{{
			Remote: &pluginsdk.RemoteRepositoryDescriptor{
				ProviderID: "bitbucket", ProviderHost: "https://bitbucket.example.test",
				OwnerOrProject: "PROJ", ProviderRepositoryID: "99", Name: "widgets",
				CloneURL: "https://attacker.example/PROJ/widgets.git",
			},
		}},
	})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("Create() error = %v, want InvalidArgument for mismatched provider origin", err)
	}
	if d.taskWriter.createCalls != 0 {
		t.Fatal("task writer called for a clone URL on another provider origin")
	}
}

func TestPluginHost_Tasks_CreateRejectsNonHTTPSManagedCloneURL(t *testing.T) {
	d := newTestDataHost(manifest.Capabilities{APIWrite: []string{"tasks"}})
	d.host.repositoryProviders = []string{"bitbucket"}

	_, err := d.host.Tasks().Create(context.Background(), pluginsdk.CreateTaskInput{
		WorkspaceID: "ws-1", WorkflowID: "wf-1", Title: "Investigate",
		Repositories: []pluginsdk.PluginTaskRepository{{
			Remote: &pluginsdk.RemoteRepositoryDescriptor{
				ProviderID: "bitbucket", ProviderHost: "https://bitbucket.example.test",
				OwnerOrProject: "PROJ", ProviderRepositoryID: "99", Name: "widgets",
				CloneURL: "ssh://git@bitbucket.example.test/PROJ/widgets.git",
			},
		}},
	})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("Create() error = %v, want InvalidArgument for non-HTTPS managed clone URL", err)
	}
	if d.taskWriter.createCalls != 0 {
		t.Fatal("task writer called for a clone URL the HTTPS credential broker cannot scope")
	}
}

func TestPluginHost_Tasks_CreateMapsOwnedRemoteDescriptorAndLaunch(t *testing.T) {
	d := newTestDataHost(manifest.Capabilities{APIWrite: []string{"tasks"}})
	d.host.repositoryProviders = []string{"bitbucket"}
	d.taskWriter.created = &taskmodels.Task{ID: "task-9", WorkspaceID: "ws-1", WorkflowID: "wf-1"}
	d.profiles.resp = &agentsettingsdto.ListAgentsResponse{Agents: []agentsettingsdto.AgentDTO{{
		ID: "agent-1", Profiles: []agentsettingsdto.AgentProfileDTO{{ID: "agent-1"}},
	}}}
	d.tasks.executorProfiles = []*taskmodels.ExecutorProfile{{ID: "executor-profile-1"}}
	base, checkout, prompt, planMode, agentProfileID, executorProfileID := "main", "feature/pr-42", "Fix failing pipeline", "replace", "agent-1", "executor-profile-1"
	pullRequest := int64(42)

	_, err := d.host.Tasks().Create(context.Background(), pluginsdk.CreateTaskInput{
		WorkspaceID: "ws-1", WorkflowID: "wf-1", Title: "Investigate PR", StartAgent: true,
		Metadata: map[string]any{"watch_id": "watch-1"},
		Repositories: []pluginsdk.PluginTaskRepository{{
			Remote: &pluginsdk.RemoteRepositoryDescriptor{
				ProviderID: "bitbucket", ProviderHost: "https://bitbucket.example.test",
				OwnerOrProject: "PROJ", ProviderRepositoryID: "99", Name: "widgets",
				CloneURL:   "https://bitbucket.example.test/bitbucket/scm/PROJ/widgets.git",
				BaseBranch: &base, HeadBranch: &checkout, PullRequestNumber: &pullRequest,
			},
		}},
		Launch: &pluginsdk.PluginTaskLaunchOptions{
			AgentProfileID: &agentProfileID, ExecutorProfileID: &executorProfileID, Prompt: &prompt, PlanMode: &planMode,
		},
	})
	if err != nil {
		t.Fatalf("Create() unexpected error: %v", err)
	}
	if len(d.taskWriter.lastCreate.Repositories) != 1 || d.taskWriter.lastCreate.Repositories[0].Remote == nil ||
		d.taskWriter.lastCreate.Repositories[0].Remote.CloneURL != "https://bitbucket.example.test/bitbucket/scm/PROJ/widgets.git" {
		t.Fatalf("remote descriptor = %+v, want exact credential-free clone URL", d.taskWriter.lastCreate.Repositories)
	}
	if d.taskWriter.lastCreate.Metadata[taskSourceMetadataKey] != "plugin:p1" {
		t.Fatalf("source = %#v, want plugin:p1", d.taskWriter.lastCreate.Metadata[taskSourceMetadataKey])
	}
	if metadata, ok := d.taskWriter.lastCreate.Metadata["plugin:p1"].(map[string]any); !ok || metadata["watch_id"] != "watch-1" {
		t.Fatalf("namespaced metadata = %#v, want watch id", d.taskWriter.lastCreate.Metadata["plugin:p1"])
	}
	if !d.taskWriter.lastCreate.PlanMode || d.starter.lastLaunch.AgentProfileID != "agent-1" ||
		d.starter.lastLaunch.ExecutorProfileID != "executor-profile-1" || d.starter.lastLaunch.Prompt != prompt || !d.starter.lastLaunch.PlanMode {
		t.Fatalf("launch mapping = %+v task plan mode=%v", d.starter.lastLaunch, d.taskWriter.lastCreate.PlanMode)
	}
}

func TestPluginHost_Tasks_CreateRejectsUnknownLaunchProfiles(t *testing.T) {
	t.Run("agent", func(t *testing.T) {
		d := newTestDataHost(manifest.Capabilities{APIWrite: []string{"tasks"}})
		d.taskWriter.created = &taskmodels.Task{ID: "task-9"}
		id := "missing-agent-profile"

		_, err := d.host.Tasks().Create(context.Background(), pluginsdk.CreateTaskInput{
			WorkspaceID: "ws-1", WorkflowID: "wf-1", Title: "Investigate", StartAgent: true,
			Launch: &pluginsdk.PluginTaskLaunchOptions{AgentProfileID: &id},
		})
		if status.Code(err) != codes.InvalidArgument {
			t.Fatalf("Create() error = %v, want InvalidArgument for an unknown agent profile", err)
		}
		if d.taskWriter.createCalls != 0 {
			t.Fatal("task writer called for unknown agent profile")
		}
	})

	t.Run("executor", func(t *testing.T) {
		d := newTestDataHost(manifest.Capabilities{APIWrite: []string{"tasks"}})
		d.taskWriter.created = &taskmodels.Task{ID: "task-9"}
		id := "missing-executor-profile"

		_, err := d.host.Tasks().Create(context.Background(), pluginsdk.CreateTaskInput{
			WorkspaceID: "ws-1", WorkflowID: "wf-1", Title: "Investigate", StartAgent: true,
			Launch: &pluginsdk.PluginTaskLaunchOptions{ExecutorProfileID: &id},
		})
		if status.Code(err) != codes.InvalidArgument {
			t.Fatalf("Create() error = %v, want InvalidArgument for an unknown executor profile", err)
		}
		if d.taskWriter.createCalls != 0 {
			t.Fatal("task writer called for unknown executor profile")
		}
	})
}

func TestPluginHost_PluginOwnedTaskTreeRejectsUserOwnedRoot(t *testing.T) {
	d := newTestDataHost(manifest.Capabilities{APIWrite: []string{"tasks"}})
	d.tasks.tasksByID = map[string]*taskmodels.Task{
		"user-task": {ID: "user-task", WorkspaceID: "ws-1", Metadata: map[string]any{taskSourceMetadataKey: "manual"}},
	}

	_, err := d.host.PluginOwnedTaskTrees().Preview(context.Background(), "user-task")
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("Preview() error = %v, want PermissionDenied for user-owned task", err)
	}
}

func TestPluginHost_PluginOwnedTaskTreeDeleteIsIdempotentAfterRootDeletion(t *testing.T) {
	d := newTestDataHost(manifest.Capabilities{APIWrite: []string{"tasks"}})
	d.tasks.tasksByID = map[string]*taskmodels.Task{}

	deleted, err := d.host.PluginOwnedTaskTrees().Delete(context.Background(), "already-deleted")

	if err != nil {
		t.Fatalf("Delete() unexpected error for an absent root: %v", err)
	}
	if len(deleted) != 0 || len(d.taskWriter.deletedIDs) != 0 {
		t.Fatalf("Delete() = %v, writer deleted=%v, want idempotent no-op", deleted, d.taskWriter.deletedIDs)
	}
}

func TestPluginHost_PluginOwnedTaskTreeRejectsDeleteWithAdoptedDescendants(t *testing.T) {
	d := newTestDataHost(manifest.Capabilities{APIWrite: []string{"tasks"}})
	root := &taskmodels.Task{ID: "root", WorkspaceID: "ws-1", Metadata: map[string]any{taskSourceMetadataKey: "plugin:p1"}}
	ownedChild := &taskmodels.Task{ID: "owned-child", WorkspaceID: "ws-1", ParentID: "root", Metadata: map[string]any{taskSourceMetadataKey: "plugin:p1"}}
	adoptedChild := &taskmodels.Task{ID: "adopted-child", WorkspaceID: "ws-1", ParentID: "root", Metadata: map[string]any{taskSourceMetadataKey: "manual"}}
	ownedGrandchild := &taskmodels.Task{ID: "owned-grandchild", WorkspaceID: "ws-1", ParentID: "adopted-child", Metadata: map[string]any{taskSourceMetadataKey: "plugin:p1"}}
	d.tasks.tasksByID = map[string]*taskmodels.Task{"root": root}
	d.tasks.tasksByWorkspace = map[string][]*taskmodels.Task{
		"ws-1": {root, ownedChild, adoptedChild, ownedGrandchild},
	}

	preview, err := d.host.PluginOwnedTaskTrees().Preview(context.Background(), "root")
	if err != nil {
		t.Fatalf("Preview() unexpected error: %v", err)
	}
	if len(preview) != 2 || preview[0].ID != "root" || preview[1].ID != "owned-child" {
		t.Fatalf("Preview() = %+v, want only contiguous plugin-owned tree", preview)
	}
	deleted, err := d.host.PluginOwnedTaskTrees().Delete(context.Background(), "root")
	if status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("Delete() error = %v, want FailedPrecondition", err)
	}
	if len(deleted) != 0 || len(d.taskWriter.deletedIDs) != 0 {
		t.Fatalf("Delete() mutated mixed-ownership tree: result=%v deleted=%v", deleted, d.taskWriter.deletedIDs)
	}
}

func TestPluginHost_Tasks_CreateStartsAgentWhenRequested(t *testing.T) {
	d := newTestDataHost(manifest.Capabilities{APIWrite: []string{"tasks"}})
	d.taskWriter.created = &taskmodels.Task{ID: "task-9", WorkspaceID: "ws-1", WorkflowID: "wf-1"}

	_, err := d.host.Tasks().Create(context.Background(), pluginsdk.CreateTaskInput{
		WorkspaceID: "ws-1", WorkflowID: "wf-1", Title: "Investigate", StartAgent: true,
	})
	if err != nil {
		t.Fatalf("Create() unexpected error: %v", err)
	}
	if d.starter.calls != 1 || d.starter.lastID != "task-9" {
		t.Fatalf("starter calls=%d lastID=%q, want 1 call for task-9", d.starter.calls, d.starter.lastID)
	}
}

// TestPluginHost_Tasks_CreateStartAgentFailureIsNonFatal proves start_agent is
// best-effort: a launch error does not fail task creation (the task is already
// created and its event has fired).
func TestPluginHost_Tasks_CreateStartAgentFailureIsNonFatal(t *testing.T) {
	d := newTestDataHost(manifest.Capabilities{APIWrite: []string{"tasks"}})
	d.taskWriter.created = &taskmodels.Task{ID: "task-9"}
	d.starter.err = errors.New("no executor available")

	task, err := d.host.Tasks().Create(context.Background(), pluginsdk.CreateTaskInput{
		WorkspaceID: "ws-1", WorkflowID: "wf-1", Title: "Investigate", StartAgent: true,
	})
	if err != nil {
		t.Fatalf("Create() should not fail on a best-effort start error: %v", err)
	}
	if task == nil || task.ID != "task-9" {
		t.Fatalf("Create() = %+v, want the created task returned despite start failure", task)
	}
}

func TestPluginHost_Tasks_CreateRequiresTitle(t *testing.T) {
	d := newTestDataHost(manifest.Capabilities{APIWrite: []string{"tasks"}})
	_, err := d.host.Tasks().Create(context.Background(), pluginsdk.CreateTaskInput{WorkspaceID: "ws-1", WorkflowID: "wf-1"})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("Create() without title error = %v, want InvalidArgument", err)
	}
	if d.taskWriter.createCalls != 0 {
		t.Fatalf("task writer called despite empty title")
	}
}

// TestPluginHost_Tasks_CreateResolvesDefaults proves an omitted workspace_id /
// workflow_id resolves to the single workspace and that workspace's first
// workflow, mirroring REST/MCP default placement.
func TestPluginHost_Tasks_CreateResolvesDefaults(t *testing.T) {
	d := newTestDataHost(manifest.Capabilities{APIWrite: []string{"tasks"}})
	d.tasks.workspaces = []*taskmodels.Workspace{{ID: "ws-only"}}
	d.workflows.workflows = map[string][]*taskmodels.Workflow{
		"ws-only": {{ID: "wf-first", WorkspaceID: "ws-only"}, {ID: "wf-second", WorkspaceID: "ws-only"}},
	}

	_, err := d.host.Tasks().Create(context.Background(), pluginsdk.CreateTaskInput{Title: "Investigate"})
	if err != nil {
		t.Fatalf("Create() unexpected error: %v", err)
	}
	if d.taskWriter.lastCreate.WorkspaceID != "ws-only" || d.taskWriter.lastCreate.WorkflowID != "wf-first" {
		t.Fatalf("resolved placement = ws=%q wf=%q, want ws-only/wf-first", d.taskWriter.lastCreate.WorkspaceID, d.taskWriter.lastCreate.WorkflowID)
	}
}

func TestPluginHost_Tasks_CreateAmbiguousWorkspaceIsInvalidArgument(t *testing.T) {
	d := newTestDataHost(manifest.Capabilities{APIWrite: []string{"tasks"}})
	d.tasks.workspaces = []*taskmodels.Workspace{{ID: "ws-1"}, {ID: "ws-2"}}

	_, err := d.host.Tasks().Create(context.Background(), pluginsdk.CreateTaskInput{Title: "Investigate"})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("Create() with ambiguous workspace error = %v, want InvalidArgument", err)
	}
	if d.taskWriter.createCalls != 0 {
		t.Fatalf("task writer called despite unresolvable workspace default")
	}
}

// ── Task update ─────────────────────────────────────────────────────────

func TestPluginHost_Tasks_UpdateSucceedsWithMask(t *testing.T) {
	d := newTestDataHost(manifest.Capabilities{APIWrite: []string{"tasks"}})
	d.taskWriter.updated = &taskmodels.Task{ID: "task-1", Title: "Renamed"}

	title := "Renamed"
	state := "IN_PROGRESS"
	_, err := d.host.Tasks().Update(context.Background(), pluginsdk.UpdateTaskInput{ID: "task-1", Title: &title, State: &state})
	if err != nil {
		t.Fatalf("Update() unexpected error: %v", err)
	}
	if d.taskWriter.lastUpdate.ID != "task-1" {
		t.Fatalf("update id = %q, want task-1", d.taskWriter.lastUpdate.ID)
	}
	if d.taskWriter.lastUpdate.Title == nil || *d.taskWriter.lastUpdate.Title != "Renamed" {
		t.Fatalf("update title = %v, want Renamed", d.taskWriter.lastUpdate.Title)
	}
	if d.taskWriter.lastUpdate.Description != nil {
		t.Fatalf("unset description must stay nil, got %v", d.taskWriter.lastUpdate.Description)
	}
}

func TestPluginHost_Tasks_UpdateRequiresID(t *testing.T) {
	d := newTestDataHost(manifest.Capabilities{APIWrite: []string{"tasks"}})
	_, err := d.host.Tasks().Update(context.Background(), pluginsdk.UpdateTaskInput{})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("Update() without id error = %v, want InvalidArgument", err)
	}
}

func TestPluginHost_Tasks_UpdateNotFoundMapsToGRPCNotFound(t *testing.T) {
	d := newTestDataHost(manifest.Capabilities{APIWrite: []string{"tasks"}})
	d.taskWriter.updateErr = repoerrors.ErrTaskNotFound

	_, err := d.host.Tasks().Update(context.Background(), pluginsdk.UpdateTaskInput{ID: "no-such-task"})
	if status.Code(err) != codes.NotFound {
		t.Fatalf("Update() of missing task error = %v, want NotFound", err)
	}
}

// ── Message send ────────────────────────────────────────────────────────

func TestPluginHost_Messages_SendSucceedsAndStampsSource(t *testing.T) {
	d := newTestDataHost(manifest.Capabilities{APIWrite: []string{"messages"}})
	d.messenger.result = PluginMessageResult{SessionID: "session-9", Status: "queued"}

	dispatch, err := d.host.Messages().Send(context.Background(), "task-1", "session-9", "please rerun the tests")
	if err != nil {
		t.Fatalf("Send() unexpected error: %v", err)
	}
	if dispatch == nil || dispatch.SessionID != "session-9" || dispatch.Status != "queued" {
		t.Fatalf("Send() = %+v, want session-9/queued", dispatch)
	}
	if d.messenger.calls != 1 {
		t.Fatalf("messenger calls = %d, want 1 (routes through the orchestrator delivery path)", d.messenger.calls)
	}
	if d.messenger.lastTaskID != "task-1" || d.messenger.lastSession != "session-9" || d.messenger.lastText != "please rerun the tests" {
		t.Fatalf("messenger recorded taskID=%q session=%q text=%q", d.messenger.lastTaskID, d.messenger.lastSession, d.messenger.lastText)
	}
	if d.messenger.lastSource != "plugin:p1" {
		t.Fatalf("message source = %q, want plugin:p1 (server-stamped provenance)", d.messenger.lastSource)
	}
}

// TestPluginHost_Messages_ReadAndWriteGateIndependently proves List
// (api_read:messages) and Send (api_write:messages) are enforced separately on
// the shared Messages() accessor: a write-only manifest can Send but not List.
func TestPluginHost_Messages_ReadAndWriteGateIndependently(t *testing.T) {
	d := newTestDataHost(manifest.Capabilities{APIWrite: []string{"messages"}})

	_, _, err := d.host.Messages().List(context.Background(), pluginsdk.MessageFilter{}, pluginsdk.Page{})
	assertPermissionDenied(t, err, "api_read:messages")

	if _, err := d.host.Messages().Send(context.Background(), "task-1", "", "hi"); err != nil {
		t.Fatalf("Send() with api_write:messages unexpected error: %v", err)
	}
}

func TestPluginHost_Messages_SendRequiresTaskAndText(t *testing.T) {
	d := newTestDataHost(manifest.Capabilities{APIWrite: []string{"messages"}})

	if _, err := d.host.Messages().Send(context.Background(), "", "", "body"); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("Send() without task_id error = %v, want InvalidArgument", err)
	}
	if _, err := d.host.Messages().Send(context.Background(), "task-1", "", ""); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("Send() without text error = %v, want InvalidArgument", err)
	}
	if d.messenger.calls != 0 {
		t.Fatalf("messenger called %d times despite invalid input", d.messenger.calls)
	}
}

// The host launches a plugin task right after CreateTask returns, so the start
// intent has to reach the create itself. Without it the task service resolves
// the parking start step and the plugin's agent runs in a column configured to
// run nothing — the bug this PR fixes for REST, WS and MCP, reached by a fourth
// route.
func TestPluginHost_Tasks_CreatePropagatesStartAgentToTheWriter(t *testing.T) {
	for name, startAgent := range map[string]bool{
		"starting an agent": true,
		"create only":       false,
	} {
		t.Run(name, func(t *testing.T) {
			d := newTestDataHost(manifest.Capabilities{APIWrite: []string{"tasks"}})
			d.taskWriter.created = &taskmodels.Task{ID: "task-9", WorkspaceID: "ws-1", WorkflowID: "wf-1"}
			d.profiles.resp = &agentsettingsdto.ListAgentsResponse{Agents: []agentsettingsdto.AgentDTO{{
				ID: "agent-1", Profiles: []agentsettingsdto.AgentProfileDTO{{ID: "agent-1"}},
			}}}
			d.tasks.executorProfiles = []*taskmodels.ExecutorProfile{{ID: "executor-profile-1"}}
			agentProfileID, executorProfileID := "agent-1", "executor-profile-1"

			_, err := d.host.Tasks().Create(context.Background(), pluginsdk.CreateTaskInput{
				WorkspaceID: "ws-1", WorkflowID: "wf-1", Title: "Investigate", StartAgent: startAgent,
				Launch: &pluginsdk.PluginTaskLaunchOptions{
					AgentProfileID: &agentProfileID, ExecutorProfileID: &executorProfileID,
				},
			})
			if err != nil {
				t.Fatalf("Create() unexpected error: %v", err)
			}
			if d.taskWriter.lastCreate.StartAgent != startAgent {
				t.Fatalf("create StartAgent = %v, want %v", d.taskWriter.lastCreate.StartAgent, startAgent)
			}
		})
	}
}
