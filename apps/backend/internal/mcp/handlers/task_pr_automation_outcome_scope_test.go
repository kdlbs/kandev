package handlers

import (
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/github"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReportPRAutoFixOutcomeExplainsUnmatchedTurn(t *testing.T) {
	automation := &recordingTaskPRAutomationService{outcomeErr: github.ErrTaskCIAutoFixAttemptNotFound}
	h := &Handlers{taskPRAutoFixOutcome: automation, logger: testLogger(t).WithFields()}

	response, err := h.handleReportTaskPRAutoFixOutcome(
		taskPRAutoFixOutcomeContext(),
		makeWSMessage(t, ws.ActionMCPReportPRAutoFixOutcome, map[string]any{
			"outcome": "blocked",
			"summary": "provider is unavailable",
		}),
	)

	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeError, response.Type)
	var payload ws.ErrorPayload
	require.NoError(t, response.ParsePayload(&payload))
	assert.Equal(t, ws.ErrorCodeValidation, payload.Code)
	assert.Contains(t, payload.Message, "No matching unresolved GitHub PR auto-fix attempt exists for this turn")
	assert.Contains(t, payload.Message, "Finish ordinary work without retrying this report or enabling auto-fix")
	assert.NotContains(t, strings.ToLower(payload.Message), "disabled")
}
