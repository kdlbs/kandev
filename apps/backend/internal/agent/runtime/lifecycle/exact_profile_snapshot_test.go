package lifecycle

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
	"go.uber.org/zap"
)

func TestExactProfileLaunchRejectsChangedResolvedProfile(t *testing.T) {
	profile := &AgentProfileInfo{ProfileID: "profile-1", WorkspaceID: "workspace-1", Enabled: true,
		Revision: 2, AgentName: "codex", Model: "model-1"}
	log, err := logger.NewFromZap(zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	manager := &Manager{profileResolver: &stubProfileResolver{profile: profile}, logger: log}
	req := &LaunchRequest{AgentProfileID: "profile-1", WorkspaceID: "workspace-1",
		ExactProfile: true, ExactProfileRevision: 2, ExactProfileModel: "model-1"}
	if _, _, err := manager.resolveAgentProfile(context.Background(), req); err != nil {
		t.Fatalf("current profile rejected: %v", err)
	}
	for _, mutate := range []func(){
		func() { profile.Revision = 3 },
		func() { profile.Revision = 2; profile.Enabled = false },
		func() { profile.Enabled = true; profile.WorkspaceID = "workspace-2" },
		func() { profile.WorkspaceID = "workspace-1"; profile.Model = "model-2" },
	} {
		mutate()
		if _, _, err := manager.resolveAgentProfile(context.Background(), req); err == nil {
			t.Fatal("changed profile admitted to exact launch")
		}
	}
}

func TestPreparedExactExecutionRequiresMatchingLiveSnapshot(t *testing.T) {
	profile := &AgentProfileInfo{ProfileID: "profile-1", WorkspaceID: "workspace-1", Enabled: true,
		Revision: 2, AgentName: "codex", Model: "model-1"}
	manager := newTestManager(t)
	manager.profileResolver = &stubProfileResolver{profile: profile}
	execution := &AgentExecution{ID: "execution-1", SessionID: "session-1", WorkspaceID: "workspace-1",
		AgentProfileID: "profile-1", ExactProfile: true, ExactProfileRevision: 2, ExactProfileModel: "model-1"}
	if err := manager.executionStore.Add(execution); err != nil {
		t.Fatal(err)
	}
	admit := func() error {
		return manager.AdmitExactProfilePreparedExecution(context.Background(),
			"execution-1", "workspace-1", "profile-1", 2, "model-1")
	}
	if err := admit(); err != nil {
		t.Fatalf("matching snapshot rejected: %v", err)
	}
	profile.Model = "model-2"
	if err := admit(); err == nil {
		t.Fatal("changed live model admitted")
	}
	profile.Model = "model-1"
	execution.ExactProfile = false
	if err := admit(); err == nil {
		t.Fatal("non-exact prepared execution admitted")
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
