package handlers

import (
	"context"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/service"
	usermodels "github.com/kandev/kandev/internal/user/models"
	userservice "github.com/kandev/kandev/internal/user/service"
	userstore "github.com/kandev/kandev/internal/user/store"
	"github.com/stretchr/testify/require"
)

type sidebarPreferenceAccessBarrier struct {
	reader  *userservice.Service
	entered chan struct{}
	release chan struct{}
}

func (r *sidebarPreferenceAccessBarrier) GetUserSettings(ctx context.Context) (*usermodels.UserSettings, error) {
	if r.entered != nil {
		close(r.entered)
		select {
		case <-r.release:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return r.reader.GetUserSettings(ctx)
}

func sidebarAccessSettings(t *testing.T, h *TaskHandlers) *userservice.Service {
	t.Helper()
	conn, err := db.OpenSQLite(filepath.Join(t.TempDir(), "settings.db"))
	require.NoError(t, err)
	sqlDB := sqlx.NewDb(conn, "sqlite3")
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	repo, cleanup, err := userstore.Provide(sqlDB, sqlDB)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, cleanup()) })
	svc := userservice.NewService(repo, nil, h.logger)
	svc.SetSidebarWorkspaceAccess(func(ctx context.Context) ([]string, error) {
		workspaces, err := h.service.ListWorkspaces(ctx)
		ids := make([]string, 0, len(workspaces))
		for _, workspace := range workspaces {
			ids = append(ids, workspace.ID)
		}
		return ids, err
	})
	return svc
}

func TestSidebarSavedDisclosureKeepsWorkspaceDenialPrecedence(t *testing.T) {
	for _, mode := range []string{"member retained", "missing view", "already removed", "removed during lookup"} {
		t.Run(mode, func(t *testing.T) {
			h, repo := newPlanTestHandlersWithRepo(t)
			h.service = service.NewService(service.Repos{
				Workspaces: repo, Tasks: repo, TaskRepos: repo, Workflows: repo, Messages: repo,
				Turns: repo, Sessions: repo, GitSnapshots: repo, RepoEntities: repo, Executors: repo,
				Environments: repo, TaskEnvironments: repo, Reviews: repo,
			}, nil, h.logger, service.RepositoryDiscoveryConfig{})
			h.service.SetSidebarGroupingResolver(savedDisclosureResolver{})
			require.NoError(t, repo.CreateWorkspace(t.Context(), &models.Workspace{ID: "private", Name: "Private", OwnerID: "owner"}))
			require.NoError(t, repo.CreateTask(t.Context(), &models.Task{ID: "private-task", WorkspaceID: "private", Title: "Private task"}))
			require.NoError(t, repo.UpsertWorkspaceMember(t.Context(), &models.WorkspaceMember{WorkspaceID: "private", UserID: userstore.DefaultUserID, Role: "viewer"}))
			readerCtx := authn.WithIdentity(t.Context(), authn.Identity{UserID: userstore.DefaultUserID, Role: authn.RoleMember})
			ownerCtx := authn.WithIdentity(t.Context(), authn.Identity{UserID: "owner", Role: authn.RoleMember})
			settings := sidebarAccessSettings(t, h)
			active := "saved"
			_, err := settings.UpdateUserSettings(readerCtx, &userservice.UpdateUserSettingsRequest{SidebarViewState: &usermodels.SidebarWorkspacePatch{
				WorkspaceID: "private", ViewsUpsert: []usermodels.SidebarView{{ID: active, Name: "Saved", Group: "plugin:acme:labels"}}, ActiveViewID: &active,
			}})
			require.NoError(t, err)
			gate := &sidebarPreferenceAccessBarrier{reader: settings}
			if mode == "removed during lookup" {
				gate.entered, gate.release = make(chan struct{}), make(chan struct{})
			}
			h.SetSidebarTaskSettingsReader(gate)
			if mode == "already removed" {
				require.NoError(t, h.service.RemoveWorkspaceMember(ownerCtx, "private", userstore.DefaultUserID))
			}
			viewID := "saved"
			if mode == "missing view" {
				viewID = "missing"
			}
			c, rec := taskRequestAs(t, userstore.DefaultUserID, http.MethodPost, "/api/v1/workspaces/private/sidebar/query", "private")
			ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
			defer cancel()
			c.Request = c.Request.WithContext(ctx)
			c.Request.Body = io.NopCloser(strings.NewReader(`{"group":"plugin:acme:labels","disclosure_view_id":"` + viewID + `"}`))
			done := make(chan struct{})
			go func() { h.httpQuerySidebarTasks(c); close(done) }()
			if gate.entered != nil {
				select {
				case <-gate.entered:
				case <-ctx.Done():
					t.Fatal(ctx.Err())
				}
				require.NoError(t, h.service.RemoveWorkspaceMember(ownerCtx, "private", userstore.DefaultUserID))
				close(gate.release)
			}
			select {
			case <-done:
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
			want := http.StatusOK
			if mode == "missing view" {
				want = http.StatusBadRequest
			} else if mode != "member retained" {
				want = http.StatusNotFound
			}
			require.Equal(t, want, rec.Code, rec.Body.String())
			if want != http.StatusOK {
				require.NotContains(t, rec.Body.String(), "Private task")
			}
		})
	}
}
