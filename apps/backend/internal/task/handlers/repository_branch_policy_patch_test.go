package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/dto"
	"github.com/kandev/kandev/internal/task/models"
	ws "github.com/kandev/kandev/pkg/websocket"
)

func observePolicyPatchEvents(t *testing.T, eventBus *bus.MemoryEventBus) *[]*bus.Event {
	t.Helper()
	published := []*bus.Event{}
	sub, err := eventBus.Subscribe(events.RepositoryBranchPolicyUpdated, func(_ context.Context, event *bus.Event) error {
		published = append(published, event)
		return nil
	})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sub.Unsubscribe()) })
	return &published
}

// @covers AC-WORKSPACES-BRANCH-POLICIES-001.7, AC-WORKSPACES-BRANCH-POLICIES-001.9
func TestBranchPolicyPatchHTTP(t *testing.T) {
	router, _, repo, eventBus := newRepositoryBranchPolicyTestRouterWithBus(t, "Policy workspace")
	policy := seedHandlerBranchPolicy(t, repo)
	published := observePolicyPatchEvents(t, eventBus)
	updated := doJSON(t, router, http.MethodPatch, "/api/v1/repository-branch-policies/"+policy.ID,
		map[string]any{"description": " Updated ", "base_branch": "develop", "pull_request_target": nil})
	require.Equal(t, http.StatusOK, updated.Code, updated.Body.String())
	var response dto.RepositoryBranchPolicyDTO
	require.NoError(t, json.Unmarshal(updated.Body.Bytes(), &response))
	stored, err := repo.GetRepositoryBranchPolicy(context.Background(), policy.ID)
	require.NoError(t, err)
	require.Equal(t, "Updated", stored.Description)
	require.Equal(t, "develop", stored.BaseBranch)
	require.Equal(t, "main", stored.PullRequestTarget)
	require.Equal(t, policy.Name, stored.Name)
	require.Equal(t, policy.BranchTemplate, stored.BranchTemplate)
	require.Equal(t, dto.FromRepositoryBranchPolicy(stored), response)
	assertPolicyPatchTransportEvent(t, *published, stored)
	failed := doJSON(t, router, http.MethodPatch, "/api/v1/repository-branch-policies/"+policy.ID,
		map[string]any{"name": "", "base_branch": "release"})
	require.Equal(t, http.StatusBadRequest, failed.Code)
	require.Len(t, *published, 1)
	saved, err := repo.GetRepositoryBranchPolicy(context.Background(), policy.ID)
	require.NoError(t, err)
	require.Equal(t, *stored, *saved)
}

// @covers AC-WORKSPACES-BRANCH-POLICIES-001.7, AC-WORKSPACES-BRANCH-POLICIES-001.9
func TestBranchPolicyPatchWS(t *testing.T) {
	_, dispatcher, repo, eventBus := newRepositoryBranchPolicyTestRouterWithBus(t, "Policy workspace")
	policy := seedHandlerBranchPolicy(t, repo)
	published := observePolicyPatchEvents(t, eventBus)
	updated := dispatchBranchPolicyAction(t, dispatcher, ws.ActionRepositoryBranchPolicyUpdate,
		map[string]any{"id": policy.ID, "description": " Updated "})
	require.NotEqual(t, ws.MessageTypeError, updated.Type, string(updated.Payload))
	var response dto.RepositoryBranchPolicyDTO
	require.NoError(t, json.Unmarshal(updated.Payload, &response))
	stored, err := repo.GetRepositoryBranchPolicy(context.Background(), policy.ID)
	require.NoError(t, err)
	require.Equal(t, "Updated", stored.Description)
	require.Equal(t, policy.BaseBranch, stored.BaseBranch)
	require.Equal(t, policy.PullRequestTarget, stored.PullRequestTarget)
	require.Equal(t, dto.FromRepositoryBranchPolicy(stored), response)
	assertPolicyPatchTransportEvent(t, *published, stored)
	failed := dispatchBranchPolicyAction(t, dispatcher, ws.ActionRepositoryBranchPolicyUpdate,
		map[string]any{"id": policy.ID, "base_branch": "bad..ref", "description": "invalid"})
	require.Equal(t, ws.MessageTypeError, failed.Type)
	var failure ws.ErrorPayload
	require.NoError(t, json.Unmarshal(failed.Payload, &failure))
	require.Equal(t, ws.ErrorCodeValidation, failure.Code)
	require.Len(t, *published, 1)
	saved, err := repo.GetRepositoryBranchPolicy(context.Background(), policy.ID)
	require.NoError(t, err)
	require.Equal(t, *stored, *saved)
}

func assertPolicyPatchTransportEvent(t *testing.T, published []*bus.Event, policy *models.RepositoryBranchPolicy) {
	t.Helper()
	require.Len(t, published, 1)
	data := published[0].Data.(map[string]interface{})
	for key, value := range map[string]string{"id": policy.ID, "repository_id": policy.RepositoryID,
		"name": policy.Name, "description": policy.Description, "base_branch": policy.BaseBranch,
		"branch_template": policy.BranchTemplate, "pull_request_target": policy.PullRequestTarget} {
		require.Equal(t, value, data[key], key)
	}
	require.NotEmpty(t, data["created_at"])
	require.NotEmpty(t, data["updated_at"])
}
