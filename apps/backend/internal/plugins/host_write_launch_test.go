package plugins

import (
	"context"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	agentsettingsdto "github.com/kandev/kandev/internal/agent/settings/dto"
	"github.com/kandev/kandev/internal/plugins/manifest"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/kdlbs/kandev/pluginsdk"
)

func TestPluginHost_Tasks_CreateOnlyRejectsUnknownLaunchProfiles(t *testing.T) {
	t.Run("agent", func(t *testing.T) {
		d := newTestDataHost(manifest.Capabilities{APIWrite: []string{"tasks"}})
		d.taskWriter.created = &taskmodels.Task{ID: "task-9"}
		id := "missing-agent-profile"

		_, err := d.host.Tasks().Create(context.Background(), pluginsdk.CreateTaskInput{
			WorkspaceID: "ws-1", WorkflowID: "wf-1", Title: "Investigate",
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
			WorkspaceID: "ws-1", WorkflowID: "wf-1", Title: "Investigate",
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

func TestPluginHost_Tasks_CreateOnlyHandsLaunchProfilesToTheWriter(t *testing.T) {
	d := newTestDataHost(manifest.Capabilities{APIWrite: []string{"tasks"}})
	d.taskWriter.created = &taskmodels.Task{ID: "task-9", WorkspaceID: "ws-1", WorkflowID: "wf-1"}
	d.profiles.resp = &agentsettingsdto.ListAgentsResponse{Agents: []agentsettingsdto.AgentDTO{{
		ID: "agent-1", Profiles: []agentsettingsdto.AgentProfileDTO{{ID: "agent-1"}},
	}}}
	d.tasks.executorProfiles = []*taskmodels.ExecutorProfile{{ID: "executor-profile-1"}}
	agentProfileID, executorProfileID := "agent-1", "executor-profile-1"

	_, err := d.host.Tasks().Create(context.Background(), pluginsdk.CreateTaskInput{
		WorkspaceID: "ws-1", WorkflowID: "wf-1", Title: "Investigate",
		Launch: &pluginsdk.PluginTaskLaunchOptions{
			AgentProfileID: &agentProfileID, ExecutorProfileID: &executorProfileID,
		},
	})
	if err != nil {
		t.Fatalf("Create() unexpected error: %v", err)
	}
	got := d.taskWriter.lastCreate.Launch
	if got.AgentProfileID != agentProfileID || got.ExecutorProfileID != executorProfileID {
		t.Fatalf("writer launch = %+v, want both profiles", got)
	}
	if d.starter.calls != 0 {
		t.Fatal("create-only task started an agent")
	}
}
