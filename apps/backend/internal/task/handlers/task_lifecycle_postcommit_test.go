package handlers

import (
	"context"
	"errors"
	"github.com/gin-gonic/gin"
	"net/http"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/service"
	"github.com/stretchr/testify/require"
)

type postCommitCleanupFailure struct {
	err error
}

func (c *postCommitCleanupFailure) CleanupTaskResources(context.Context, string, bool) {}

func (c *postCommitCleanupFailure) PrepareTaskResourceCleanup(
	context.Context,
	string,
	models.TaskResourceCleanupTrigger,
	string,
	bool,
) error {
	return nil
}

func (c *postCommitCleanupFailure) StartPreparedTaskResourceCleanup(context.Context, string) error {
	return c.err
}

func (c *postCommitCleanupFailure) CancelPreparedTaskResourceCleanup(context.Context, string) error {
	return nil
}

func (r *authzDeleteRepo) ListStructuralChildrenLimited(context.Context, string, int) ([]*models.Task, error) {
	return nil, nil
}

func (r *authzDeleteRepo) ReparentDirectChildrenInWorkspace(context.Context, string, string, string) error {
	return nil
}

func (r *authzDeleteRepo) archivedTaskIDs() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	ids := make([]string, 0, len(r.archived))
	for _, archived := range r.archived {
		if index := strings.IndexByte(archived, '/'); index >= 0 {
			ids = append(ids, archived[:index])
		} else {
			ids = append(ids, archived)
		}
	}
	return ids
}

func TestHTTPTaskLifecycleReturnsPendingAfterPostCommitHousekeepingFailure(t *testing.T) {
	for _, tc := range []struct {
		name string
		call func(*TaskHandlers, *gin.Context)
		want []string
	}{
		{name: "delete", call: (*TaskHandlers).httpDeleteTask, want: []string{"task-b"}},
		{name: "archive", call: (*TaskHandlers).httpArchiveTask, want: []string{"task-b"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &authzDeleteRepo{}
			h := newAuthzTaskHandlers(t, repo)
			handoff := service.NewHandoffService(repo, nil, nil, nil, nil, h.logger)
			handoff.SetTaskResourceCleaner(&postCommitCleanupFailure{err: errors.New("cleanup unavailable")})
			h.SetHandoffService(handoff)

			c, rec := authzDeleteRequest(t, "user-b", "task-b")
			tc.call(h, c)

			require.Equal(t, http.StatusServiceUnavailable, rec.Code, rec.Body.String())
			require.JSONEq(t, `{"success":false,"pending":true,"task_id":"task-b"}`, rec.Body.String())
			if tc.name == "delete" {
				require.Equal(t, tc.want, repo.deletes())
			} else {
				require.Equal(t, tc.want, repo.archivedTaskIDs())
			}
		})
	}
}
