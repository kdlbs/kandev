package orchestrator

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

type committedOrphanFaultService struct {
	*repoTurnService
	svc       *Service
	completed bool
	successor string
}

func (r *committedOrphanFaultService) GetActiveTurn(ctx context.Context, sessionID string) (*models.Turn, error) {
	if r.completed {
		return nil, errors.New("post-commit read failed")
	}
	return r.repoTurnService.GetActiveTurn(ctx, sessionID)
}
func (r *committedOrphanFaultService) CompleteTurn(ctx context.Context, id string) error {
	if err := r.repoTurnService.CompleteTurn(ctx, id); err != nil {
		return err
	}
	r.completed = true
	if r.successor != "" {
		r.svc.activeTurns.Store("s1", r.successor)
	}
	return nil
}
func TestCommittedOrphanClearsOnlyItsCacheDespiteReadFailure(t *testing.T) {
	for _, successor := range []string{"", "successor"} {
		name := successor
		if name == "" {
			name = "no_successor"
		}
		t.Run(name, func(t *testing.T) {
			f := newCancelTurnFixture(t)
			setFixtureSessionState(t, f, models.TaskSessionStateWaitingForInput)
			f.svc.activeTurns.Store("s1", "turn-1")
			f.svc.turnService = &committedOrphanFaultService{repoTurnService: &repoTurnService{repo: f.repo}, svc: f.svc, successor: successor}
			if err := f.svc.CompleteUnattendedOrphanTurn(t.Context(), "s1", "turn-1"); err == nil {
				t.Fatal("verification failure was hidden")
			}
			if f.activeTurnID(t) != "" {
				t.Fatal("completion did not commit")
			}
			cached, ok := f.svc.activeTurns.Load("s1")
			if successor == "" && ok {
				t.Fatalf("completed cached turn survived: %v", cached)
			}
			if successor != "" && (!ok || cached != successor) {
				t.Fatal("successor cache was removed")
			}
		})
	}
}
