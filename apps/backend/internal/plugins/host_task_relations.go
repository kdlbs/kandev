package plugins

import (
	"context"
	"errors"

	"github.com/kandev/kandev/internal/task/repository"
	"github.com/kandev/kandev/pkg/pluginsdk"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type taskRelationsReader struct {
	source taskRelationsSource
}

func (r taskRelationsReader) Get(ctx context.Context, workspaceID, taskID string) (*pluginsdk.TaskRelations, error) {
	relations, err := r.source.GetTaskRelations(ctx, workspaceID, taskID)
	if errors.Is(err, repository.ErrTaskNotFound) {
		return nil, status.Error(codes.NotFound, "task relations not found")
	}
	return relations, err
}

type deniedTaskRelationsReader struct{}

func (deniedTaskRelationsReader) Get(context.Context, string, string) (*pluginsdk.TaskRelations, error) {
	return nil, permissionDenied(apiReadCapability(resourceTaskRelations))
}
