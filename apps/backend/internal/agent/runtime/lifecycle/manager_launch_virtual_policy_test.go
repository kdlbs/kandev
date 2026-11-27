package lifecycle

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/agent/agents"
	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
)

func TestManagedToolPolicyAdmissionPreservesVirtualErrorIdentity(t *testing.T) {
	m := newTestManager(t)
	m.profileResolver = nil
	policy := mcpprofile.ManagedToolPolicy{
		PluginID: "coordinator.v1", InstallationID: "installation-1", WorkspaceID: "workspace-1",
		InstanceKey: "lead", ConversationRevision: 3, ApprovalRevision: 2,
		ManifestDigest: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		AgentToolNames: []string{"read_task"},
	}
	profile := mcpprofile.New(mcpprofile.SurfaceManagedConversation, nil, nil).WithManagedToolPolicy(policy)
	req := &LaunchRequest{AgentProfileID: agents.DynamicAgentID, McpProfile: &profile}
	err := m.validateManagedToolPolicyAdmission(context.Background(), req)
	if !errors.Is(err, ErrVirtualProfile) || !errors.Is(err, ErrManagedToolPolicyUnsupported) {
		t.Fatalf("admission error = %v, want virtual and managed-policy identities", err)
	}
	req.AgentProfileID = "missing-agent"
	err = m.validateManagedToolPolicyAdmission(context.Background(), req)
	if errors.Is(err, ErrVirtualProfile) || !errors.Is(err, ErrManagedToolPolicyUnsupported) {
		t.Fatalf("missing agent admission error = %v, want only managed-policy identity", err)
	}
	req.McpProfile = nil
	if err := m.validateManagedToolPolicyAdmission(context.Background(), req); err != nil {
		t.Fatalf("ordinary launch policy admission = %v", err)
	}
}
