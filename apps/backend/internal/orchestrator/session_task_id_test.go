package orchestrator

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/models"
)

// sessionTaskIDStubRepo satisfies sessionExecutorStore with only GetTaskSession
// usable — SessionTaskID calls nothing else, so the embedded nil interface is
// never touched (same pattern as pairStubRepo).
type sessionTaskIDStubRepo struct {
	sessionExecutorStore
	session *models.TaskSession
	err     error
}

func (r *sessionTaskIDStubRepo) GetTaskSession(context.Context, string) (*models.TaskSession, error) {
	return r.session, r.err
}

func TestSessionTaskIDResolvesOwningTask(t *testing.T) {
	s := &Service{
		logger: logger.Default(),
		repo: &sessionTaskIDStubRepo{
			session: &models.TaskSession{ID: "s1", TaskID: "task-1"},
		},
	}
	got, err := s.SessionTaskID(context.Background(), "s1")
	if err != nil {
		t.Fatalf("SessionTaskID: %v", err)
	}
	if got != "task-1" {
		t.Fatalf("SessionTaskID = %q, want %q", got, "task-1")
	}
}

func TestSessionTaskIDEmptyForUnknownSession(t *testing.T) {
	s := &Service{logger: logger.Default(), repo: &sessionTaskIDStubRepo{session: nil}}
	got, err := s.SessionTaskID(context.Background(), "ghost")
	if err != nil {
		t.Fatalf("SessionTaskID: %v", err)
	}
	if got != "" {
		t.Fatalf("SessionTaskID = %q, want empty for unknown session", got)
	}
}

func TestSessionTaskIDEmptyForEmptyInput(t *testing.T) {
	s := &Service{
		logger: logger.Default(),
		repo: &sessionTaskIDStubRepo{
			session: &models.TaskSession{ID: "s1", TaskID: "task-1"},
		},
	}
	got, err := s.SessionTaskID(context.Background(), "")
	if err != nil {
		t.Fatalf("SessionTaskID: %v", err)
	}
	if got != "" {
		t.Fatalf("SessionTaskID = %q, want empty for empty session id", got)
	}
}

func TestSessionTaskIDPropagatesRepoError(t *testing.T) {
	repoErr := errors.New("db down")
	s := &Service{logger: logger.Default(), repo: &sessionTaskIDStubRepo{err: repoErr}}
	if _, err := s.SessionTaskID(context.Background(), "s1"); !errors.Is(err, repoErr) {
		t.Fatalf("SessionTaskID error = %v, want %v", err, repoErr)
	}
}
