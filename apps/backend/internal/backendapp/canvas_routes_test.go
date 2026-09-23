package backendapp

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

type canvasWorkspaceOwnerAccessStub struct {
	workspace     *models.Workspace
	accessErr     error
	workspaceErr  error
	workspaceRead bool
}

func (s *canvasWorkspaceOwnerAccessStub) AuthorizeWorkspaceAccess(context.Context, string) error {
	return s.accessErr
}

func (s *canvasWorkspaceOwnerAccessStub) GetWorkspace(context.Context, string) (*models.Workspace, error) {
	s.workspaceRead = true
	return s.workspace, s.workspaceErr
}

func TestCheckCanvasWorkspaceOwnerRequiresCurrentWorkspaceOwner(t *testing.T) {
	tests := []struct {
		name      string
		userID    string
		workspace *models.Workspace
		wantOwner bool
	}{
		{name: "owner", userID: "owner-1", workspace: &models.Workspace{ID: "workspace-1", OwnerID: "owner-1"}, wantOwner: true},
		{name: "workspace member", userID: "member-1", workspace: &models.Workspace{ID: "workspace-1", OwnerID: "owner-1"}},
		{name: "empty user", workspace: &models.Workspace{ID: "workspace-1", OwnerID: "owner-1"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			access := &canvasWorkspaceOwnerAccessStub{workspace: tt.workspace}
			owner, err := checkCanvasWorkspaceOwner(context.Background(), access, "workspace-1", tt.userID)
			if err != nil {
				t.Fatalf("checkCanvasWorkspaceOwner() error = %v", err)
			}
			if owner != tt.wantOwner {
				t.Fatalf("owner = %t, want %t", owner, tt.wantOwner)
			}
			if !access.workspaceRead {
				t.Fatal("workspace owner was not checked")
			}
		})
	}
}

func TestCheckCanvasWorkspaceOwnerDoesNotReadWorkspaceAfterAccessDenial(t *testing.T) {
	access := &canvasWorkspaceOwnerAccessStub{accessErr: errors.New("workspace access denied")}
	owner, err := checkCanvasWorkspaceOwner(context.Background(), access, "workspace-1", "owner-1")
	if err == nil {
		t.Fatal("checkCanvasWorkspaceOwner() error = nil, want access denial")
	}
	if owner {
		t.Fatal("owner = true after workspace access denial")
	}
	if access.workspaceRead {
		t.Fatal("workspace details were read after access denial")
	}
}
