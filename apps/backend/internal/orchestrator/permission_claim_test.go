package orchestrator

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

// claimingMessageCreator scripts the durable claim so the test can drive the
// exact race RespondToPermission now arbitrates.
type claimingMessageCreator struct {
	mockMessageCreator

	mu       sync.Mutex
	resolved models.PermissionStatus
	updates  []models.PermissionStatus
}

func (c *claimingMessageCreator) ClaimPermissionResponse(
	_ context.Context, _, _ string, status models.PermissionStatus,
) (bool, models.PermissionStatus, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.resolved != "" {
		return false, c.resolved, nil
	}
	c.resolved = status
	return true, status, nil
}

func (c *claimingMessageCreator) UpdatePermissionMessage(
	_ context.Context, _, _ string, status models.PermissionStatus,
) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.updates = append(c.updates, status)
	return nil
}

// dispatchCounter records every response that would reach the agent.
type dispatchCounter struct {
	mu    sync.Mutex
	calls int
}

func (d *dispatchCounter) dispatch(context.Context) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.calls++
	return nil
}

func (d *dispatchCounter) count() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.calls
}

// TestRespondToPermissionClaimsBeforeDispatch is the regression guard for the
// race two reviewers flagged: without a durable claim, a second responder
// (another tab, or a plugin) passes the same pending check and dispatches
// again. agentctl keeps the pending entry after a response and its response
// channel holds one slot, so the loser either answers a request the agent
// never asked again for, or fails and drives the row to "expired" over the
// winner's real outcome.
func TestRespondToPermissionClaimsBeforeDispatch(t *testing.T) {
	creator := &claimingMessageCreator{}
	exec := &dispatchCounter{}
	// A real (empty) repo: the post-response session refresh looks a session up
	// and tolerates its absence, so the test exercises the arbitration without
	// standing up an executor or an agent manager.
	svc := &Service{logger: testLogger(), messageCreator: creator, repo: setupTestRepo(t)}

	if err := svc.respondToPermission(
		context.Background(), "session-1", "pending-1", "allow", false, false, exec.dispatch,
	); err != nil {
		t.Fatalf("first responder: %v", err)
	}
	if exec.count() != 1 {
		t.Fatalf("dispatches after the winner = %d, want 1", exec.count())
	}

	// A conflicting second responder must be refused before dispatch.
	err := svc.respondToPermission(
		context.Background(), "session-1", "pending-1", "deny", false, true, exec.dispatch,
	)
	var resolved *PermissionAlreadyResolvedError
	if !errors.As(err, &resolved) {
		t.Fatalf("second responder err = %v, want PermissionAlreadyResolvedError", err)
	}
	if resolved.Status != string(models.PermissionStatusApproved) {
		t.Fatalf("conflict reported status %q, want the winner's approved", resolved.Status)
	}
	if exec.count() != 1 {
		t.Fatalf("dispatches after the conflicting loser = %d, want 1", exec.count())
	}
}

// TestRespondToPermissionDuplicateSubmitIsNotAnError keeps the common case
// harmless: a double-click, or a retry of a response that already landed,
// submits the SAME outcome. Reporting that as a failure would tell a user
// their own decision did not take, so it succeeds without dispatching twice.
func TestRespondToPermissionDuplicateSubmitIsNotAnError(t *testing.T) {
	creator := &claimingMessageCreator{}
	exec := &dispatchCounter{}
	// A real (empty) repo: the post-response session refresh looks a session up
	// and tolerates its absence, so the test exercises the arbitration without
	// standing up an executor or an agent manager.
	svc := &Service{logger: testLogger(), messageCreator: creator, repo: setupTestRepo(t)}

	for attempt := 0; attempt < 2; attempt++ {
		if err := svc.respondToPermission(
			context.Background(), "session-1", "pending-1", "allow", false, false, exec.dispatch,
		); err != nil {
			t.Fatalf("attempt %d: %v", attempt+1, err)
		}
	}
	if exec.count() != 1 {
		t.Fatalf("dispatches for a duplicate submit = %d, want 1", exec.count())
	}
}
