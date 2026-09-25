package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/service"

	ws "github.com/kandev/kandev/pkg/websocket"
)

type dismissalWSMessageRepository struct {
	mockRepository
	message   *models.Message
	updateErr error
}

func (r *dismissalWSMessageRepository) GetMessage(_ context.Context, id string) (*models.Message, error) {
	if r.message == nil || r.message.ID != id {
		return nil, errors.New("message not found")
	}
	return r.message, nil
}

func (r *dismissalWSMessageRepository) UpdateMessage(_ context.Context, message *models.Message) error {
	if r.updateErr != nil {
		return r.updateErr
	}
	r.message = message
	return nil
}

func dispatchGitPushDismissal(t *testing.T, repo *dismissalWSMessageRepository, ctx context.Context, payload any) *ws.Message {
	t.Helper()
	log := newTestLogger(t)
	svc := service.NewService(service.Repos{
		Messages: repo, Sessions: repo, Tasks: repo, Workspaces: repo,
	}, nil, log, service.RepositoryDiscoveryConfig{})
	dispatcher := ws.NewDispatcher()
	NewMessageHandlers(svc, nil, log).registerWS(dispatcher)
	request, err := ws.NewRequest("request-1", ws.ActionMessageDismissGitPushError, payload)
	if err != nil {
		t.Fatal(err)
	}
	response, err := dispatcher.Dispatch(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	return response
}

func TestWSMessageDismissGitPushError(t *testing.T) {
	t.Run("returns the exact message acknowledgment", func(t *testing.T) {
		repo := &dismissalWSMessageRepository{message: &models.Message{
			ID: "push-failure", TaskSessionID: "session-1", AuthorType: models.MessageAuthorAgent,
			Type: models.MessageTypeError, Content: "push failed",
			Metadata: map[string]any{
				"git_operation_error": true, "operation": "push", "error_output": "private diagnostics",
			},
		}}
		response := dispatchGitPushDismissal(t, repo, context.Background(), map[string]string{"message_id": "push-failure"})
		if response.Type != ws.MessageTypeResponse || response.Action != ws.ActionMessageDismissGitPushError {
			t.Fatalf("response envelope = %+v", response)
		}
		var payload map[string]any
		if err := json.Unmarshal(response.Payload, &payload); err != nil {
			t.Fatal(err)
		}
		if len(payload) != 2 || payload["message_id"] != "push-failure" {
			t.Fatalf("success payload = %#v, want only message_id and dismissed_at", payload)
		}
		if _, ok := payload["dismissed_at"].(string); !ok {
			t.Fatalf("dismissed_at = %#v, want string", payload["dismissed_at"])
		}
		if _, exists := payload["error_output"]; exists || string(response.Payload) == "" {
			t.Fatalf("response exposed diagnostic metadata: %s", response.Payload)
		}
	})

	t.Run("returns generic errors for access, validation, and persistence failures", func(t *testing.T) {
		message := func() *models.Message {
			return &models.Message{
				ID: "push-failure", TaskSessionID: "session-1", AuthorType: models.MessageAuthorAgent,
				Type: models.MessageTypeError, Content: "push failed",
				Metadata: map[string]any{"git_operation_error": true, "operation": "push", "error_output": "private diagnostics"},
			}
		}
		for _, tc := range []struct {
			name     string
			ctx      context.Context
			repo     *dismissalWSMessageRepository
			wantCode string
		}{
			{name: "cross workspace", ctx: authzWSContext("user-a"), repo: &dismissalWSMessageRepository{mockRepository: mockRepository{sessions: map[string]*models.TaskSession{"session-1": {ID: "session-1", TaskID: "foreign-task"}}}, message: message()}, wantCode: ws.ErrorCodeNotFound},
			{name: "unrelated message", ctx: context.Background(), repo: &dismissalWSMessageRepository{message: &models.Message{ID: "push-failure", TaskSessionID: "session-1", Type: models.MessageTypeError, Metadata: map[string]any{"operation": "pull", "error_output": "private diagnostics"}}}, wantCode: ws.ErrorCodeValidation},
			{name: "persistence failure", ctx: context.Background(), repo: &dismissalWSMessageRepository{message: message(), updateErr: errors.New("write failed")}, wantCode: ws.ErrorCodeInternalError},
		} {
			t.Run(tc.name, func(t *testing.T) {
				response := dispatchGitPushDismissal(t, tc.repo, tc.ctx, map[string]string{"message_id": "push-failure"})
				if response.Type != ws.MessageTypeError {
					t.Fatalf("response type = %q, want error", response.Type)
				}
				var payload ws.ErrorPayload
				if err := json.Unmarshal(response.Payload, &payload); err != nil {
					t.Fatal(err)
				}
				if payload.Code != tc.wantCode {
					t.Fatalf("error code = %q, want %q", payload.Code, tc.wantCode)
				}
				if len(payload.Details) != 0 || payload.Message == "private diagnostics" || string(response.Payload) == "" {
					t.Fatalf("error response exposed diagnostics: %s", response.Payload)
				}
			})
		}
	})
}
