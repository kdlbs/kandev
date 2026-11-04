package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/service"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
)

func newTurnErrorMessageHarness(t *testing.T) (*MessageHandlers, *messageAddSwitchRepo) {
	t.Helper()
	now := time.Now().UTC()
	repo := &messageAddSwitchRepo{
		tasks: map[string]*models.Task{"t1": {
			ID: "t1", WorkspaceID: "ws1", State: v1.TaskStateInProgress, UpdatedAt: now,
			Metadata: map[string]interface{}{
				models.MetaKeyLastTurnError: models.TaskTurnError{
					StampValue: "failure-1", SessionID: "s1", MessageID: "failure-1",
					OccurredAt: now.Add(-time.Minute), Message: "Failed to execute statement",
				},
			},
		}},
		sessions: map[string]*models.TaskSession{
			"s1": {ID: "s1", TaskID: "t1", State: models.TaskSessionStateWaitingForInput, UpdatedAt: now},
		},
		primaryID: "s1",
	}
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	require.NoError(t, err)
	svc := service.NewService(service.Repos{Tasks: repo, TaskRepos: repo, Messages: repo, Turns: repo, Sessions: repo}, nil, log, service.RepositoryDiscoveryConfig{})
	return NewMessageHandlers(svc, nil, log, nil), repo
}

func TestAcceptedManualPromptAcknowledgesTaskTurnError(t *testing.T) {
	h, repo := newTurnErrorMessageHarness(t)
	req, err := ws.NewRequest("req-turn-error", ws.ActionMessageAdd, map[string]any{
		"task_id": "t1", "session_id": "s1", "content": "follow up",
		"observed_turn_error_stamp": "failure-1",
	})
	require.NoError(t, err)
	resp, err := h.wsAddMessage(context.Background(), req)
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, resp.Type)
	require.Len(t, repo.messages, 1)
	require.Equal(t, "failure-1", repo.acknowledgedTurnError,
		"an accepted manual prompt acknowledges only the client-observed same-session stamp")
}

func TestAcceptedManualPromptWithoutObservationLeavesAttention(t *testing.T) {
	h, repo := newTurnErrorMessageHarness(t)
	req, err := ws.NewRequest("req-no-observation", ws.ActionMessageAdd, map[string]any{
		"task_id": "t1", "session_id": "s1", "content": "follow up",
	})
	require.NoError(t, err)
	resp, err := h.wsAddMessage(context.Background(), req)
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, resp.Type)
	require.Empty(t, repo.acknowledgedTurnError,
		"omitting the observation never substitutes the backend's current stamp")
}

func TestAcceptedManualPromptReportsAcknowledgementFailure(t *testing.T) {
	h, repo := newTurnErrorMessageHarness(t)
	repo.turnErrorAckErr = errors.New("metadata write failed")
	req, err := ws.NewRequest("req-turn-error-failure", ws.ActionMessageAdd, map[string]any{
		"task_id": "t1", "session_id": "s1", "content": "follow up",
		"observed_turn_error_stamp": "failure-1",
	})
	require.NoError(t, err)
	resp, err := h.wsAddMessage(context.Background(), req)
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, resp.Type, "the accepted prompt is never rejected")
	require.Len(t, repo.messages, 1, "the prompt is not resubmitted")
	var payload struct {
		TurnErrorAcknowledgementFailed bool `json:"turn_error_acknowledgement_failed"`
	}
	require.NoError(t, json.Unmarshal(resp.Payload, &payload))
	require.True(t, payload.TurnErrorAcknowledgementFailed,
		"the client needs nonfatal feedback when the badge could not be retired")
}

func TestAddMessageFingerprintIncludesObservedTurnErrorStamp(t *testing.T) {
	base := wsAddMessageRequest{TaskID: "t1", TaskSessionID: "s1", Content: "hello"}
	observed := base
	observed.ObservedTurnErrorStamp = "failure-1"
	baseFingerprint, err := addMessageRequestFingerprint(base)
	require.NoError(t, err)
	observedFingerprint, err := addMessageRequestFingerprint(observed)
	require.NoError(t, err)
	require.NotEqual(t, baseFingerprint, observedFingerprint,
		"a replay must not attach a newly observed stamp to older input")
}
