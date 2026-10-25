package linear

import (
	"context"
	"errors"
)

var ErrPluginWritebackUnsupported = errors.New("linear: linked issue writeback is unsupported by this client")

type pluginCommentClient interface {
	AddComment(context.Context, string, string) (string, error)
}

// GetIssueForWorkspace reads one issue with the credentials owned by workspaceID.
func (s *Service) GetIssueForWorkspace(ctx context.Context, workspaceID, identifier string) (*LinearIssue, error) {
	client, err := s.clientFor(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	return client.GetIssue(ctx, identifier)
}

// AddCommentForWorkspace writes one comment through the workspace-owned client.
func (s *Service) AddCommentForWorkspace(ctx context.Context, workspaceID, issueID, body string) (string, error) {
	client, err := s.clientFor(ctx, workspaceID)
	if err != nil {
		return "", err
	}
	commenter, ok := client.(pluginCommentClient)
	if !ok {
		return "", ErrPluginWritebackUnsupported
	}
	return commenter.AddComment(ctx, issueID, body)
}

// SetIssueStateForWorkspace sets an issue state using workspace-owned credentials.
func (s *Service) SetIssueStateForWorkspace(ctx context.Context, workspaceID, issueID, stateID string) error {
	client, err := s.clientFor(ctx, workspaceID)
	if err != nil {
		return err
	}
	return client.SetIssueState(ctx, issueID, stateID)
}
