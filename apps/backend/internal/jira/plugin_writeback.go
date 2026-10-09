package jira

import (
	"context"
	"errors"
)

var ErrPluginWritebackUnsupported = errors.New("jira: linked issue writeback is unsupported by this client")

type pluginCommentClient interface {
	AddComment(context.Context, string, string) (string, error)
}

// GetTicketForWorkspace reads one issue with the credentials owned by workspaceID.
func (s *Service) GetTicketForWorkspace(ctx context.Context, workspaceID, issueKey string) (*JiraTicket, error) {
	client, err := s.clientFor(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	return client.GetTicket(ctx, issueKey)
}

// AddCommentForWorkspace writes one comment through the workspace-owned client.
func (s *Service) AddCommentForWorkspace(ctx context.Context, workspaceID, issueKey, body string) (string, error) {
	client, err := s.clientFor(ctx, workspaceID)
	if err != nil {
		return "", err
	}
	commenter, ok := client.(pluginCommentClient)
	if !ok {
		return "", ErrPluginWritebackUnsupported
	}
	return commenter.AddComment(ctx, issueKey, body)
}

// DoTransitionForWorkspace applies a previously-observed transition through
// the workspace-owned client.
func (s *Service) DoTransitionForWorkspace(ctx context.Context, workspaceID, issueKey, transitionID string) error {
	client, err := s.clientFor(ctx, workspaceID)
	if err != nil {
		return err
	}
	return client.DoTransition(ctx, issueKey, transitionID)
}
