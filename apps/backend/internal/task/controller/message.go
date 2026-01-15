package controller

import (
	"context"

	"github.com/kandev/kandev/internal/task/dto"
	"github.com/kandev/kandev/internal/task/service"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

type MessageController struct {
	service *service.Service
}

func NewMessageController(svc *service.Service) *MessageController {
	return &MessageController{service: svc}
}

func (c *MessageController) ListMessages(ctx context.Context, req dto.ListMessagesRequest) (dto.ListMessagesResponse, error) {
	messages, hasMore, err := c.service.ListMessagesPaginated(ctx, service.ListMessagesRequest{
		AgentSessionID: req.AgentSessionID,
		Limit:          req.Limit,
		Before:         req.Before,
		After:          req.After,
		Sort:           req.Sort,
	})
	if err != nil {
		return dto.ListMessagesResponse{}, err
	}
	result := make([]*v1.Message, 0, len(messages))
	for _, message := range messages {
		result = append(result, message.ToAPI())
	}
	cursor := ""
	if len(result) > 0 {
		cursor = result[len(result)-1].ID
	}
	return dto.ListMessagesResponse{
		Messages: result,
		Total:    len(result),
		HasMore:  hasMore,
		Cursor:   cursor,
	}, nil
}

func (c *MessageController) ListAllMessages(ctx context.Context, sessionID string) (dto.ListMessagesResponse, error) {
	messages, err := c.service.ListMessages(ctx, sessionID)
	if err != nil {
		return dto.ListMessagesResponse{}, err
	}
	result := make([]*v1.Message, 0, len(messages))
	for _, message := range messages {
		result = append(result, message.ToAPI())
	}
	return dto.ListMessagesResponse{
		Messages: result,
		Total:    len(result),
		HasMore:  false,
		Cursor:   "",
	}, nil
}

func (c *MessageController) CreateMessage(ctx context.Context, req dto.CreateMessageRequest) (*v1.Message, error) {
	message, err := c.service.CreateMessage(ctx, &service.CreateMessageRequest{
		AgentSessionID: req.AgentSessionID,
		TaskID:         req.TaskID,
		Content:        req.Content,
		AuthorType:     req.AuthorType,
		AuthorID:       req.AuthorID,
		Type:           req.Type,
		RequestsInput:  req.RequestsInput,
		Metadata:       req.Metadata,
	})
	if err != nil {
		return nil, err
	}
	return message.ToAPI(), nil
}
