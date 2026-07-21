package mentions

import (
	"context"
	"fmt"
	"net/url"

	"github.com/kandev/kandev/internal/task/models"
)

const taskMentionSource = "kandev_tasks"

// TaskMentionSearcher is the lightweight task lookup consumed by the Kandev provider.
type TaskMentionSearcher interface {
	SearchMentionTasks(
		ctx context.Context,
		workspaceID, query, excludeTaskID string,
		limit int,
	) ([]*models.Task, error)
	GetTask(ctx context.Context, id string) (*models.Task, error)
}

type taskProvider struct {
	searcher TaskMentionSearcher
}

// NewTaskProvider creates the built-in Kandev task mention source.
func NewTaskProvider(searcher TaskMentionSearcher) MentionProvider {
	return &taskProvider{searcher: searcher}
}

func (p *taskProvider) Descriptor() ProviderDescriptor {
	return ProviderDescriptor{
		Source:      taskMentionSource,
		Provider:    mentionProviderKandev,
		Kind:        mentionKindTask,
		DisplayName: "Kandev tasks",
		KindLabel:   mentionLabelTask,
	}
}

func (p *taskProvider) Search(ctx context.Context, request SearchRequest) ([]Candidate, error) {
	if p.searcher == nil {
		return nil, NewProviderError(StatusNotConfigured)
	}
	tasks, err := p.searcher.SearchMentionTasks(
		ctx,
		request.WorkspaceID,
		request.Query,
		request.ExcludeTaskID,
		request.Limit,
	)
	if err != nil {
		return nil, fmt.Errorf("search Kandev task mentions: %w", err)
	}
	candidates := make([]Candidate, 0, len(tasks))
	for _, task := range tasks {
		if task == nil || task.WorkspaceID != request.WorkspaceID {
			continue
		}
		candidates = append(candidates, Candidate{
			ID:    task.ID,
			Key:   task.Identifier,
			Title: task.Title,
			URL:   "/t/" + url.PathEscape(task.ID),
			Scope: task.WorkspaceID,
		})
	}
	return candidates, nil
}

func (p *taskProvider) AuthorizeReference(ctx context.Context, request ReferenceAuthorizationRequest) error {
	reference := request.Reference
	if request.WorkspaceID == "" || reference.Version != 1 || reference.Provider != mentionProviderKandev ||
		reference.Kind != mentionKindTask || reference.Scope != request.WorkspaceID ||
		reference.Ref != canonicalRef(mentionProviderKandev, mentionKindTask, reference.Scope, reference.ID) ||
		reference.URL != "/t/"+url.PathEscape(reference.ID) {
		return ErrReferenceUnauthorized
	}
	if request.Purpose == ReferencePurposeSearch {
		return nil
	}
	if request.Purpose != ReferencePurposeSubmission || p.searcher == nil {
		return ErrReferenceUnauthorized
	}
	task, err := p.searcher.GetTask(ctx, reference.ID)
	if err != nil || task == nil || task.WorkspaceID != request.WorkspaceID || task.IsEphemeral {
		return ErrReferenceUnauthorized
	}
	return nil
}
