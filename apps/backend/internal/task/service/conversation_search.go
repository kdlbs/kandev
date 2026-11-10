package service

import (
	"context"
	"sync"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

type conversationSearcher interface {
	SearchTaskConversations(context.Context, string, string, int) (*models.ConversationSearchPage, error)
}

type conversationAdmission struct {
	sync.Mutex
	active map[string]bool
}

var conversationQueries = conversationAdmission{active: make(map[string]bool)}

func (g *conversationAdmission) acquire(workspace string) bool {
	g.Lock()
	defer g.Unlock()
	if g.active[workspace] || len(g.active) >= 4 {
		return false
	}
	g.active[workspace] = true
	return true
}
func (g *conversationAdmission) release(workspace string) {
	g.Lock()
	delete(g.active, workspace)
	g.Unlock()
}

// SearchTaskConversations bounds optional text discovery independently of metadata search.
func (s *Service) SearchTaskConversations(ctx context.Context, workspace, query string, limit int) ([]*models.Task, *models.ConversationSearchPage, error) {
	ctx, cancel := context.WithTimeout(ctx, 750*time.Millisecond)
	defer cancel()
	if err := s.authorizeWorkspaceID(ctx, workspace); err != nil {
		return nil, nil, err
	}
	page := &models.ConversationSearchPage{Hits: []models.ConversationSearchHit{}, State: models.ConversationSearchUnavailable}
	search, ok := s.tasks.(conversationSearcher)
	if !ok {
		return nil, page, nil
	}
	if !conversationQueries.acquire(workspace) {
		page.State = models.ConversationSearchLimited
		return nil, page, nil
	}
	defer conversationQueries.release(workspace)
	result, err := search.SearchTaskConversations(ctx, workspace, query, limit)
	if err != nil {
		if ctx.Err() != nil {
			page.State = models.ConversationSearchLimited
		}
		return nil, page, nil
	}
	if len(result.Hits) == 0 {
		return nil, result, nil
	}
	ids := make([]string, 0, len(result.Hits))
	for _, hit := range result.Hits {
		ids = append(ids, hit.TaskID)
	}
	tasks, err := s.tasks.GetTasksByIDs(ctx, ids)
	if err != nil {
		return nil, nil, err
	}
	// Recheck ownership after the candidate read in case a task moved workspaces.
	visible := tasks[:0]
	for _, task := range tasks {
		if task.WorkspaceID == workspace {
			visible = append(visible, task)
		}
	}
	return visible, result, nil
}
