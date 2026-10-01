package backendapp

import (
	"context"
	"errors"
	"testing"

	agenttypes "github.com/kandev/kandev/internal/agent/agents"
	settingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
)

type projectWorkspaceAgentStoreStub struct {
	agent *settingsmodels.Agent
	err   error
	gotID string
}

func (s *projectWorkspaceAgentStoreStub) GetAgent(_ context.Context, id string) (*settingsmodels.Agent, error) {
	s.gotID = id
	return s.agent, s.err
}

type projectWorkspaceAgentRegistryStub struct {
	agent agenttypes.Agent
	gotID string
}

func (r *projectWorkspaceAgentRegistryStub) Get(id string) (agenttypes.Agent, bool) {
	r.gotID = id
	return r.agent, r.agent != nil
}

func TestProjectWorkspaceAgentCapabilityResolvesDatabaseAgentIDToRegistryName(t *testing.T) {
	store := &projectWorkspaceAgentStoreStub{agent: &settingsmodels.Agent{
		ID:   "agent-database-id",
		Name: "mock-agent",
	}}
	registry := &projectWorkspaceAgentRegistryStub{agent: agenttypes.NewMockAgent()}

	if !projectWorkspaceAgentSupported(context.Background(), "agent-database-id", store, registry) {
		t.Fatal("expected ACP project workspace capability to be supported")
	}
	if store.gotID != "agent-database-id" {
		t.Fatalf("profile agent lookup ID = %q, want database ID", store.gotID)
	}
	if registry.gotID != "mock-agent" {
		t.Fatalf("registry lookup ID = %q, want agent name", registry.gotID)
	}
}

func TestProjectWorkspaceAgentCapabilityFailsClosedWhenAgentCannotBeResolved(t *testing.T) {
	registry := &projectWorkspaceAgentRegistryStub{agent: agenttypes.NewMockAgent()}
	for _, store := range []*projectWorkspaceAgentStoreStub{
		{err: errors.New("missing agent")},
		{agent: nil},
	} {
		if projectWorkspaceAgentSupported(context.Background(), "agent-database-id", store, registry) {
			t.Fatal("unresolved database agent ID was accepted")
		}
	}
}
