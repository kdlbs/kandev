package azuredevops

import (
	"context"
	"errors"
	"testing"
)

type fakeBoardReader struct {
	teams    []Team
	boards   []BoardReference
	snapshot *BoardSnapshot
}

func TestServiceBoardReadsAuthorizeWorkspaceBeforeCreatingClient(t *testing.T) {
	clientCalls := 0
	service, _, _ := newTestService(t, func(*Config, string) Client {
		clientCalls++
		return &invalidClient{}
	})
	if _, err := service.SetConfigForWorkspace(t.Context(), "ws-1", &SetConfigRequest{OrganizationURL: "https://dev.azure.com/acme", PAT: "pat"}); err != nil {
		t.Fatalf("set config: %v", err)
	}
	clientCalls = 0
	denied := errors.New("workspace denied")
	service.SetWorkspaceAuthorizer(func(context.Context, string) error { return denied })

	if _, err := service.ListTeamsForWorkspace(t.Context(), "ws-1", "project-1"); !errors.Is(err, denied) {
		t.Fatalf("ListTeamsForWorkspace error = %v, want authorization denial", err)
	}
	if clientCalls != 0 {
		t.Fatalf("client factory calls = %d, want 0", clientCalls)
	}
}

func TestServiceBoardCapabilitiesReportNotConfigured(t *testing.T) {
	service, _, _ := newTestService(t, func(*Config, string) Client { return &invalidClient{} })
	if _, err := service.SetConfigForWorkspace(t.Context(), "ws-1", &SetConfigRequest{OrganizationURL: "https://dev.azure.com/acme", PAT: "pat"}); err != nil {
		t.Fatalf("set config: %v", err)
	}
	if _, err := service.ListTeamsForWorkspace(t.Context(), "ws-1", "project-1"); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("ListTeamsForWorkspace error = %v, want ErrNotConfigured", err)
	}
	column := "column-1"
	if _, err := service.UpdateBoardWorkItemForWorkspace(t.Context(), "ws-1", "project-1", "team-1", "board-1", 101, BoardWorkItemUpdateRequest{Revision: 7, ColumnID: &column}); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("UpdateBoardWorkItemForWorkspace error = %v, want ErrNotConfigured", err)
	}
}

type fakeBoardWriter struct {
	*fakeBoardReader
	request BoardWorkItemUpdateRequest
}

func (f *fakeBoardWriter) GetCurrentIdentity(context.Context) (*Identity, error) {
	return &Identity{ID: "me", DisplayName: "Ada"}, nil
}

func (f *fakeBoardWriter) UpdateBoardWorkItem(_ context.Context, _, _, _ string, _ int, request BoardWorkItemUpdateRequest) (*BoardWorkItem, error) {
	f.request = request
	return &BoardWorkItem{WorkItem: WorkItem{ID: 101, Revision: request.Revision + 1, Title: "Updated"}}, nil
}

func (f *fakeBoardReader) ListTeams(context.Context, string) ([]Team, error) {
	return f.teams, nil
}
func (f *fakeBoardReader) ListBoards(context.Context, string, string) ([]BoardReference, error) {
	return f.boards, nil
}
func (f *fakeBoardReader) GetBoardSnapshot(context.Context, string, string, string) (*BoardSnapshot, error) {
	return f.snapshot, nil
}

func TestServiceBoardReads(t *testing.T) {
	reader := &fakeBoardReader{
		teams:    []Team{{ID: "team-1", Name: "Platform Team", ProjectID: "project-1"}},
		boards:   []BoardReference{{ID: "board-1", Name: "Stories"}},
		snapshot: &BoardSnapshot{Board: Board{ID: "board-1", Name: "Stories"}, Items: []BoardWorkItem{{WorkItem: WorkItem{ID: 101, Title: "Fix build"}}}},
	}
	service, _, _ := newTestService(t, func(*Config, string) Client {
		return &struct {
			invalidClient
			*fakeBoardReader
		}{fakeBoardReader: reader}
	})
	if _, err := service.SetConfigForWorkspace(context.Background(), "ws-1", &SetConfigRequest{OrganizationURL: "https://dev.azure.com/acme", PAT: "pat"}); err != nil {
		t.Fatalf("set config: %v", err)
	}
	teams, err := service.ListTeamsForWorkspace(context.Background(), "ws-1", "project-1")
	if err != nil || len(teams) != 1 || teams[0].ID != "team-1" {
		t.Fatalf("ListTeamsForWorkspace = %+v, %v", teams, err)
	}
	boards, err := service.ListBoardsForWorkspace(context.Background(), "ws-1", "project-1", "team-1")
	if err != nil || len(boards) != 1 || boards[0].ID != "board-1" {
		t.Fatalf("ListBoardsForWorkspace = %+v, %v", boards, err)
	}
	snapshot, err := service.GetBoardSnapshotForWorkspace(context.Background(), "ws-1", "project-1", "team-1", "board-1")
	if err != nil || snapshot == nil || len(snapshot.Items) != 1 {
		t.Fatalf("GetBoardSnapshotForWorkspace = %+v, %v", snapshot, err)
	}
}

func TestServiceUpdateBoardWorkItemIdentity(t *testing.T) {
	reader := &fakeBoardReader{snapshot: &BoardSnapshot{Board: Board{ID: "board-1"}}}
	writer := &fakeBoardWriter{fakeBoardReader: reader}
	service, _, _ := newTestService(t, func(*Config, string) Client {
		return &struct {
			invalidClient
			*fakeBoardWriter
		}{fakeBoardWriter: writer}
	})
	if _, err := service.SetConfigForWorkspace(t.Context(), "ws-1", &SetConfigRequest{OrganizationURL: "https://dev.azure.com/acme", PAT: "pat"}); err != nil {
		t.Fatalf("set config: %v", err)
	}
	assign := "assign_current_user"
	item, err := service.UpdateBoardWorkItemForWorkspace(t.Context(), "ws-1", "project-1", "team-1", "board-1", 101, BoardWorkItemUpdateRequest{Revision: 7, AssigneeAction: &assign})
	if err != nil || item == nil || writer.request.Revision != 7 || writer.request.resolvedAssignee != "Ada" {
		t.Fatalf("updated item = %+v, request=%+v, err=%v", item, writer.request, err)
	}
}

func TestServiceUpdateBoardWorkItemRejectsUnsupportedFields(t *testing.T) {
	if err := validateBoardWorkItemUpdate(BoardWorkItemUpdateRequest{Revision: 7}); err == nil {
		t.Fatal("validateBoardWorkItemUpdate accepted an empty mutation")
	}
}
