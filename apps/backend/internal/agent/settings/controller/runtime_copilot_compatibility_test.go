package controller

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/settings/models"
	"github.com/kandev/kandev/internal/agent/settings/store"
)

func newCopilotCompatibilityController(t *testing.T) (*Controller, *fakeStore) {
	t.Helper()
	st := newFakeStore()
	agent := &models.Agent{ID: "copilot-row", Name: agents.CopilotACPAgentID}
	st.agents[agent.ID] = agent
	st.byName[agent.Name] = agent
	st.profiles[agent.ID] = []*models.AgentProfile{
		{ID: "selected-enabled", AgentID: agent.ID, Enabled: true, CopilotSelectedPlugins: []string{"installed:one"}},
		{ID: "selected-disabled", AgentID: agent.ID, Enabled: false, CopilotSelectedPlugins: []string{"installed:two"}},
		{ID: "empty", AgentID: agent.ID, Enabled: true},
		{ID: "deleted", AgentID: agent.ID, DeletedAt: ptrTime(time.Now()), CopilotSelectedPlugins: []string{"installed:three"}},
	}
	foreign := &models.Agent{ID: "claude-row", Name: "claude-acp"}
	st.agents[foreign.ID] = foreign
	st.byName[foreign.Name] = foreign
	st.profiles[foreign.ID] = []*models.AgentProfile{
		{ID: "foreign", AgentID: foreign.ID, CopilotSelectedPlugins: []string{"installed:foreign"}},
	}
	ctrl := newTestController(map[string]agents.Agent{agent.Name: agents.NewCopilotACP()})
	ctrl.repo = st
	ctrl.SetRuntimeUpdater(&recoveryRuntimeUpdater{
		metadata: RuntimeVersionMetadata{
			Versions: []string{"1.0.88", "1.0.104", "1.0.105", "9.9.9"},
			Latest:   "9.9.9",
		},
	})
	return ctrl, st
}

func ptrTime(value time.Time) *time.Time { return &value }

func TestCopilotRuntimeCompatibilityPreview(t *testing.T) {
	ctrl, st := newCopilotCompatibilityController(t)

	preview, err := ctrl.PreviewAgentUpdate(context.Background(), agents.CopilotACPAgentID, "1.0.105")
	if err != nil {
		t.Fatalf("PreviewAgentUpdate: %v", err)
	}
	if len(st.created) != 0 || len(st.updated) != 0 || len(st.softDeleted) != 0 {
		t.Fatalf("preview wrote profiles: created=%d updated=%d deleted=%d",
			len(st.created), len(st.updated), len(st.softDeleted))
	}
	compatibility := preview.CopilotPluginCompatibility
	if compatibility == nil {
		t.Fatal("supported Copilot preview omitted plugin compatibility")
	}
	if compatibility.State != "not_checked" || compatibility.RuntimeVersion != "1.0.105" {
		t.Fatalf("compatibility = %#v", compatibility)
	}
	if compatibility.Reason != "" {
		t.Fatalf("unchecked target reason = %q, want empty", compatibility.Reason)
	}
	if compatibility.SelectedProfileCount == nil || *compatibility.SelectedProfileCount != 2 {
		t.Fatalf("selected profile count = %v, want 2", compatibility.SelectedProfileCount)
	}
}

func TestCopilotRuntimeCompatibilityPreviewKeepsUnknownTargetUnchecked(t *testing.T) {
	ctrl, _ := newCopilotCompatibilityController(t)

	preview, err := ctrl.PreviewAgentUpdate(context.Background(), agents.CopilotACPAgentID, "9.9.9")
	if err != nil {
		t.Fatalf("PreviewAgentUpdate: %v", err)
	}
	compatibility := preview.CopilotPluginCompatibility
	if compatibility == nil {
		t.Fatal("unqualified Copilot preview omitted plugin compatibility")
	}
	if compatibility.State != "not_checked" || compatibility.RuntimeVersion != "9.9.9" {
		t.Fatalf("compatibility = %#v", compatibility)
	}
	if compatibility.Reason != "" {
		t.Fatalf("reason = %q, want none without concrete evidence", compatibility.Reason)
	}
	if compatibility.SelectedProfileCount == nil || *compatibility.SelectedProfileCount != 2 {
		t.Fatalf("selected profile count = %v, want 2", compatibility.SelectedProfileCount)
	}
}

func TestCopilotRuntimeCompatibilityPreviewKeepsUnknownImpact(t *testing.T) {
	ctrl, st := newCopilotCompatibilityController(t)
	st.listProfErr = map[string]error{"copilot-row": errors.New("profile read failed")}

	preview, err := ctrl.PreviewAgentUpdate(context.Background(), agents.CopilotACPAgentID, "9.9.9")
	if err != nil {
		t.Fatalf("PreviewAgentUpdate: %v", err)
	}
	compatibility := preview.CopilotPluginCompatibility
	if compatibility == nil {
		t.Fatal("preview omitted plugin compatibility")
	}
	if compatibility.SelectedProfileCount != nil {
		t.Fatalf("selected profile count = %d, want unknown", *compatibility.SelectedProfileCount)
	}
}

func TestCopilotRuntimeCompatibilityPreviewOmitsForeignProvider(t *testing.T) {
	ag := &managedTestAgent{
		testAgent: testAgent{id: "managed-acp", name: "Managed", enabled: true},
		spec:      managedRuntimeSpec(),
	}
	ctrl := newTestController(map[string]agents.Agent{ag.ID(): ag})
	ctrl.SetRuntimeUpdater(&recoveryRuntimeUpdater{
		metadata: RuntimeVersionMetadata{Versions: []string{"1.1.0"}, Latest: "1.1.0"},
	})

	preview, err := ctrl.PreviewAgentUpdate(context.Background(), ag.ID(), "1.1.0")
	if err != nil {
		t.Fatalf("PreviewAgentUpdate: %v", err)
	}
	if preview.CopilotPluginCompatibility != nil {
		t.Fatalf("foreign provider compatibility = %#v, want omitted", preview.CopilotPluginCompatibility)
	}
}

func TestCopilotRuntimeCompatibilityPreviewHonorsCancellation(t *testing.T) {
	ctrl, st := newCopilotCompatibilityController(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	ctrl.repo = cancelAwareRepository{Repository: st}

	_, err := ctrl.PreviewAgentUpdate(ctx, agents.CopilotACPAgentID, "1.0.104")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("PreviewAgentUpdate error = %v, want context.Canceled", err)
	}
}

type cancelAwareRepository struct {
	store.Repository
}

func (r cancelAwareRepository) GetAgentByName(ctx context.Context, name string) (*models.Agent, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return r.Repository.GetAgentByName(ctx, name)
}
