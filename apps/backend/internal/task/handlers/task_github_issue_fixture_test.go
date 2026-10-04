package handlers

import (
	"context"
	"errors"
	"github.com/kandev/kandev/internal/task/models"
)

func (*mockRepository) UpdateTaskGitHubIssue(context.Context, string, *models.TaskGitHubIssueLink) (*models.Task, error) {
	return nil, errors.New("issue mutations are not supported by this test repository")
}
