package mcp

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
	ws "github.com/kandev/kandev/pkg/websocket"
)

func TestServerSurfaceCoordinatorRegistersBoundAnswerTool(t *testing.T) {
	log := newTestLogger(t)
	backend := NewChannelBackendClient(log)
	defer backend.Close()

	bound := func(names ...string) []string {
		profile := mcpprofile.NewCoordinator()
		profile.CoordinatorToolPolicy = &mcpprofile.CoordinatorToolPolicy{
			Version: 1, CoordinatorID: "c", WorkspaceID: "w", ConversationTaskID: "t", ToolNames: names,
		}
		return getRegisteredToolNames(NewWithProfile(backend, "s", "t", 10005, log, "", false, profile))
	}
	assert.Contains(t, bound("list_tasks_kandev", "propose_answer_kandev"), "propose_answer_kandev")
	assert.NotContains(t, bound("list_tasks_kandev"), "propose_answer_kandev")
}

func TestAnswerErrorText(t *testing.T) {
	withField := &BackendError{Code: ws.ErrorCodeValidation, Message: "empty_answer", Details: map[string]interface{}{"field": "draft"}}
	assert.Equal(t, "empty_answer (field: draft)", answerErrorText(withField))

	bare := &BackendError{Code: ws.ErrorCodeValidation, Message: "nothing_to_answer", Details: map[string]interface{}{"code": "nothing_to_answer", "field": ""}}
	assert.Equal(t, "nothing_to_answer", answerErrorText(bare))

	noDetails := &BackendError{Code: ws.ErrorCodeValidation, Message: "too long"}
	assert.Equal(t, "too long", answerErrorText(noDetails))

	internal := &BackendError{Code: ws.ErrorCodeInternalError, Message: "could not read the task's current question"}
	assert.Equal(t, internal.Error(), answerErrorText(internal))

	plain := errors.New("closed")
	assert.Equal(t, "closed", answerErrorText(plain))
}

func TestProposeAnswerToolSchemaMatchesTheAnswerKind(t *testing.T) {
	log := newTestLogger(t)
	backend := NewChannelBackendClient(log)
	defer backend.Close()
	profile := mcpprofile.NewCoordinator()
	profile.CoordinatorToolPolicy = &mcpprofile.CoordinatorToolPolicy{
		Version: 1, CoordinatorID: "c", WorkspaceID: "w", ConversationTaskID: "t", ToolNames: []string{"propose_answer_kandev"},
	}
	s := NewWithProfile(backend, "s", "t", 10006, log, "", false, profile)
	raw, err := json.Marshal(s.mcpServer.ListTools()["propose_answer_kandev"].Tool.InputSchema)
	assert.NoError(t, err)
	var schema struct {
		Required   []string `json:"required"`
		Properties map[string]struct {
			Type  string   `json:"type"`
			Enum  []string `json:"enum"`
			Items struct {
				Type       string                       `json:"type"`
				Properties map[string]map[string]string `json:"properties"`
			} `json:"items"`
		} `json:"properties"`
	}
	assert.NoError(t, json.Unmarshal(raw, &schema))

	for _, name := range []string{"task_id", "target", "pending_id", "draft", "confidence", "needs_decision"} {
		assert.Contains(t, schema.Required, name)
	}
	assert.NotContains(t, schema.Required, "sources")
	assert.NotContains(t, schema.Required, "rationale")

	desc := s.mcpServer.ListTools()["propose_answer_kandev"].Tool.Description
	assert.NotContains(t, desc, "replaces the open one")
	assert.Contains(t, desc, "unchanged")

	confidence := schema.Properties["confidence"]
	assert.Equal(t, "string", confidence.Type)
	assert.Equal(t, []string{"low", "med", "high"}, confidence.Enum)

	sources := schema.Properties["sources"]
	assert.Equal(t, "object", sources.Items.Type)
	assert.Equal(t, "string", sources.Items.Properties["ref"]["type"])
	assert.Equal(t, "string", sources.Items.Properties["label"]["type"])
}
