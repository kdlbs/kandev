package service

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	taskrepo "github.com/kandev/kandev/internal/task/repository"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

type conversationSourceMessageRepository struct {
	taskrepo.MessageRepository
	taskrepo.ConversationSourceRepository
	turnCalls int
}

func (r *conversationSourceMessageRepository) ReadConversationTurnsPage(
	ctx context.Context,
	req models.ConversationTurnPageRequest,
) (models.ConversationTurnPage, error) {
	r.turnCalls++
	return r.ConversationSourceRepository.ReadConversationTurnsPage(ctx, req)
}

type conversationMessageOnlyRepository struct {
	taskrepo.MessageRepository
}

func TestConversationSourceServiceUsesMessageRepositoryForEveryRead(t *testing.T) {
	source := &conversationSourceMessageRepository{
		ConversationSourceRepository: conversationSourceStub{
			revision: models.ConversationRevision{SessionID: "session-1", Exists: true, Revision: 7},
			messages: models.ConversationMessagePage{Revision: 7},
			turns:    models.ConversationTurnPage{Revision: 7},
		},
	}
	svc, _, _ := createTestService(t)
	svc.messages = source

	ctx := context.Background()
	if _, err := svc.ReadConversationRevision(ctx, "session-1"); err != nil {
		t.Fatalf("ReadConversationRevision: %v", err)
	}
	if _, err := svc.ReadConversationMessagesPage(ctx, models.ConversationMessagePageRequest{SessionID: "session-1"}); err != nil {
		t.Fatalf("ReadConversationMessagesPage: %v", err)
	}
	if _, err := svc.ReadConversationTurnsPage(ctx, models.ConversationTurnPageRequest{SessionID: "session-1"}); err != nil {
		t.Fatalf("ReadConversationTurnsPage: %v", err)
	}
	if source.turnCalls != 1 {
		t.Fatalf("turn source calls = %d, want 1", source.turnCalls)
	}
}

func TestConversationSourceServiceAuthorizesAllReadsBeforeDispatch(t *testing.T) {
	svc, _, repo := createTestService(t)
	seedScopedWorkspaces(t, repo)
	source := &conversationSourceMessageRepository{
		ConversationSourceRepository: conversationSourceStub{},
	}
	svc.messages = source
	ctx := ctxAs("user-a")

	checks := []struct {
		name string
		call func() error
	}{
		{
			name: "revision",
			call: func() error {
				_, err := svc.ReadConversationRevision(ctx, "sess-b")
				return err
			},
		},
		{
			name: "messages",
			call: func() error {
				_, err := svc.ReadConversationMessagesPage(ctx, models.ConversationMessagePageRequest{SessionID: "sess-b"})
				return err
			},
		},
		{
			name: "turns",
			call: func() error {
				_, err := svc.ReadConversationTurnsPage(ctx, models.ConversationTurnPageRequest{SessionID: "sess-b"})
				return err
			},
		},
	}
	for _, check := range checks {
		t.Run(check.name, func(t *testing.T) {
			if err := check.call(); !errors.Is(err, repoerrors.ErrTaskNotFound) {
				t.Fatalf("error = %v, want ErrTaskNotFound", err)
			}
		})
	}
	if source.turnCalls != 0 {
		t.Fatalf("source turns calls = %d after denied reads, want 0", source.turnCalls)
	}
}

func TestConversationSourceServiceReportsUnavailableMessageRepository(t *testing.T) {
	svc, _, _ := createTestService(t)
	svc.messages = conversationMessageOnlyRepository{}
	ctx := context.Background()

	if _, err := svc.ReadConversationRevision(ctx, "session-1"); !errors.Is(err, errConversationSourceUnavailable) {
		t.Fatalf("revision error = %v, want source unavailable", err)
	}
	if _, err := svc.ReadConversationMessagesPage(ctx, models.ConversationMessagePageRequest{SessionID: "session-1"}); !errors.Is(err, errConversationSourceUnavailable) {
		t.Fatalf("message error = %v, want source unavailable", err)
	}
	if _, err := svc.ReadConversationTurnsPage(ctx, models.ConversationTurnPageRequest{SessionID: "session-1"}); !errors.Is(err, errConversationSourceUnavailable) {
		t.Fatalf("turn error = %v, want source unavailable", err)
	}
}

type conversationSourceStub struct {
	revision models.ConversationRevision
	messages models.ConversationMessagePage
	turns    models.ConversationTurnPage
}

func (s conversationSourceStub) ReadConversationRevision(context.Context, string) (models.ConversationRevision, error) {
	return s.revision, nil
}

func (s conversationSourceStub) ReadConversationMessagesPage(context.Context, models.ConversationMessagePageRequest) (models.ConversationMessagePage, error) {
	return s.messages, nil
}

func (s conversationSourceStub) ReadConversationTurnsPage(context.Context, models.ConversationTurnPageRequest) (models.ConversationTurnPage, error) {
	return s.turns, nil
}
