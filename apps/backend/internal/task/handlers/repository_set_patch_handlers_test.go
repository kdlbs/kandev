package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/dto"
	ws "github.com/kandev/kandev/pkg/websocket"
)

// @covers AC-WORKSPACES-REPOSITORY-SETS-001.7, AC-WORKSPACES-REPOSITORY-SETS-001.9
func TestHTTPRepositorySetPatchPresenceAndEvents(t *testing.T) {
	testSetPatchBoundary(t, false)
}

func TestWSRepositorySetPatchPresenceAndEvents(t *testing.T) {
	testSetPatchBoundary(t, true)
}

type setPatchBoundaryCase struct {
	name                      string
	body                      map[string]any
	wantName, wantDescription string
	memberChange, invalid     bool
}

func testSetPatchBoundary(t *testing.T, websocket bool) {
	items := []map[string]any{
		{"repository_id": "repo-orders", "base_branch": "release-orders"},
		{"repository_id": "repo-web", "base_branch": "release-web"},
	}
	cases := []setPatchBoundaryCase{
		{"name", map[string]any{"name": "Renamed"}, "Renamed", "Original description", false, false},
		{"description", map[string]any{"description": "Changed"}, "Original", "Changed", false, false},
		{"clear", map[string]any{"description": ""}, "Original", "", false, false},
		{"members", map[string]any{"repositories": items}, "Original", "Original description", true, false},
		{"legacy members", map[string]any{"repository_ids": []string{"repo-orders", "repo-web"}},
			"Original", "Original description", true, false},
		{"combined", map[string]any{"name": "Both", "description": "Combined", "repositories": items},
			"Both", "Combined", true, false},
		{"no-op", map[string]any{}, "Original", "Original description", false, false},
		{"null", map[string]any{"name": nil, "description": nil, "repositories": nil},
			"Original", "Original description", false, false},
		{"invalid combined", map[string]any{"name": "Rejected", "description": "Rejected",
			"repositories": []map[string]any{}}, "Original", "Original description", false, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) { runSetPatchBoundary(t, websocket, tc) })
	}
}

func runSetPatchBoundary(t *testing.T, websocket bool, tc setPatchBoundaryCase) {
	t.Helper()
	router, dispatcher, repo, eventBus := newRepositorySetTestRouterWithStore(t)
	created := createSetViaHTTP(t, router, map[string]any{
		"name": "Original", "description": "Original description",
		"repositories": []map[string]any{
			{"repository_id": "repo-gateway", "base_branch": "original-gateway"},
			{"repository_id": "repo-web", "base_branch": "original-web"},
		},
	})
	before, err := repo.GetRepositorySet(context.Background(), created.ID)
	require.NoError(t, err)
	updates := make(chan *bus.Event, 1)
	sub, err := eventBus.Subscribe("repository_set.updated", func(_ context.Context, event *bus.Event) error {
		updates <- event
		return nil
	})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sub.Unsubscribe()) })
	var response dto.RepositorySetDTO
	if websocket {
		response = requestSetPatchWS(t, dispatcher, created.ID, tc)
	} else {
		response = requestSetPatchHTTP(t, router, created.ID, tc)
	}
	stored, err := repo.GetRepositorySet(context.Background(), created.ID)
	require.NoError(t, err)
	if tc.invalid {
		require.Equal(t, before, stored)
		select {
		case event := <-updates:
			t.Fatalf("rejected patch emitted update: %#v", event)
		default:
		}
		return
	}
	require.Equal(t, dto.FromRepositorySet(stored), response)
	require.Equal(t, tc.wantName, stored.Name)
	require.Equal(t, tc.wantDescription, stored.Description)
	assertSetPatchBoundaryMembers(t, tc, created, response)
	assertSetPatchBoundaryEvent(t, updates, response)
}

func requestSetPatchWS(t *testing.T, dispatcher *ws.Dispatcher, id string, tc setPatchBoundaryCase) dto.RepositorySetDTO {
	t.Helper()
	tc.body["id"] = id
	request, err := ws.NewRequest("patch-request", ws.ActionRepositorySetUpdate, tc.body)
	require.NoError(t, err)
	message, err := dispatcher.Dispatch(context.Background(), request)
	require.NoError(t, err)
	require.Equal(t, request.ID, message.ID)
	require.Equal(t, request.Action, message.Action)
	if tc.invalid {
		require.Equal(t, ws.MessageTypeError, message.Type)
		var failure ws.ErrorPayload
		require.NoError(t, json.Unmarshal(message.Payload, &failure))
		require.Equal(t, ws.ErrorCodeValidation, failure.Code)
		return dto.RepositorySetDTO{}
	}
	require.Equal(t, ws.MessageTypeResponse, message.Type)
	var response dto.RepositorySetDTO
	require.NoError(t, json.Unmarshal(message.Payload, &response))
	return response
}

func requestSetPatchHTTP(t *testing.T, router *gin.Engine, id string, tc setPatchBoundaryCase) dto.RepositorySetDTO {
	t.Helper()
	recorder := doJSON(t, router, http.MethodPatch, "/api/v1/repository-sets/"+id, tc.body)
	if tc.invalid {
		require.Equal(t, http.StatusBadRequest, recorder.Code)
		return dto.RepositorySetDTO{}
	}
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	var response dto.RepositorySetDTO
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	return response
}

func assertSetPatchBoundaryMembers(t *testing.T, tc setPatchBoundaryCase, before, after dto.RepositorySetDTO) {
	t.Helper()
	if !tc.memberChange {
		require.Equal(t, before.Repositories, after.Repositories)
		return
	}
	require.Len(t, after.Repositories, 2)
	require.Equal(t, "repo-orders", after.Repositories[0].RepositoryID)
	require.Equal(t, "repo-web", after.Repositories[1].RepositoryID)
	require.Equal(t, 0, after.Repositories[0].Position)
	require.Equal(t, 1, after.Repositories[1].Position)
	if tc.body["repository_ids"] != nil {
		require.Empty(t, after.Repositories[0].BaseBranch)
		require.Empty(t, after.Repositories[1].BaseBranch)
	} else {
		require.Equal(t, "release-orders", after.Repositories[0].BaseBranch)
		require.Equal(t, "release-web", after.Repositories[1].BaseBranch)
	}
}

func assertSetPatchBoundaryEvent(t *testing.T, updates <-chan *bus.Event, response dto.RepositorySetDTO) {
	t.Helper()
	select {
	case event := <-updates:
		require.Equal(t, "repository_set.updated", event.Type)
		encoded, err := json.Marshal(event.Data)
		require.NoError(t, err)
		var projected dto.RepositorySetDTO
		require.NoError(t, json.Unmarshal(encoded, &projected))
		response.CreatedAt = response.CreatedAt.Truncate(time.Second)
		response.UpdatedAt = response.UpdatedAt.Truncate(time.Second)
		require.Equal(t, response, projected)
	case <-time.After(5 * time.Second):
		t.Fatal("successful patch did not publish an update")
	}
}
