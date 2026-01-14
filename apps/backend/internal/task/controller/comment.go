package controller

import (
	"context"

	"github.com/kandev/kandev/internal/task/dto"
	"github.com/kandev/kandev/internal/task/service"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

type CommentController struct {
	service *service.Service
}

func NewCommentController(svc *service.Service) *CommentController {
	return &CommentController{service: svc}
}

func (c *CommentController) ListComments(ctx context.Context, req dto.ListCommentsRequest) (dto.ListCommentsResponse, error) {
	comments, err := c.service.ListComments(ctx, req.TaskID)
	if err != nil {
		return dto.ListCommentsResponse{}, err
	}
	result := make([]*v1.Comment, 0, len(comments))
	for _, comment := range comments {
		result = append(result, comment.ToAPI())
	}
	return dto.ListCommentsResponse{
		Comments: result,
		Total:    len(result),
	}, nil
}

func (c *CommentController) CreateComment(ctx context.Context, req dto.CreateCommentRequest) (*v1.Comment, error) {
	comment, err := c.service.CreateComment(ctx, &service.CreateCommentRequest{
		TaskID:         req.TaskID,
		Content:        req.Content,
		AuthorType:     req.AuthorType,
		AuthorID:       req.AuthorID,
		Type:           req.Type,
		RequestsInput:  req.RequestsInput,
		Metadata:       req.Metadata,
		ACPSessionID:   req.ACPSessionID,
		AgentSessionID: req.AgentSessionID,
	})
	if err != nil {
		return nil, err
	}
	return comment.ToAPI(), nil
}
