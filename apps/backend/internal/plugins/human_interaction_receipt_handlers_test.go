package plugins

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/task/models"
)

func TestHumanInteractionReceiptHandlerIssuesForSessionControllerAndCurrentRevision(t *testing.T) {
	router, svc := newTestRouterWithIdentity(t, authn.Identity{UserID: "workspace-collaborator", Role: authn.RoleMember})
	data := newTestDataHost(readWriteCaps())
	interaction := pendingPermissionInteraction()
	withInteraction(data, interaction)
	data.tasks.tasksByID = map[string]*models.Task{
		interaction.TaskID: {ID: interaction.TaskID, WorkspaceID: capabilityApprovalTestWorkspace, UpdatedAt: interaction.UpdatedAt},
	}
	svc.SetDataSources(data.tasks, nil, nil, nil, nil, nil, data.interactions, nil)
	svc.SetHumanInteractionResponseAuthorizer(func(ctx context.Context, workspaceID string) error {
		identity, ok := authn.IdentityFromContext(ctx)
		if !ok || identity.UserID != "workspace-collaborator" || workspaceID != capabilityApprovalTestWorkspace {
			return fmt.Errorf("workspace session control denied")
		}
		return nil
	})

	version := digestPublicValue(interactionModelToDTO(interaction))
	endpoint := "/api/plugins/host/interactions/response-receipts"
	stale := doRequest(router, http.MethodPost, endpoint,
		`{"workspace_id":"`+capabilityApprovalTestWorkspace+`","interaction_id":"`+interaction.ID+`","expected_resource_version":"sha256:stale","kind":"permission","option_id":"allow"}`,
		jsonHeaders())
	if stale.Code != http.StatusConflict {
		t.Fatalf("stale receipt status = %d, body=%s, want 409", stale.Code, stale.Body.String())
	}

	valid := doRequest(router, http.MethodPost, endpoint,
		`{"workspace_id":"`+capabilityApprovalTestWorkspace+`","interaction_id":"`+interaction.ID+`","expected_resource_version":"`+version+`","kind":"permission","option_id":"allow"}`,
		jsonHeaders())
	if valid.Code != http.StatusCreated {
		t.Fatalf("valid receipt status = %d, body=%s, want 201", valid.Code, valid.Body.String())
	}
	var issued issueHumanInteractionResponseReceiptResponse
	if err := json.Unmarshal(valid.Body.Bytes(), &issued); err != nil {
		t.Fatalf("decode receipt response: %v", err)
	}
	if issued.ID == "" || issued.InteractionID != interaction.ID || issued.ResourceVersion != version {
		t.Fatalf("issued receipt = %+v", issued)
	}

	svc.humanInteractionReceipts.mu.Lock()
	record, found := svc.humanInteractionReceipts.entries[issued.ID]
	svc.humanInteractionReceipts.mu.Unlock()
	if !found || record.HumanActor != "workspace-collaborator" || record.PayloadDigest != exactHumanResponseDigest("permission", HumanInteractionResponse{Kind: "permission", OptionID: "allow"}) {
		t.Fatalf("stored receipt = %+v, found=%v", record, found)
	}
}

func TestHumanInteractionReceiptHandlerRejectsUnauthorizedWorkspace(t *testing.T) {
	router, svc := newTestRouterWithIdentity(t, authn.Identity{UserID: "workspace-member", Role: authn.RoleMember})
	data := newTestDataHost(readWriteCaps())
	interaction := pendingPermissionInteraction()
	withInteraction(data, interaction)
	data.tasks.tasksByID = map[string]*models.Task{
		interaction.TaskID: {ID: interaction.TaskID, WorkspaceID: capabilityApprovalTestWorkspace, UpdatedAt: time.Now().UTC()},
	}
	svc.SetDataSources(data.tasks, nil, nil, nil, nil, nil, data.interactions, nil)
	svc.SetHumanInteractionResponseAuthorizer(func(context.Context, string) error {
		return fmt.Errorf("workspace session control denied")
	})

	request := `{"workspace_id":"` + capabilityApprovalTestWorkspace + `","interaction_id":"` + interaction.ID + `","expected_resource_version":"` + digestPublicValue(interactionModelToDTO(interaction)) + `","kind":"permission","option_id":"allow"}`
	response := doRequest(router, http.MethodPost, "/api/plugins/host/interactions/response-receipts", request, jsonHeaders())
	if response.Code != http.StatusForbidden {
		t.Fatalf("unauthorized receipt status = %d, body=%s, want 403", response.Code, response.Body.String())
	}
	if got := len(svc.humanInteractionReceipts.entries); got != 0 {
		t.Fatalf("unauthorized request issued %d receipts, want none", got)
	}
}
