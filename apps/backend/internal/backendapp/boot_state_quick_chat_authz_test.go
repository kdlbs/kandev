package backendapp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/webapp"
)

// seedOwnedQuickChat gives userID a workspace holding one restorable quick chat
// whose title is the thing that must not cross an ownership boundary.
func seedOwnedQuickChat(t *testing.T, harness bootStateTestHarness, workspaceID, userID, title string) {
	t.Helper()
	ctx := context.Background()
	if err := harness.taskRepo.CreateWorkspace(ctx, &models.Workspace{
		ID: workspaceID, Name: workspaceID, OwnerID: userID,
	}); err != nil {
		t.Fatalf("CreateWorkspace(%s): %v", workspaceID, err)
	}
	taskID := workspaceID + "-chat"
	if err := harness.taskRepo.CreateTask(ctx, &models.Task{
		ID: taskID, WorkspaceID: workspaceID, Title: title,
		State: "todo", Priority: "medium", IsEphemeral: true,
	}); err != nil {
		t.Fatalf("CreateTask(%s): %v", taskID, err)
	}
	if err := harness.taskRepo.CreateTaskSession(ctx, &models.TaskSession{
		ID: taskID + "-session", TaskID: taskID,
		State: models.TaskSessionStateCompleted, IsPrimary: true,
	}); err != nil {
		t.Fatalf("CreateTaskSession(%s): %v", taskID, err)
	}
}

func bootQuickChatNames(t *testing.T, params routeParams, userID, workspaceID string) []string {
	t.Helper()
	ctx := authn.WithIdentity(t.Context(), authn.Identity{UserID: userID, Role: authn.RoleMember})
	req := httptest.NewRequest(http.MethodGet, "/?workspaceId="+workspaceID, nil).WithContext(ctx)
	state := bootInitialState(ctx, req, params, webapp.RouteClassification{Route: webapp.RouteHome})

	quickChat, ok := state["quickChat"].(map[string]any)
	if !ok {
		return nil
	}
	sessions, _ := quickChat["sessions"].([]map[string]any)
	names := make([]string, 0, len(sessions))
	for _, session := range sessions {
		name, _ := session["name"].(string)
		names = append(names, name)
	}
	return names
}

// TestBootStateQuickChatIsScopedToTheCaller is the regression guard for the
// second ListQuickChatSessions caller. Its main job is the owner case: the boot
// payload must still carry the caller's tabs now that the service can refuse.
//
// The foreign case documents rather than detects. resolveQuickChatWorkspaceID
// validates every candidate against the caller's own visible workspace list, so
// a foreign ID never reaches the service at all -- removing the service guard
// entirely leaves this assertion green. That is the property being pinned: the
// boot path fails closed one layer up, so a future refactor that starts
// trusting a request-supplied workspace ID here has to break this test first.
func TestBootStateQuickChatIsScopedToTheCaller(t *testing.T) {
	harness := newBootStateTestHarness(t)
	params := routeParams{taskSvc: harness.taskSvc, userCtrl: harness.userCtrl}
	seedOwnedQuickChat(t, harness, "ws-boot-a", "user-a", "A's boot chat")
	seedOwnedQuickChat(t, harness, "ws-boot-b", "user-b", "B's boot chat")

	owned := bootQuickChatNames(t, params, "user-a", "ws-boot-a")
	if len(owned) != 1 || owned[0] != "A's boot chat" {
		t.Fatalf("owner boot payload quick chats = %v; want [A's boot chat]", owned)
	}

	// Asking for user-b's workspace must not put user-b's tab in user-a's boot
	// payload -- neither directly nor by falling back to it.
	foreign := bootQuickChatNames(t, params, "user-a", "ws-boot-b")
	for _, name := range foreign {
		if name == "B's boot chat" {
			t.Fatalf("boot payload leaked a foreign quick chat: %v", foreign)
		}
	}
}
