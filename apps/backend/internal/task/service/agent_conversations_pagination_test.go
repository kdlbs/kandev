package service

import (
	"context"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/pkg/pluginsdk"
)

// acPagingTaskRepo is a task repo fake that honours page/pageSize the way the
// real SQLite repository does, so the managed-conversation lookup is exercised
// against a workspace holding more ephemeral tasks than one page.
type acPagingTaskRepo struct {
	mu    sync.Mutex
	tasks []*models.Task
}

func (f *acPagingTaskRepo) GetWorkspace(_ context.Context, id string) (*models.Workspace, error) {
	profileID := "profile-workspace-default"
	return &models.Workspace{ID: id, DefaultAgentProfileID: &profileID}, nil
}

func (f *acPagingTaskRepo) ListTasksByWorkspace(_ context.Context, workspaceID, _, _, _ string, page, pageSize int, _ string, _, includeEphemeral, onlyEphemeral, _ bool) ([]*models.Task, int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var filtered []*models.Task
	for _, t := range f.tasks {
		if t.WorkspaceID != workspaceID {
			continue
		}
		if onlyEphemeral && !t.IsEphemeral {
			continue
		}
		if !includeEphemeral && t.IsEphemeral {
			continue
		}
		filtered = append(filtered, t)
	}
	// The real repository defaults to updated_at DESC, so the most recently
	// touched ephemeral tasks fill the first pages and an older managed
	// conversation drifts onto a later one.
	sort.SliceStable(filtered, func(i, j int) bool {
		return filtered[i].UpdatedAt.After(filtered[j].UpdatedAt)
	})
	total := len(filtered)
	offset := (page - 1) * pageSize
	if offset >= total {
		return nil, total, nil
	}
	end := offset + pageSize
	if end > total {
		end = total
	}
	return filtered[offset:end], total, nil
}

func (f *acPagingTaskRepo) ListEphemeralTasksAllWorkspaces(_ context.Context) ([]*models.Task, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var filtered []*models.Task
	for _, t := range f.tasks {
		if t.IsEphemeral {
			filtered = append(filtered, t)
		}
	}
	return filtered, nil
}

func (f *acPagingTaskRepo) CreateTask(_ context.Context, task *models.Task) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.tasks = append(f.tasks, task)
	return nil
}

func (f *acPagingTaskRepo) DeleteTask(_ context.Context, taskID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i, t := range f.tasks {
		if t.ID == taskID {
			f.tasks = append(f.tasks[:i], f.tasks[i+1:]...)
			return nil
		}
	}
	return nil
}

// fillWithOtherEphemeralTasks appends n unrelated ephemeral tasks (quick
// chats, automation runs) so the managed conversation is pushed off the first
// page of the workspace listing.
func fillWithOtherEphemeralTasks(repo *acPagingTaskRepo, workspaceID string, n int) {
	base := time.Now().UTC().Add(time.Hour)
	for i := 0; i < n; i++ {
		repo.tasks = append(repo.tasks, &models.Task{
			ID:          uuid.New().String(),
			WorkspaceID: workspaceID,
			IsEphemeral: true,
			UpdatedAt:   base.Add(time.Duration(i) * time.Second),
		})
	}
}

func newACPagingService(repo *acPagingTaskRepo) (*AgentConversationService, *acFakeDispatcher) {
	dispatcher := newACFakeDispatcher()
	svc := NewAgentConversationService(repo, newACFakeSessionRepo(), newACFakeProfileRepo(), newACFakeStateRepo(), &acFakeEventBus{})
	svc.SetDispatcher(dispatcher)
	return svc, dispatcher
}

// A workspace that accumulated more ephemeral tasks than one page must still
// resolve its existing managed conversation: a first-page-only scan reports
// "not found" and Ensure silently creates a second hidden conversation,
// splitting the coordinator's history.
func TestEnsureFindsConversationBeyondFirstPage(t *testing.T) {
	repo := &acPagingTaskRepo{}
	svc, _ := newACPagingService(repo)
	spec := pluginsdk.AgentConversationSpec{WorkspaceID: "ws-1", ConversationKey: "coordinator"}

	first, statusStr, err := svc.Ensure(context.Background(), "plugin-coordinator", spec)
	if err != nil {
		t.Fatalf("first Ensure: %v", err)
	}
	if statusStr != AgentConversationStatusCreated {
		t.Fatalf("first Ensure status = %q, want %q", statusStr, AgentConversationStatusCreated)
	}

	fillWithOtherEphemeralTasks(repo, "ws-1", managedConversationPageSize*2)

	second, statusStr, err := svc.Ensure(context.Background(), "plugin-coordinator", spec)
	if err != nil {
		t.Fatalf("second Ensure: %v", err)
	}
	if statusStr != AgentConversationStatusExists {
		t.Fatalf("second Ensure status = %q, want %q", statusStr, AgentConversationStatusExists)
	}
	if second.TaskID != first.TaskID {
		t.Fatalf("second Ensure returned a different conversation: %q vs %q", second.TaskID, first.TaskID)
	}
	if got := len(repo.tasks); got != 1+managedConversationPageSize*2 {
		t.Fatalf("expected no duplicate conversation task, got %d tasks", got)
	}
}

// Dispatch must reach the same conversation once it is past the first page,
// rather than reporting NotFound after the scheduler already claimed the
// occurrence key.
func TestDispatchFindsConversationBeyondFirstPage(t *testing.T) {
	repo := &acPagingTaskRepo{}
	svc, dispatcher := newACPagingService(repo)
	spec := pluginsdk.AgentConversationSpec{WorkspaceID: "ws-1", ConversationKey: "coordinator"}

	if _, _, err := svc.Ensure(context.Background(), "plugin-coordinator", spec); err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	fillWithOtherEphemeralTasks(repo, "ws-1", managedConversationPageSize*2)

	result, err := svc.Dispatch(context.Background(), "plugin-coordinator", "ws-1", "coordinator", "WAKE:CYCLE", "occ-1")
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if result.Status == "" || result.SessionID == "" {
		t.Fatalf("Dispatch returned empty result: %+v", result)
	}
	if len(dispatcher.calls) != 1 {
		t.Fatalf("expected exactly one delivery, got %d", len(dispatcher.calls))
	}
}

// Delete must sweep every owned conversation, including any that a
// first-page-only scan would leave orphaned after uninstall.
func TestDeleteFindsConversationsBeyondFirstPage(t *testing.T) {
	repo := &acPagingTaskRepo{}
	svc, _ := newACPagingService(repo)
	spec := pluginsdk.AgentConversationSpec{WorkspaceID: "ws-1", ConversationKey: "coordinator"}

	if _, _, err := svc.Ensure(context.Background(), "plugin-coordinator", spec); err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	fillWithOtherEphemeralTasks(repo, "ws-1", managedConversationPageSize*2)

	count, err := svc.Delete(context.Background(), "plugin-coordinator", "ws-1", "coordinator")
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if count != 1 {
		t.Fatalf("Delete removed %d conversations, want 1", count)
	}
}
