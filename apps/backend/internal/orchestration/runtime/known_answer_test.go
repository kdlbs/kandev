package runtime

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/kandev/kandev/internal/orchestration/models"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestAssistantKnownAnswerScope(t *testing.T) {
	s, token, run, goal := assistantContextFixture(t)
	ctx := context.Background()
	agent, human := assistantRuntimeRouter(s), assistantRouter(s)
	updated := runtimeRequest(t, agent, "PATCH", "/api/v1/orchestration/runtime/objectives/"+goal, token, run, map[string]any{"mode": "execute", "expected_revision": 1, "operation_id": "execute", "expected_intent_revision": 0})
	require.Equal(t, 200, updated.Code, updated.Body.String())
	path := "/api/v1/orchestration/assistant/memory/color"
	saved := runtimeRequest(t, human, "PUT", path, "", "", map[string]any{"key": "color", "content": "Use blue for the example", "scope": "workspace", "source_comment_id": "source", "confirmed": true})
	require.Equal(t, 200, saved.Code, saved.Body.String())
	var memory models.ContextMemory
	require.NoError(t, json.Unmarshal(saved.Body.Bytes(), &memory))
	worker := &taskmodels.Task{ID: "worker", WorkspaceID: "ws", Metadata: map[string]any{"orchestration_objective_id": goal}}
	s.Tasks.(*testTasks).tasks[worker.ID] = worker
	response := runtimeRequest(t, agent, "GET", "/api/v1/orchestration/runtime/context/"+goal+"?profile_id=personal&task_id=worker", token, run, nil)
	require.Equal(t, 200, response.Code, response.Body.String())
	var packet models.ContextPacket
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &packet))
	worker.Metadata["orchestration_binding_id"] = packet.BindingID
	binding, err := s.Repo.AssistantBindingByID(ctx, packet.BindingID)
	require.NoError(t, err)
	input := &models.AttentionInput{Kind: "question", ProfileID: "personal", Questions: []models.InputQuestion{{ID: "color", AssistantDelegable: true}}}
	request := &attentionResponseRequest{ContextRef: packet.ID, MemoryIDs: []string{memory.ID}}
	row := models.Attention{TaskID: worker.ID}
	require.NoError(t, s.knownAnswerContext(ctx, binding, row, input, request))
	for _, kind := range []string{"permission", "authentication"} {
		input.Kind = kind
		require.Error(t, s.knownAnswerContext(ctx, binding, row, input, request))
	}
	input.Kind = "question"
	input.Questions[0].AssistantDelegable = false
	require.Error(t, s.knownAnswerContext(ctx, binding, row, input, request))
	input.Questions[0].AssistantDelegable = true
	input.ProfileID = "another-account"
	require.Error(t, s.knownAnswerContext(ctx, binding, row, input, request))
	input.ProfileID = "personal"
	request.MemoryIDs = []string{"uncited-or-unconfirmed"}
	require.Error(t, s.knownAnswerContext(ctx, binding, row, input, request))
	request.MemoryIDs = []string{memory.ID}
	require.Equal(t, 200, runtimeRequest(t, human, "DELETE", path, "", "", map[string]int{"expected_revision": 1}).Code)
	require.Error(t, s.knownAnswerContext(ctx, binding, row, input, request))
}

func TestAssistantKnownAnswerBrokerActorAndPermissionDenial(t *testing.T) {
	s, b, row, input := assistantInputFixture(t)
	ctx := context.Background()
	input.input.ProfileID = "personal"
	input.input.Questions[0].AssistantDelegable = true
	run, err := s.Runs.ClaimNextEligibleRun(ctx)
	require.NoError(t, err)
	require.NoError(t, s.Runs.UpdateRunRuntimeSnapshot(ctx, run.ID, assistantBrokerAudience, run.Payload, "session"))
	token, err := s.Auth.MintRuntimeJWT(b.OrchestratorID, b.ConversationID, b.WorkspaceID, run.ID, "session", assistantBrokerAudience)
	require.NoError(t, err)
	agent := assistantRuntimeRouter(s)
	saved := runtimeRequest(t, assistantRouter(s), "PUT", "/api/v1/orchestration/assistant/memory/color", "", "", map[string]any{"key": "color", "content": "Use blue for the example", "scope": "workspace", "source_comment_id": "source", "confirmed": true})
	require.Equal(t, 200, saved.Code, saved.Body.String())
	var memory models.ContextMemory
	require.NoError(t, json.Unmarshal(saved.Body.Bytes(), &memory))
	goals, err := s.Repo.Objectives(ctx, b.ID, "", 10)
	require.NoError(t, err)
	require.Len(t, goals, 1)
	worker := s.Tasks.(*testTasks).tasks["worker"]
	worker.Metadata = map[string]any{"orchestration_binding_id": b.ID, "orchestration_objective_id": goals[0].ID}
	packetResponse := runtimeRequest(t, agent, "GET", "/api/v1/orchestration/runtime/context/"+goals[0].ID+"?profile_id=personal&task_id=worker", token, run.ID, nil)
	require.Equal(t, 200, packetResponse.Code, packetResponse.Body.String())
	var packet models.ContextPacket
	require.NoError(t, json.Unmarshal(packetResponse.Body.Bytes(), &packet))
	body := inputBody(row, "known-answer")
	body["context_ref"], body["memory_ids"] = packet.ID, []string{memory.ID}
	body["actor_type"], body["actor_id"] = "user", "owner"
	response := runtimeRequest(t, agent, "POST", "/api/v1/orchestration/runtime/attention/"+row.ID+"/answer", token, run.ID, body)
	require.Equal(t, 200, response.Code, response.Body.String())
	require.Equal(t, "agent", input.actor.ActorType)
	require.Equal(t, b.OrchestratorID, input.actor.ActorID)
	require.Equal(t, []string{memory.ID}, input.actor.SourceMemoryIDs)
	rows, err := s.Repo.AttentionPage(ctx, b.ID, "", 10)
	require.NoError(t, err)
	for _, permission := range rows {
		if permission.Kind == "permission" {
			input.input = models.AttentionInput{SourceID: permission.SourceID, Kind: permission.Kind, State: "pending", SourceRevision: permission.SourceRevision, SessionID: permission.SessionID, TaskID: permission.TaskID}
			body = inputBody(permission, "permission-denied")
			body["context_ref"], body["memory_ids"], body["option_id"] = packet.ID, []string{memory.ID}, "allow"
			response = runtimeRequest(t, agent, "POST", "/api/v1/orchestration/runtime/attention/"+permission.ID+"/answer", token, run.ID, body)
			require.Equal(t, 403, response.Code, response.Body.String())
		}
	}
	require.Equal(t, 1, input.calls)
}
