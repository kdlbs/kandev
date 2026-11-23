package service

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	"github.com/stretchr/testify/require"
)

type pendingSidebarClassification struct {
	entered chan struct{}
	release chan struct{}
}

func (r *pendingSidebarClassification) ResolveSidebarGrouping(ctx context.Context, workspace, reference string) (*models.SidebarPluginGrouping, error) {
	close(r.entered)
	select {
	case <-r.release:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	return (&testSidebarGroupingResolver{}).ResolveSidebarGrouping(ctx, workspace, reference)
}

func TestSidebarGroupingRenewsMembershipAfterClassification(t *testing.T) {
	for _, revoke := range []bool{false, true} {
		t.Run(map[bool]string{false: "membership retained", true: "membership revoked"}[revoke], func(t *testing.T) {
			svc, _, repo := createTestService(t)
			seedScopedWorkspaces(t, repo)
			require.NoError(t, repo.UpsertWorkspaceMember(t.Context(), &models.WorkspaceMember{WorkspaceID: "ws-b", UserID: "reader", Role: "viewer"}))
			resolver := &pendingSidebarClassification{entered: make(chan struct{}), release: make(chan struct{})}
			svc.SetSidebarGroupingResolver(resolver)
			query := models.SidebarTaskViewQuery{Sort: models.SidebarTaskViewSort{Key: "title", Direction: "asc"}, Group: "plugin:acme:labels", Page: 1, PageSize: 100, Locale: "en"}
			type result struct {
				page *models.SidebarTaskPageResult
				err  error
			}
			done := make(chan result, 1)
			go func() {
				page, err := svc.QuerySidebarTaskPage(ctxAs("reader"), "ws-b", query, models.SidebarTaskViewPreferences{})
				done <- result{page, err}
			}()
			<-resolver.entered
			if revoke {
				require.NoError(t, svc.RemoveWorkspaceMember(ctxAs("user-b"), "ws-b", "reader"))
			}
			close(resolver.release)
			got := <-done
			if revoke {
				require.ErrorIs(t, got.err, repoerrors.ErrWorkspaceNotFound)
				require.Nil(t, got.page)
			} else {
				require.NoError(t, got.err)
				require.Equal(t, 1, got.page.TotalTasks)
			}
		})
	}
}
