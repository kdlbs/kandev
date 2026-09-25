package service

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
)

type gitPushErrorMessageDismisser interface {
	DismissGitPushErrorMessage(context.Context, string) (string, error)
}

type dismissalTrackingMessageRepository struct {
	repository.MessageRepository
	writes   int
	writeErr error
	message  *models.Message
}

func (r *dismissalTrackingMessageRepository) GetMessage(ctx context.Context, id string) (*models.Message, error) {
	if r.message != nil && r.message.ID == id {
		return r.message, nil
	}
	return r.MessageRepository.GetMessage(ctx, id)
}

func (r *dismissalTrackingMessageRepository) UpdateMessage(ctx context.Context, message *models.Message) error {
	r.writes++
	if r.writeErr != nil {
		return r.writeErr
	}
	return r.MessageRepository.UpdateMessage(ctx, message)
}

func TestDismissGitPushErrorMessage(t *testing.T) {
	t.Run("persists one marker, retains diagnostics, and is idempotent", func(t *testing.T) {
		svc, eventBus, repo := createTestService(t)
		ctx := context.Background()
		setupTestTask(t, repo)
		sessionID := setupTestSession(t, repo)
		turnID := setupTestTurn(t, repo, sessionID, "task-123", "turn-push-failure")
		message := &models.Message{
			ID: "push-failure", TaskID: "task-123", TaskSessionID: sessionID, TurnID: turnID,
			AuthorType: models.MessageAuthorAgent, Type: models.MessageTypeError,
			Content: "Git push failed: remote rejected the branch",
			Metadata: map[string]any{
				"git_operation_error": true,
				"operation":           "push",
				"error_output":        "remote rejected the branch",
				"actions":             []any{map[string]any{"label": "Fix", "type": "ws_request"}},
				"kept_diagnostic":     "preserve me",
			},
		}
		if err := repo.CreateMessage(ctx, message); err != nil {
			t.Fatal(err)
		}
		tracking := &dismissalTrackingMessageRepository{MessageRepository: repo}
		svc.messages = tracking
		eventBus.ClearEvents()
		dismisser, ok := any(svc).(gitPushErrorMessageDismisser)
		if !ok {
			t.Fatal("Service must support message-scoped Git push error dismissal")
		}

		dismissedAt, err := dismisser.DismissGitPushErrorMessage(ctx, message.ID)
		if err != nil {
			t.Fatalf("dismiss eligible message: %v", err)
		}
		parsed, err := time.Parse(time.RFC3339Nano, dismissedAt)
		if err != nil || parsed.Location() != time.UTC {
			t.Fatalf("dismissed_at = %q, parse err = %v, want UTC RFC3339Nano", dismissedAt, err)
		}
		stored, err := repo.GetMessage(ctx, message.ID)
		if err != nil {
			t.Fatal(err)
		}
		if stored.Metadata["git_operation_error_dismissed_at"] != dismissedAt {
			t.Fatalf("stored dismissal marker = %#v, want %q", stored.Metadata["git_operation_error_dismissed_at"], dismissedAt)
		}
		if stored.Content != message.Content || stored.Metadata["error_output"] != "remote rejected the branch" || stored.Metadata["kept_diagnostic"] != "preserve me" {
			t.Fatalf("dismissal changed diagnostic data: %+v", stored)
		}
		if len(stored.Metadata) != len(message.Metadata)+1 {
			t.Fatalf("stored metadata keys = %d, want original keys plus dismissal marker: %#v", len(stored.Metadata), stored.Metadata)
		}
		for key, want := range message.Metadata {
			if got := stored.Metadata[key]; !reflect.DeepEqual(got, want) {
				t.Errorf("stored metadata[%q] = %#v, want original value %#v", key, got, want)
			}
		}
		if len(eventBus.GetPublishedEvents()) != 1 || countEvents(eventBus.GetPublishedEvents(), events.MessageUpdated) != 1 {
			t.Fatalf("published events = %#v, want one message update", eventBus.GetPublishedEvents())
		}

		repeatedAt, err := dismisser.DismissGitPushErrorMessage(ctx, message.ID)
		if err != nil || repeatedAt != dismissedAt {
			t.Fatalf("repeated dismissal = %q, %v; want original timestamp %q", repeatedAt, err, dismissedAt)
		}
		if tracking.writes != 1 || countEvents(eventBus.GetPublishedEvents(), events.MessageUpdated) != 1 {
			t.Fatalf("repeated dismissal writes/events = %d/%d, want 1/1", tracking.writes, countEvents(eventBus.GetPublishedEvents(), events.MessageUpdated))
		}
	})

	t.Run("rejects sessionless and ineligible rows without updates", func(t *testing.T) {
		cases := []struct {
			name    string
			session bool
			typ     models.MessageType
			marker  any
			op      string
		}{
			{name: "sessionless", typ: models.MessageTypeError, marker: true, op: "push"},
			{name: "wrong message type", session: true, typ: models.MessageTypeMessage, marker: true, op: "push"},
			{name: "wrong operation", session: true, typ: models.MessageTypeError, marker: true, op: "pull"},
			{name: "missing marker", session: true, typ: models.MessageTypeError, marker: false, op: "push"},
			{name: "malformed marker", session: true, typ: models.MessageTypeError, marker: "true", op: "push"},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				svc, eventBus, repo := createTestService(t)
				ctx := context.Background()
				setupTestTask(t, repo)
				sessionID := ""
				turnID := ""
				if tc.session {
					sessionID = setupTestSession(t, repo)
					turnID = setupTestTurn(t, repo, sessionID, "task-123", "turn-ineligible")
				}
				message := &models.Message{
					ID: "ineligible", TaskID: "task-123", TaskSessionID: sessionID, TurnID: turnID,
					AuthorType: models.MessageAuthorAgent, Type: tc.typ,
					Metadata: map[string]any{"git_operation_error": tc.marker, "operation": tc.op},
				}
				tracking := &dismissalTrackingMessageRepository{MessageRepository: repo}
				if tc.session {
					if err := repo.CreateMessage(ctx, message); err != nil {
						t.Fatal(err)
					}
				} else {
					tracking.message = message
				}
				svc.messages = tracking
				eventBus.ClearEvents()
				dismisser, ok := any(svc).(gitPushErrorMessageDismisser)
				if !ok {
					t.Fatal("Service must support message-scoped Git push error dismissal")
				}
				if _, err := dismisser.DismissGitPushErrorMessage(ctx, message.ID); err == nil {
					t.Fatal("dismissal succeeded for an ineligible message")
				}
				stored, err := tracking.GetMessage(ctx, message.ID)
				if err != nil {
					t.Fatal(err)
				}
				if _, exists := stored.Metadata["git_operation_error_dismissed_at"]; exists {
					t.Fatalf("rejected message gained dismissal metadata: %#v", stored.Metadata)
				}
				if tracking.writes != 0 || len(eventBus.GetPublishedEvents()) != 0 {
					t.Fatalf("rejected message writes/events = %d/%d, want 0/0", tracking.writes, len(eventBus.GetPublishedEvents()))
				}
			})
		}
	})

	t.Run("denies a cross-workspace reader without updating the message", func(t *testing.T) {
		svc, eventBus, repo := createTestService(t)
		ctx := context.Background()
		seedScopedWorkspaces(t, repo)
		message := &models.Message{
			ID: "foreign-push-failure", TaskID: "task-b", TaskSessionID: "sess-b",
			TurnID:     setupTestTurn(t, repo, "sess-b", "task-b", "turn-foreign-push"),
			AuthorType: models.MessageAuthorAgent, Type: models.MessageTypeError,
			Metadata: map[string]any{"git_operation_error": true, "operation": "push"},
		}
		if err := repo.CreateMessage(ctx, message); err != nil {
			t.Fatal(err)
		}
		tracking := &dismissalTrackingMessageRepository{MessageRepository: repo}
		svc.messages = tracking
		eventBus.ClearEvents()
		dismisser, ok := any(svc).(gitPushErrorMessageDismisser)
		if !ok {
			t.Fatal("Service must support message-scoped Git push error dismissal")
		}
		if _, err := dismisser.DismissGitPushErrorMessage(ctxAs("user-a"), message.ID); err == nil {
			t.Fatal("cross-workspace dismissal was allowed")
		}
		if tracking.writes != 0 || len(eventBus.GetPublishedEvents()) != 0 {
			t.Fatalf("denied dismissal writes/events = %d/%d, want 0/0", tracking.writes, len(eventBus.GetPublishedEvents()))
		}
	})

	t.Run("leaves the row unchanged when persistence fails", func(t *testing.T) {
		svc, eventBus, repo := createTestService(t)
		ctx := context.Background()
		setupTestTask(t, repo)
		sessionID := setupTestSession(t, repo)
		turnID := setupTestTurn(t, repo, sessionID, "task-123", "turn-write-failure")
		message := &models.Message{
			ID: "write-failure", TaskID: "task-123", TaskSessionID: sessionID, TurnID: turnID,
			AuthorType: models.MessageAuthorAgent, Type: models.MessageTypeError,
			Metadata: map[string]any{"git_operation_error": true, "operation": "push", "error_output": "diagnostics"},
		}
		if err := repo.CreateMessage(ctx, message); err != nil {
			t.Fatal(err)
		}
		tracking := &dismissalTrackingMessageRepository{MessageRepository: repo, writeErr: errors.New("write failed")}
		svc.messages = tracking
		eventBus.ClearEvents()
		dismisser, ok := any(svc).(gitPushErrorMessageDismisser)
		if !ok {
			t.Fatal("Service must support message-scoped Git push error dismissal")
		}
		if _, err := dismisser.DismissGitPushErrorMessage(ctx, message.ID); err == nil {
			t.Fatal("persistence failure was not returned")
		}
		stored, err := repo.GetMessage(ctx, message.ID)
		if err != nil {
			t.Fatal(err)
		}
		if _, exists := stored.Metadata["git_operation_error_dismissed_at"]; exists || stored.Metadata["error_output"] != "diagnostics" {
			t.Fatalf("persistence failure changed message: %#v", stored.Metadata)
		}
		if tracking.writes != 1 || len(eventBus.GetPublishedEvents()) != 0 {
			t.Fatalf("failed write count/events = %d/%d, want 1/0", tracking.writes, len(eventBus.GetPublishedEvents()))
		}
	})
}
