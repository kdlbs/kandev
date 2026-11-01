package backendapp

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/service"
)

type transcriptMessageStore struct {
	rows             map[string]*models.Message
	createCalls      int
	createErrorAfter bool
}

func (s *transcriptMessageStore) GetMessage(_ context.Context, id string) (*models.Message, error) {
	if message := s.rows[id]; message != nil {
		return message, nil
	}
	return nil, sql.ErrNoRows
}

func (s *transcriptMessageStore) CreateMessageWithID(
	_ context.Context,
	id string,
	request *service.CreateMessageRequest,
) (*models.Message, error) {
	s.createCalls++
	message := &models.Message{
		ID: id, TaskSessionID: request.TaskSessionID, TaskID: request.TaskID,
		TurnID: request.TurnID, Content: request.Content, AuthorType: models.MessageAuthorType(request.AuthorType),
		Type: models.MessageType(request.Type), Metadata: request.Metadata,
	}
	s.rows[id] = message
	if s.createErrorAfter {
		return nil, errors.New("message write receipt was lost")
	}
	return message, nil
}

func TestAsyncQuestionTranscriptEntryUsesStableMessageIdentity(t *testing.T) {
	store := &transcriptMessageStore{rows: make(map[string]*models.Message)}
	writer := &asyncQuestionTranscriptWriter{reader: store, creator: store}
	question := &models.AsyncQuestion{
		ID: "question-transcript", TaskID: "task-transcript", SessionID: "session-transcript",
		TurnID: "turn-transcript", PayloadDigest: "request-digest",
	}
	delivery := &models.AsyncQuestionDelivery{Content: "ASYNC QUESTION REQUEST"}
	ctx := context.Background()

	require.NoError(t, writer.WriteAsyncQuestionTranscriptEntry(ctx, delivery, question))
	require.NoError(t, writer.WriteAsyncQuestionTranscriptEntry(ctx, delivery, question))
	require.Equal(t, 1, store.createCalls)
	message := store.rows[asyncQuestionTranscriptMessagePrefix+question.ID]
	require.Equal(t, models.MessageTypeAgentQuestionRequest, message.Type)
	require.Equal(t, models.MessageAuthorAgent, message.AuthorType)
	require.Equal(t, question.ID, message.Metadata["async_question_id"])
}

func TestAsyncQuestionTranscriptEntryRecoversLostCreateReceipt(t *testing.T) {
	store := &transcriptMessageStore{rows: make(map[string]*models.Message), createErrorAfter: true}
	writer := &asyncQuestionTranscriptWriter{reader: store, creator: store}
	question := &models.AsyncQuestion{
		ID: "question-transcript-race", TaskID: "task-transcript", SessionID: "session-transcript",
		TurnID: "turn-transcript", PayloadDigest: "request-digest",
	}
	err := writer.WriteAsyncQuestionTranscriptEntry(context.Background(), &models.AsyncQuestionDelivery{
		Content: "ASYNC QUESTION REQUEST",
	}, question)
	require.NoError(t, err, "the stored row must resolve an ambiguous create result")
	require.Equal(t, 1, store.createCalls)
}
