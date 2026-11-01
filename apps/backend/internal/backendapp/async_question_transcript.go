package backendapp

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/questions"
	taskservice "github.com/kandev/kandev/internal/task/service"
)

const asyncQuestionTranscriptMessagePrefix = "async-question-"

type asyncQuestionTranscriptMessageReader interface {
	GetMessage(ctx context.Context, id string) (*models.Message, error)
}

type asyncQuestionTranscriptMessageCreator interface {
	CreateMessageWithID(ctx context.Context, id string, req *taskservice.CreateMessageRequest) (*models.Message, error)
}

type asyncQuestionTranscriptWriter struct {
	reader  asyncQuestionTranscriptMessageReader
	creator asyncQuestionTranscriptMessageCreator
}

func (w *asyncQuestionTranscriptWriter) WriteAsyncQuestionTranscriptEntry(
	ctx context.Context,
	delivery *models.AsyncQuestionDelivery,
	question *models.AsyncQuestion,
) error {
	if w == nil || w.reader == nil || w.creator == nil || delivery == nil || question == nil {
		return errors.New("async question transcript dependencies are incomplete")
	}
	messageID := asyncQuestionTranscriptMessagePrefix + question.ID
	if existing, err := w.reader.GetMessage(ctx, messageID); err == nil {
		return verifyAsyncQuestionTranscript(existing, delivery, question)
	} else if !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("read async question transcript entry: %w", err)
	}
	request := &taskservice.CreateMessageRequest{
		TaskSessionID: question.SessionID,
		TaskID:        question.TaskID,
		TurnID:        question.TurnID,
		Content:       delivery.Content,
		AuthorType:    string(models.MessageAuthorAgent),
		Type:          string(models.MessageTypeAgentQuestionRequest),
		Metadata: map[string]interface{}{
			questions.MetadataAsyncQuestionID:   question.ID,
			questions.MetadataAsyncQuestionKind: string(models.AsyncQuestionDeliveryTranscript),
			"async_question_request_digest":     question.PayloadDigest,
		},
	}
	if _, err := w.creator.CreateMessageWithID(ctx, messageID, request); err == nil {
		return nil
	} else if existing, readErr := w.reader.GetMessage(ctx, messageID); readErr == nil {
		return verifyAsyncQuestionTranscript(existing, delivery, question)
	} else {
		return fmt.Errorf("create async question transcript entry: %w", err)
	}
}

func verifyAsyncQuestionTranscript(
	message *models.Message,
	delivery *models.AsyncQuestionDelivery,
	question *models.AsyncQuestion,
) error {
	if message == nil || message.ID != asyncQuestionTranscriptMessagePrefix+question.ID ||
		message.TaskID != question.TaskID || message.TaskSessionID != question.SessionID ||
		message.TurnID != question.TurnID || message.AuthorType != models.MessageAuthorAgent ||
		message.Type != models.MessageTypeAgentQuestionRequest || message.Content != delivery.Content ||
		message.Metadata[questions.MetadataAsyncQuestionID] != question.ID ||
		message.Metadata["async_question_request_digest"] != question.PayloadDigest {
		return errors.New("async question transcript identity conflicts with the request")
	}
	return nil
}
