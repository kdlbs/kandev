package coordinator

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

type concurrentPrincipalStore struct {
	lookups      atomic.Int32
	lookupsReady chan struct{}
	mu           sync.Mutex
	principal    *models.WorkspaceAgentPrincipal
	rebinds      int
}

func newConcurrentPrincipalStore() *concurrentPrincipalStore {
	return &concurrentPrincipalStore{lookupsReady: make(chan struct{})}
}

func (s *concurrentPrincipalStore) GetActiveWorkspaceAgentPrincipalForTask(context.Context, string, string) (*models.WorkspaceAgentPrincipal, error) {
	return nil, nil
}

func (s *concurrentPrincipalStore) GetWorkspaceAgentPrincipalByContext(context.Context, string, string, string) (*models.WorkspaceAgentPrincipal, error) {
	lookup := s.lookups.Add(1)
	if lookup == 2 {
		close(s.lookupsReady)
	}
	<-s.lookupsReady
	if lookup <= 2 {
		return nil, repoerrors.ErrWorkspaceAgentPrincipalNotFound
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.principal == nil {
		return nil, repoerrors.ErrWorkspaceAgentPrincipalNotFound
	}
	return s.principal, nil
}

func (s *concurrentPrincipalStore) CreateWorkspaceAgentPrincipal(_ context.Context, principal *models.WorkspaceAgentPrincipal) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.principal != nil {
		return repoerrors.ErrWorkspaceAgentPrincipalConflict
	}
	principal.ID = "winner"
	s.principal = principal
	return nil
}

func (s *concurrentPrincipalStore) RebindWorkspaceAgentPrincipal(context.Context, string, string, string, time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rebinds++
	return nil
}

func TestEnsureTaskPrincipalDeniesLoserOfConcurrentAdmission(t *testing.T) {
	store := newConcurrentPrincipalStore()
	results := make(chan error, 2)
	for _, sessionID := range []string{"session-a", "session-b"} {
		go func(sessionID string) {
			_, err := EnsureTaskPrincipal(context.Background(), store, "workspace-1", "task-1", sessionID)
			results <- err
		}(sessionID)
	}

	var successes, conflicts int
	for range 2 {
		if err := <-results; err == nil {
			successes++
		} else if errors.Is(err, repoerrors.ErrWorkspaceAgentPrincipalConflict) {
			conflicts++
		} else {
			t.Fatalf("EnsureTaskPrincipal error = %v, want conflict", err)
		}
	}

	store.mu.Lock()
	defer store.mu.Unlock()
	if successes != 1 || conflicts != 1 {
		t.Fatalf("admission results = successes %d, conflicts %d; want one each", successes, conflicts)
	}
	if store.principal == nil || (store.principal.BackingSessionID != "session-a" && store.principal.BackingSessionID != "session-b") {
		t.Fatalf("winner principal = %#v, want one original session binding", store.principal)
	}
	if store.rebinds != 0 {
		t.Fatalf("rebinds = %d, want no conflict-recovery rebind", store.rebinds)
	}
}

func TestEnsureTaskPrincipalPassesWorkspaceAndTaskToActiveLookup(t *testing.T) {
	store := &recordingPrincipalStore{}
	_, err := EnsureTaskPrincipal(context.Background(), store, "workspace-1", "task-1", "session-1")
	if err != nil {
		t.Fatalf("EnsureTaskPrincipal: %v", err)
	}
	if store.workspaceID != "workspace-1" || store.taskID != "task-1" {
		t.Fatalf("active lookup = (%q, %q), want (workspace-1, task-1)", store.workspaceID, store.taskID)
	}
}

func TestEnsureTaskPrincipalClaimsUnboundPrincipalOnce(t *testing.T) {
	store := &claimingPrincipalStore{principal: &models.WorkspaceAgentPrincipal{
		ID: "principal-1", WorkspaceID: "workspace-1", BackingTaskID: "task-1",
	}}

	principal, err := EnsureTaskPrincipal(context.Background(), store, "workspace-1", "task-1", "session-1")
	if err != nil {
		t.Fatalf("first claim: %v", err)
	}
	if principal == nil || principal.BackingSessionID != "session-1" {
		t.Fatalf("claimed principal = %#v, want session-1", principal)
	}

	if _, err := EnsureTaskPrincipal(context.Background(), store, "workspace-1", "task-1", "session-2"); err == nil {
		t.Fatal("second session claim succeeded, want denial")
	}
	if store.claims != 1 {
		t.Fatalf("claims = %d, want exactly one", store.claims)
	}
}

type claimingPrincipalStore struct {
	principal *models.WorkspaceAgentPrincipal
	claims    int
}

func (s *claimingPrincipalStore) GetActiveWorkspaceAgentPrincipalForTask(context.Context, string, string) (*models.WorkspaceAgentPrincipal, error) {
	return s.principal, nil
}

func (s *claimingPrincipalStore) GetWorkspaceAgentPrincipalByContext(context.Context, string, string, string) (*models.WorkspaceAgentPrincipal, error) {
	return nil, repoerrors.ErrWorkspaceAgentPrincipalNotFound
}

func (s *claimingPrincipalStore) CreateWorkspaceAgentPrincipal(context.Context, *models.WorkspaceAgentPrincipal) error {
	return errors.New("unexpected create")
}

func (s *claimingPrincipalStore) RebindWorkspaceAgentPrincipal(context.Context, string, string, string, time.Time) error {
	return errors.New("unexpected rebind")
}

func (s *claimingPrincipalStore) ClaimWorkspaceAgentPrincipal(_ context.Context, id, taskID, sessionID string, updatedAt time.Time) error {
	if s.principal == nil || s.principal.ID != id || s.principal.BackingTaskID != taskID || s.principal.BackingSessionID != "" {
		return repoerrors.ErrWorkspaceAgentPrincipalConflict
	}
	s.principal.BackingSessionID = sessionID
	s.principal.UpdatedAt = updatedAt
	s.claims++
	return nil
}

type recordingPrincipalStore struct {
	workspaceID string
	taskID      string
}

func (s *recordingPrincipalStore) GetActiveWorkspaceAgentPrincipalForTask(_ context.Context, workspaceID, taskID string) (*models.WorkspaceAgentPrincipal, error) {
	s.workspaceID, s.taskID = workspaceID, taskID
	return &models.WorkspaceAgentPrincipal{ID: "custom", BackingTaskID: taskID, BackingSessionID: "session-1"}, nil
}

func (s *recordingPrincipalStore) GetWorkspaceAgentPrincipalByContext(context.Context, string, string, string) (*models.WorkspaceAgentPrincipal, error) {
	return nil, repoerrors.ErrWorkspaceAgentPrincipalNotFound
}

func (s *recordingPrincipalStore) CreateWorkspaceAgentPrincipal(context.Context, *models.WorkspaceAgentPrincipal) error {
	return errors.New("unexpected create")
}

func (s *recordingPrincipalStore) RebindWorkspaceAgentPrincipal(context.Context, string, string, string, time.Time) error {
	return errors.New("unexpected rebind")
}
