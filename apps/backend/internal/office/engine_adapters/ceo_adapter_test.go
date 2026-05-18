package engine_adapters

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/repository/sqlite"
)

type fakeOfficeRepo struct {
	fields *sqlite.TaskExecutionFields
	agents []*models.AgentInstance

	fieldsErr error
	listErr   error

	gotWorkspace string
	gotFilter    sqlite.AgentListFilter
}

func (f *fakeOfficeRepo) GetTaskExecutionFields(_ context.Context, _ string) (*sqlite.TaskExecutionFields, error) {
	return f.fields, f.fieldsErr
}

func (f *fakeOfficeRepo) ListAgentInstancesFiltered(
	_ context.Context, workspaceID string, filter sqlite.AgentListFilter,
) ([]*models.AgentInstance, error) {
	f.gotWorkspace = workspaceID
	f.gotFilter = filter
	return f.agents, f.listErr
}

func TestCEOAgentAdapter_ResolvesAgentProfileID(t *testing.T) {
	// Wave G: AgentInstance.ID is the agent_profiles row id, so the
	// resolver returns it directly. There is no separate AgentProfileID.
	repo := &fakeOfficeRepo{
		fields: &sqlite.TaskExecutionFields{ID: "t-1", WorkspaceID: "ws-1"},
		agents: []*models.AgentInstance{
			{ID: "ai-ceo", Role: models.AgentRoleCEO},
		},
	}
	a := NewCEOAgentAdapter(repo)
	got, err := a.ResolveCEOAgentProfileID(context.Background(), "t-1")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if got != "ai-ceo" {
		t.Errorf("profile id = %q, want ai-ceo", got)
	}
	if repo.gotFilter.Role != "ceo" {
		t.Errorf("filter role = %q, want ceo", repo.gotFilter.Role)
	}
	if repo.gotWorkspace != "ws-1" {
		t.Errorf("workspace = %q, want ws-1", repo.gotWorkspace)
	}
}

func TestCEOAgentAdapter_NoCEOReturnsEmpty(t *testing.T) {
	repo := &fakeOfficeRepo{
		fields: &sqlite.TaskExecutionFields{ID: "t-1", WorkspaceID: "ws-1"},
	}
	a := NewCEOAgentAdapter(repo)
	got, err := a.ResolveCEOAgentProfileID(context.Background(), "t-1")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if got != "" {
		t.Errorf("got = %q, want empty when no CEO configured", got)
	}
}

func TestCEOAgentAdapter_RejectsEmptyTaskID(t *testing.T) {
	a := NewCEOAgentAdapter(&fakeOfficeRepo{})
	if _, err := a.ResolveCEOAgentProfileID(context.Background(), ""); err == nil {
		t.Fatal("expected error for empty task id")
	}
}

func TestCEOAgentAdapter_PropagatesFieldsError(t *testing.T) {
	repo := &fakeOfficeRepo{fieldsErr: errors.New("missing task")}
	a := NewCEOAgentAdapter(repo)
	if _, err := a.ResolveCEOAgentProfileID(context.Background(), "t-1"); err == nil {
		t.Fatal("expected error")
	}
}
