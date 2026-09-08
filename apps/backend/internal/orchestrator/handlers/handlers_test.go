package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/orchestrator"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
)

func setupOrchestratorHandlers(t *testing.T) *Handlers {
	t.Helper()
	log, err := logger.NewLogger(logger.LoggingConfig{
		Level:      "error",
		Format:     "console",
		OutputPath: "stderr",
	})
	require.NoError(t, err)
	return NewHandlers(&orchestrator.Service{}, log)
}

func TestWsRecoverSessionCancelRetryReportsServiceResult(t *testing.T) {
	handlers := setupOrchestratorHandlers(t)
	response, err := handlers.wsRecoverSession(context.Background(), createTestMessage(t, ws.ActionSessionRecover, map[string]interface{}{
		"task_id":    "t1",
		"session_id": "s1",
		"action":     "cancel_retry",
	}))
	require.NoError(t, err)

	var payload struct {
		Cancelled bool `json:"cancelled"`
	}
	require.NoError(t, json.Unmarshal(response.Payload, &payload))
	require.False(t, payload.Cancelled)
}

func TestBranchRecoveryConflictResponsePreservesRecoveryDetails(t *testing.T) {
	msg := createTestMessage(t, ws.ActionSessionRecover, map[string]interface{}{})
	err := &orchestrator.BranchRecoveryError{
		Cause:          errors.New("branch is gone"),
		SessionID:      "session-1",
		RepositoryID:   "repo-1",
		OriginalBranch: "feature/lost",
		BaseBranch:     "main",
	}

	response, responseErr := branchRecoveryConflictResponse(msg, err)
	require.NoError(t, responseErr)
	require.NotNil(t, response)
	payload := parseError(t, response)
	require.Equal(t, ws.ErrorCodeConflict, payload.Code)
	require.Equal(t, "feature/lost", payload.Details["original_branch"])
	require.Equal(t, "main", payload.Details["base_branch"])
	require.Equal(t, "resume_new_branch", payload.Details["recovery_action"])
}

func TestWsEnsureSessionRequestParsesAutoStartOverride(t *testing.T) {
	tests := []struct {
		name      string
		payload   map[string]interface{}
		wantSet   bool
		wantValue bool
	}{
		{name: "absent auto_start stays nil", payload: map[string]interface{}{"task_id": "t1"}, wantSet: false},
		{name: "auto_start false parses", payload: map[string]interface{}{"task_id": "t1", "auto_start": false}, wantSet: true, wantValue: false},
		{name: "auto_start true parses", payload: map[string]interface{}{"task_id": "t1", "auto_start": true}, wantSet: true, wantValue: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := createTestMessage(t, ws.ActionSessionEnsure, tt.payload)
			var req wsEnsureSessionRequest
			require.NoError(t, msg.ParsePayload(&req))
			if tt.wantSet {
				require.NotNil(t, req.AutoStart, "auto_start should be set")
				require.Equal(t, tt.wantValue, *req.AutoStart)
			} else {
				require.Nil(t, req.AutoStart, "auto_start should be absent")
			}
		})
	}
}

func TestWsRespondToPermissionRequiresTaskAndRequestIdentity(t *testing.T) {
	handlers := setupOrchestratorHandlers(t)
	for _, test := range []struct {
		name    string
		payload map[string]any
		want    string
	}{
		{
			name: "missing task",
			payload: map[string]any{
				"session_id": "session-1", "request_id": "request-1", "pending_id": "pending-1", "option_id": "allow-once",
			},
			want: "task_id is required",
		},
		{
			name: "missing request generation",
			payload: map[string]any{
				"task_id": "task-1", "session_id": "session-1", "pending_id": "pending-1", "option_id": "allow-once",
			},
			want: "request_id is required",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			response, err := handlers.wsRespondToPermission(context.Background(), createTestMessage(t, ws.ActionPermissionRespond, test.payload))
			require.NoError(t, err)
			var payload ws.ErrorPayload
			require.NoError(t, json.Unmarshal(response.Payload, &payload))
			require.Equal(t, ws.ErrorCodeValidation, payload.Code)
			require.Equal(t, test.want, payload.Message)
		})
	}
}
