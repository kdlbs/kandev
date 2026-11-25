package orchestrator

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/sqlite"
)

// Keep the real recovery path and full single-session reads, but reject a
// periodic broad scan that would decode every session's retained payloads.
type watchdogNarrowOnlyRepo struct {
	*sqlite.Repository
	t *testing.T
}

func (r watchdogNarrowOnlyRepo) ListActiveTaskSessions(context.Context) ([]*models.TaskSession, error) {
	r.t.Fatal("watchdog used the full active-session reader")
	return nil, nil
}
