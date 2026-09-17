package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/kandev/kandev/internal/orchestration/models"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestAssistantContextPaginationContinuations(t *testing.T) {
	s, token, run, goal := assistantContextFixture(t)
	ctx := context.Background()
	b, err := s.Repo.AssistantBinding(ctx, "owner")
	require.NoError(t, err)
	s.Tasks.(*testTasks).tasks["own-task"] = &taskmodels.Task{ID: "own-task", WorkspaceID: "ws"}
	for i := 0; i < 105; i++ {
		id := fmt.Sprintf("entry-%03d", i)
		require.NoError(t, s.Repo.SaveAssistantMemory(ctx, &models.AgentMemory{ID: id, AgentProfileID: b.OrchestratorID, OwnerUserID: b.OwnerUserID, Layer: "user", Key: id, Content: "Confirmed example preference", Scope: "task", ScopeID: "own-task", Confirmed: true}, 0))
	}
	router := assistantRuntimeRouter(s)
	response := runtimeRequest(t, router, "GET", "/api/v1/orchestration/runtime/context/"+goal+"?profile_id=personal&task_id=own-task", token, run, nil)
	require.Equal(t, 200, response.Code, response.Body.String())
	var packet models.ContextPacket
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &packet))
	require.Contains(t, packet.MemoryReference, "/runtime/context/")
	require.Greater(t, packet.OmittedMemory, 0)
	response = runtimeRequest(t, router, "GET", packet.MemoryReference, token, run, nil)
	require.Equal(t, 200, response.Code, response.Body.String())
	var page struct {
		Memory []models.AgentMemory `json:"memory"`
		Next   string               `json:"next_cursor"`
	}
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &page))
	require.Len(t, page.Memory, 50)
	require.NotEmpty(t, page.Next)
	other := runtimeRequest(t, router, "GET", "/api/v1/orchestration/runtime/context/"+goal+"/memory?profile_id=personal&after="+page.Next, token, run, nil)
	require.Equal(t, 400, other.Code)
}
