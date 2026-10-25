package lifecycle

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
	"go.uber.org/zap"
)

func TestExactProfileLaunchRejectsChangedResolvedProfile(t *testing.T) {
	profile := &AgentProfileInfo{ProfileID: "profile-1", WorkspaceID: "workspace-1", Enabled: true,
		Revision: 2, AgentName: "codex"}
	log, err := logger.NewFromZap(zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	manager := &Manager{profileResolver: &stubProfileResolver{profile: profile}, logger: log}
	req := &LaunchRequest{AgentProfileID: "profile-1", WorkspaceID: "workspace-1",
		ExactProfile: true, ExactProfileRevision: 2}
	if _, _, err := manager.resolveAgentProfile(context.Background(), req); err != nil {
		t.Fatalf("current profile rejected: %v", err)
	}
	for _, mutate := range []func(){
		func() { profile.Revision = 3 },
		func() { profile.Revision = 2; profile.Enabled = false },
		func() { profile.Enabled = true; profile.WorkspaceID = "workspace-2" },
	} {
		mutate()
		if _, _, err := manager.resolveAgentProfile(context.Background(), req); err == nil {
			t.Fatal("changed profile admitted to exact launch")
		}
	}
}

func TestExecutorInstanceCarriesExactProfileSnapshot(t *testing.T) {
	execution := (&ExecutorInstance{InstanceID: "execution-1"}).ToAgentExecution(&ExecutorCreateRequest{
		TaskID:               "task-1",
		SessionID:            "session-1",
		ExactProfile:         true,
		ExactProfileModel:    "gpt-5.6-codex",
		ExactProfileRevision: 1726500000000000000,
	})

	if !execution.ExactProfile || execution.ExactProfileModel != "gpt-5.6-codex" || execution.ExactProfileRevision != 1726500000000000000 {
		t.Fatalf("exact profile snapshot = %+v, want exact model and revision", execution)
	}
}
