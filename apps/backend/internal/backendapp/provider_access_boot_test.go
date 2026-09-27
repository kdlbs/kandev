package backendapp

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/provideraccess"
)

func TestBootWiresProviderAccessFenceBeforeWorkspaceDeletion(t *testing.T) {
	services, _, _ := provideTestServices(t, "provider-access-boot")
	if services.ProviderAccess == nil {
		t.Fatal("provider access store is unavailable after boot")
	}
	workspaceID := seedOwnedWorkspace(t, services.Task, "provider-access-owner")
	grant := provideraccess.Grant{
		ID: "provider-access-boot-grant", CreatedByUserID: "provider-access-owner",
		ExpiresAt: time.Now().Add(10 * time.Minute),
		GrantScope: provideraccess.GrantScope{
			PluginInstallationID: "installation-1", PluginID: "plugin-1",
			WorkspaceID: workspaceID, ConversationKey: "conversation-1",
			TargetTaskID: "task-1", RepositoryID: "owner/repository",
			Provider: "github", Purpose: "actions_write",
		},
	}
	ctx := context.Background()
	if err := services.ProviderAccess.ReplaceGrant(ctx, &grant); err != nil {
		t.Fatal(err)
	}
	if err := services.Task.DeleteWorkspace(ctx, workspaceID); err != nil {
		t.Fatal(err)
	}
	if retained, err := services.ProviderAccess.GetGrant(ctx, grant.ID); err != nil || retained != nil {
		t.Fatalf("ordinary grant after workspace deletion = %+v, err = %v", retained, err)
	}
	newGrant := grant
	newGrant.ID = "provider-access-late-grant"
	if err := services.ProviderAccess.ReplaceGrant(ctx, &newGrant); !errors.Is(err, provideraccess.ErrGrantUnavailable) {
		t.Fatalf("late grant after workspace deletion = %v, want fenced", err)
	}
}
