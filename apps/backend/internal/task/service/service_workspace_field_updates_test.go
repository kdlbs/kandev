package service

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/auth/authn"
	internaldb "github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/orgunit"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
)

type settingsSnapshotGate struct {
	*sqliterepo.Repository
	armed         atomic.Bool
	observed      chan struct{}
	continueWrite chan struct{}
}

func (g *settingsSnapshotGate) GetWorkspace(ctx context.Context, id string) (*models.Workspace, error) {
	row, err := g.Repository.GetWorkspace(ctx, id)
	if err != nil || !g.armed.Swap(false) {
		return row, err
	}
	close(g.observed)
	select {
	case <-g.continueWrite:
		return row, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

type settingsServiceFixture struct {
	service  *Service
	gate     *settingsSnapshotGate
	bus      *MockEventBus
	database *sqlx.DB
}

func settingsPointer[T any](value T) *T { return &value }

func settingsServices(t *testing.T) [2]settingsServiceFixture {
	t.Helper()
	first, filename := serviceTestSQLiteTemplate.Open(t)
	raw, err := internaldb.OpenSQLite(filename)
	require.NoError(t, err)
	second := sqlx.NewDb(raw, "sqlite3")
	t.Cleanup(func() { require.NoError(t, second.Close()) })
	var fixtures [2]settingsServiceFixture
	var physical [2]any
	for i, database := range []*sqlx.DB{first, second} {
		database.SetMaxOpenConns(1)
		database.SetMaxIdleConns(1)
		conn, err := database.Conn(t.Context())
		require.NoError(t, err)
		err = conn.Raw(func(driver any) error { physical[i] = driver; return nil })
		closeErr := conn.Close()
		require.NoError(t, err)
		require.NoError(t, closeErr)
		repo := sqliterepo.NewWithInitializedDB(database, database, nil)
		gate := &settingsSnapshotGate{Repository: repo, observed: make(chan struct{}), continueWrite: make(chan struct{})}
		bus := NewMockEventBus()
		t.Cleanup(bus.Close)
		fixtures[i] = settingsServiceFixture{
			service: NewService(Repos{Workspaces: gate}, bus, accessTestLogger(t), RepositoryDiscoveryConfig{}),
			gate:    gate, bus: bus, database: database,
		}
	}
	require.NotEqual(t, fmt.Sprintf("%p", physical[0]), fmt.Sprintf("%p", physical[1]))
	t.Logf("workspace settings physical SQLite connections: %p / %p", physical[0], physical[1])
	require.NoError(t, fixtures[0].gate.CreateWorkspace(t.Context(), &models.Workspace{
		ID: "settings-row", Name: "Before", Description: "Keep description", OwnerID: "settings-owner",
		DefaultExecutorID: settingsPointer("old-executor"), DefaultEnvironmentID: settingsPointer("old-environment"),
		DefaultAgentProfileID: settingsPointer("old-agent"), DefaultConfigAgentProfileID: settingsPointer("old-config"),
		ACPIdleTimeoutMinutes: 120,
	}))
	return fixtures
}

type settingsOutcome struct {
	row *models.Workspace
	err error
}

func settingsOverlap(t *testing.T, f [2]settingsServiceFixture, held int, requests [2]*UpdateWorkspaceRequest) [2]settingsOutcome {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	gate := f[held].gate
	var join sync.WaitGroup
	var release sync.Once
	defer func() { release.Do(func() { close(gate.continueWrite) }); cancel(); join.Wait() }()
	gate.armed.Store(true)
	done := make(chan settingsOutcome, 1)
	join.Add(1)
	go func() {
		defer join.Done()
		row, err := f[held].service.UpdateWorkspace(ctx, "settings-row", requests[held])
		done <- settingsOutcome{row: row, err: err}
	}()
	select {
	case <-gate.observed:
	case <-ctx.Done():
		t.Fatal("workspace service never observed its real snapshot", ctx.Err())
	}
	var result [2]settingsOutcome
	result[1-held].row, result[1-held].err = f[1-held].service.UpdateWorkspace(ctx, "settings-row", requests[1-held])
	require.NoError(t, result[1-held].err)
	release.Do(func() { close(gate.continueWrite) })
	select {
	case result[held] = <-done:
	case <-ctx.Done():
		t.Fatal("held workspace service did not settle", ctx.Err())
	}
	return result
}

func assertSettingsEvent(t *testing.T, f settingsServiceFixture, row *models.Workspace) {
	t.Helper()
	seen := f.bus.GetPublishedEvents()
	require.Len(t, seen, 1)
	require.Equal(t, events.WorkspaceUpdated, seen[0].Type)
	data, ok := seen[0].Data.(map[string]interface{})
	require.True(t, ok)
	require.Equal(t, row.Name, data["name"])
	require.Equal(t, row.Description, data["description"])
	require.Equal(t, row.UnitID, data["unit_id"])
	require.Equal(t, row.DefaultExecutorID, data["default_executor_id"])
	require.Equal(t, row.DefaultEnvironmentID, data["default_environment_id"])
	require.Equal(t, row.DefaultAgentProfileID, data["default_agent_profile_id"])
	require.Equal(t, row.DefaultConfigAgentProfileID, data["default_config_agent_profile_id"])
	require.Equal(t, row.ACPIdleSuspensionEnabled, data["acp_idle_suspension_enabled"])
	require.Equal(t, row.ACPIdleTimeoutMinutes, data["acp_idle_timeout_minutes"])
	require.Equal(t, row.UpdatedAt.Format(time.RFC3339), data["updated_at"])
}

// @covers AC-WORKSPACES-SETTINGS-UPDATES-001.1, AC-WORKSPACES-SETTINGS-UPDATES-001.7
func TestWorkspaceFieldUpdatesConcurrentSQLite(t *testing.T) {
	for _, pair := range []string{"name_policy", "defaults_description"} {
		for held := range 2 {
			t.Run(fmt.Sprintf("%s/held_%d", pair, held), func(t *testing.T) {
				f := settingsServices(t)
				requests := [2]*UpdateWorkspaceRequest{
					{Name: settingsPointer("Saved name")},
					{ACPIdleSuspensionEnabled: settingsPointer(true), ACPIdleTimeoutMinutes: settingsPointer(45)},
				}
				if pair == "defaults_description" {
					requests = [2]*UpdateWorkspaceRequest{
						{DefaultExecutorID: settingsPointer("new-executor"), DefaultAgentProfileID: settingsPointer("new-agent")},
						{Description: settingsPointer("Saved description")},
					}
				}
				result := settingsOverlap(t, f, held, requests)
				require.NoError(t, result[held].err)
				stored, err := f[0].gate.Repository.GetWorkspace(t.Context(), "settings-row")
				require.NoError(t, err)
				if pair == "name_policy" {
					require.Equal(t, "Saved name", stored.Name)
					require.True(t, stored.ACPIdleSuspensionEnabled)
					require.Equal(t, 45, stored.ACPIdleTimeoutMinutes)
					require.Equal(t, "Keep description", stored.Description)
				} else {
					require.Equal(t, "Saved description", stored.Description)
					require.Equal(t, settingsPointer("new-executor"), stored.DefaultExecutorID)
					require.Equal(t, settingsPointer("new-agent"), stored.DefaultAgentProfileID)
					require.Equal(t, "Before", stored.Name)
				}
				require.Equal(t, settingsPointer("old-environment"), stored.DefaultEnvironmentID)
				require.Equal(t, settingsPointer("old-config"), stored.DefaultConfigAgentProfileID)
				require.Equal(t, stored, result[held].row, "later mutation returns the persisted union")
				for i := range f {
					assertSettingsEvent(t, f[i], result[i].row)
				}
			})
		}
	}
	t.Run("sequential_and_false", func(t *testing.T) {
		f := settingsServices(t)
		_, err := f[0].service.UpdateWorkspace(t.Context(), "settings-row", &UpdateWorkspaceRequest{Name: settingsPointer("Sequential")})
		require.NoError(t, err)
		_, err = f[1].service.UpdateWorkspace(t.Context(), "settings-row", &UpdateWorkspaceRequest{
			ACPIdleSuspensionEnabled: settingsPointer(true), ACPIdleTimeoutMinutes: settingsPointer(45),
		})
		require.NoError(t, err)
		row, err := f[0].service.UpdateWorkspace(t.Context(), "settings-row", &UpdateWorkspaceRequest{ACPIdleSuspensionEnabled: settingsPointer(false)})
		require.NoError(t, err)
		require.Equal(t, "Sequential", row.Name)
		require.False(t, row.ACPIdleSuspensionEnabled)
		require.Equal(t, 45, row.ACPIdleTimeoutMinutes)
	})
}

// @covers AC-WORKSPACES-SETTINGS-UPDATES-001.2, AC-WORKSPACES-SETTINGS-UPDATES-001.3
func TestWorkspaceFieldUpdatesPresenceAndDefaults(t *testing.T) {
	f := settingsServices(t)
	ctx := t.Context()
	require.NoError(t, f[0].gate.CreateWorkspace(ctx, &models.Workspace{ID: "settings-sibling", Name: "Sibling"}))
	sibling, err := f[0].gate.Repository.GetWorkspace(ctx, "settings-sibling")
	require.NoError(t, err)
	require.False(t, sibling.ACPIdleSuspensionEnabled)
	require.Equal(t, 120, sibling.ACPIdleTimeoutMinutes)
	row, err := f[0].service.UpdateWorkspace(ctx, "settings-row", &UpdateWorkspaceRequest{
		Name: settingsPointer(""), Description: settingsPointer(""),
		DefaultExecutorID: settingsPointer(""), DefaultEnvironmentID: settingsPointer(" \t "),
		DefaultAgentProfileID: settingsPointer(" agent-value "), DefaultConfigAgentProfileID: settingsPointer(" config-value "),
		ACPIdleSuspensionEnabled: settingsPointer(true), ACPIdleTimeoutMinutes: settingsPointer(17),
	})
	require.NoError(t, err)
	require.Empty(t, row.Name)
	require.Empty(t, row.Description)
	require.Nil(t, row.DefaultExecutorID)
	require.Nil(t, row.DefaultEnvironmentID)
	require.Equal(t, settingsPointer("agent-value"), row.DefaultAgentProfileID)
	require.Equal(t, settingsPointer("config-value"), row.DefaultConfigAgentProfileID)
	row, err = f[1].service.UpdateWorkspace(ctx, "settings-row", &UpdateWorkspaceRequest{
		DefaultAgentProfileID: settingsPointer(""), DefaultConfigAgentProfileID: settingsPointer(" \t"),
		ACPIdleSuspensionEnabled: settingsPointer(false),
	})
	require.NoError(t, err)
	require.False(t, row.ACPIdleSuspensionEnabled)
	require.Equal(t, 17, row.ACPIdleTimeoutMinutes)
	require.Nil(t, row.DefaultAgentProfileID)
	require.Nil(t, row.DefaultConfigAgentProfileID)
	var allNull bool
	require.NoError(t, f[0].gate.DB().QueryRowContext(ctx, `SELECT default_executor_id IS NULL AND default_environment_id IS NULL AND default_agent_profile_id IS NULL AND default_config_agent_profile_id IS NULL FROM workspaces WHERE id='settings-row'`).Scan(&allNull))
	require.True(t, allNull)
	_, err = f[0].gate.DB().ExecContext(ctx, `UPDATE workspaces SET updated_at = ? WHERE id = ?`, time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC), row.ID)
	require.NoError(t, err)
	row, err = f[0].gate.Repository.GetWorkspace(ctx, row.ID)
	require.NoError(t, err)
	before := *row
	row, err = f[0].service.UpdateWorkspace(ctx, "settings-row", &UpdateWorkspaceRequest{})
	require.NoError(t, err)
	require.True(t, row.UpdatedAt.After(before.UpdatedAt))
	row.UpdatedAt = before.UpdatedAt
	require.Equal(t, &before, row)
	for _, name := range []string{"First value", "Later value"} {
		_, err = f[0].service.UpdateWorkspace(ctx, "settings-row", &UpdateWorkspaceRequest{Name: &name})
		require.NoError(t, err)
	}
	stored, err := f[1].gate.Repository.GetWorkspace(ctx, "settings-row")
	require.NoError(t, err)
	require.Equal(t, "Later value", stored.Name)
	unchangedSibling, err := f[1].gate.Repository.GetWorkspace(ctx, sibling.ID)
	require.NoError(t, err)
	require.Equal(t, sibling, unchangedSibling)
}

// @covers AC-WORKSPACES-SETTINGS-UPDATES-001.5, AC-WORKSPACES-SETTINGS-UPDATES-001.6, AC-WORKSPACES-SETTINGS-UPDATES-001.8
func TestWorkspaceFieldUpdatesExactFenceSQLite(t *testing.T) {
	t.Run("matched_and_stale", func(t *testing.T) {
		f := settingsServices(t)
		before, err := f[0].gate.Repository.GetWorkspace(t.Context(), "settings-row")
		require.NoError(t, err)
		row, err := f[0].service.UpdateWorkspace(t.Context(), before.ID, &UpdateWorkspaceRequest{Name: settingsPointer("Exact"), ExpectedUpdatedAt: &before.UpdatedAt})
		require.NoError(t, err)
		assertSettingsEvent(t, f[0], row)
		f[0].bus.ClearEvents()
		_, err = f[0].service.UpdateWorkspace(t.Context(), before.ID, &UpdateWorkspaceRequest{Name: settingsPointer("Rejected"), ExpectedUpdatedAt: &before.UpdatedAt})
		require.ErrorIs(t, err, repoerrors.ErrTaskVersionConflict)
		require.Empty(t, f[0].bus.GetPublishedEvents())
		stored, err := f[0].gate.Repository.GetWorkspace(t.Context(), before.ID)
		require.NoError(t, err)
		require.Equal(t, row, stored)
		stored.Description = "Complete writer"
		require.NoError(t, f[0].gate.UpdateWorkspaceIfUnchanged(t.Context(), stored, stored.UpdatedAt))
		require.ErrorIs(t, f[1].gate.UpdateWorkspaceIfUnchanged(t.Context(), before, before.UpdatedAt), repoerrors.ErrTaskVersionConflict)
	})
	t.Run("intervening_write_after_service_snapshot", func(t *testing.T) {
		f := settingsServices(t)
		before, err := f[0].gate.Repository.GetWorkspace(t.Context(), "settings-row")
		require.NoError(t, err)
		result := settingsOverlap(t, f, 0, [2]*UpdateWorkspaceRequest{
			{Name: settingsPointer("Rejected"), ExpectedUpdatedAt: &before.UpdatedAt},
			{ACPIdleSuspensionEnabled: settingsPointer(true), ACPIdleTimeoutMinutes: settingsPointer(45)},
		})
		require.ErrorIs(t, result[0].err, repoerrors.ErrTaskVersionConflict)
		require.Empty(t, f[0].bus.GetPublishedEvents())
		stored, err := f[0].gate.Repository.GetWorkspace(t.Context(), "settings-row")
		require.NoError(t, err)
		require.Equal(t, result[1].row, stored)
		assertSettingsEvent(t, f[1], result[1].row)
	})
}

// @covers AC-WORKSPACES-SETTINGS-UPDATES-001.4, AC-WORKSPACES-SETTINGS-UPDATES-001.6
func TestWorkspaceFieldUpdatesAdmissionAndFailures(t *testing.T) {
	t.Run("mutation_failure_and_positive_event_control", func(t *testing.T) {
		f := settingsServices(t)
		before, err := f[0].gate.Repository.GetWorkspace(t.Context(), "settings-row")
		require.NoError(t, err)
		for _, minutes := range []int{0, -1} {
			_, err = f[0].service.UpdateWorkspace(t.Context(), before.ID, &UpdateWorkspaceRequest{Name: settingsPointer("Rejected"), ACPIdleTimeoutMinutes: &minutes})
			require.ErrorIs(t, err, ErrWorkspaceIdleTimeoutInvalid)
		}
		cancelled, cancel := context.WithCancel(t.Context())
		cancel()
		_, err = f[0].service.UpdateWorkspace(cancelled, before.ID, &UpdateWorkspaceRequest{Name: settingsPointer("Cancelled")})
		require.ErrorIs(t, err, context.Canceled)
		_, err = f[0].service.UpdateWorkspace(t.Context(), "missing-settings", &UpdateWorkspaceRequest{Name: settingsPointer("Missing")})
		require.ErrorIs(t, err, repoerrors.ErrWorkspaceNotFound)
		_, err = f[0].gate.DB().ExecContext(t.Context(), `CREATE TRIGGER reject_settings_change BEFORE UPDATE ON workspaces BEGIN SELECT RAISE(ABORT, 'settings mutation rejected'); END`)
		require.NoError(t, err)
		_, err = f[0].service.UpdateWorkspace(t.Context(), before.ID, &UpdateWorkspaceRequest{Name: settingsPointer("Rejected"), ACPIdleSuspensionEnabled: settingsPointer(true)})
		require.ErrorContains(t, err, "settings mutation rejected")
		stored, err := f[0].gate.Repository.GetWorkspace(t.Context(), before.ID)
		require.NoError(t, err)
		require.Equal(t, before, stored)
		require.Empty(t, f[0].bus.GetPublishedEvents())
		_, err = f[0].gate.DB().ExecContext(t.Context(), `DROP TRIGGER reject_settings_change`)
		require.NoError(t, err)
		row, err := f[0].service.UpdateWorkspace(t.Context(), before.ID, &UpdateWorkspaceRequest{Name: settingsPointer("Allowed")})
		require.NoError(t, err)
		assertSettingsEvent(t, f[0], row)
	})
	t.Run("scoped_manage_and_tenant_admission", func(t *testing.T) {
		f := settingsServices(t)
		_, err := f[0].gate.DB().ExecContext(t.Context(), `UPDATE workspaces SET org_id='owned-org' WHERE id='settings-row'`)
		require.NoError(t, err)
		before, err := f[0].gate.Repository.GetWorkspace(t.Context(), "settings-row")
		require.NoError(t, err)
		require.NoError(t, f[0].gate.UpsertWorkspaceMember(t.Context(), &models.WorkspaceMember{WorkspaceID: "settings-row", UserID: "viewer", Role: "viewer"}))
		for _, user := range []struct {
			id   string
			want error
		}{{"viewer", ErrForbidden}, {"outsider", repoerrors.ErrWorkspaceNotFound}} {
			_, err := f[0].service.UpdateWorkspace(ctxAs(user.id), "settings-row", &UpdateWorkspaceRequest{Name: settingsPointer("Rejected")})
			require.ErrorIs(t, err, user.want)
		}
		foreign := authn.WithIdentity(t.Context(), authn.Identity{UserID: "settings-owner", Role: authn.RoleAdmin, OrgID: "other-org"})
		_, err = f[0].service.UpdateWorkspace(foreign, "settings-row", &UpdateWorkspaceRequest{Name: settingsPointer("Foreign denied")})
		require.ErrorIs(t, err, repoerrors.ErrWorkspaceNotFound)
		stored, err := f[0].gate.Repository.GetWorkspace(t.Context(), "settings-row")
		require.NoError(t, err)
		require.Equal(t, before, stored)
		require.Empty(t, f[0].bus.GetPublishedEvents())
		row, err := f[0].service.UpdateWorkspace(ctxAs("settings-owner"), "settings-row", &UpdateWorkspaceRequest{Name: settingsPointer("Owner saved")})
		require.NoError(t, err)
		assertSettingsEvent(t, f[0], row)
	})
	t.Run("unit_admission_and_omitted_unit_overlap", func(t *testing.T) {
		f := settingsServices(t)
		store, err := orgunit.NewStore(internaldb.NewPool(f[0].database, f[0].database))
		require.NoError(t, err)
		units := orgunit.NewService(store, nil)
		root, err := units.EnsureRoot(t.Context(), "", "Settings org")
		require.NoError(t, err)
		destination, err := units.Create(t.Context(), root.ID, "Destination")
		require.NoError(t, err)
		foreign, err := units.EnsureRoot(t.Context(), "foreign-org", "Foreign org")
		require.NoError(t, err)
		require.NoError(t, f[0].gate.PlaceWorkspace(t.Context(), "settings-row", root.ID))
		for i := range f {
			f[i].service.SetUnitPlacer(units)
		}
		owner := authn.WithIdentity(t.Context(), authn.Identity{UserID: "settings-owner", Role: authn.RoleMember})
		_, err = f[0].service.UpdateWorkspace(owner, "settings-row", &UpdateWorkspaceRequest{UnitID: &destination.ID})
		require.ErrorIs(t, err, ErrForbidden)
		_, err = f[0].service.UpdateWorkspace(t.Context(), "settings-row", &UpdateWorkspaceRequest{UnitID: &foreign.ID})
		require.ErrorIs(t, err, ErrUnitNotInWorkspaceOrg)
		_, err = f[0].service.UpdateWorkspace(t.Context(), "settings-row", &UpdateWorkspaceRequest{UnitID: settingsPointer("missing-unit")})
		require.Error(t, err)
		f[0].service.SetUnitPlacer(nil)
		_, err = f[0].service.UpdateWorkspace(t.Context(), "settings-row", &UpdateWorkspaceRequest{UnitID: &destination.ID})
		require.ErrorIs(t, err, ErrUnitNotInWorkspaceOrg)
		f[0].service.SetUnitPlacer(units)
		require.Empty(t, f[0].bus.GetPublishedEvents())
		for _, unit := range []string{"", root.ID} {
			row, err := f[0].service.UpdateWorkspace(owner, "settings-row", &UpdateWorkspaceRequest{UnitID: &unit})
			require.NoError(t, err)
			require.Equal(t, root.ID, row.UnitID)
		}
		f[0].bus.ClearEvents()
		admin := authn.WithIdentity(t.Context(), authn.Identity{UserID: "settings-owner", Role: authn.RoleAdmin})
		moved, err := f[0].service.UpdateWorkspace(admin, "settings-row", &UpdateWorkspaceRequest{UnitID: &destination.ID})
		require.NoError(t, err)
		require.Equal(t, destination.ID, moved.UnitID)
		assertSettingsEvent(t, f[0], moved)
		require.NoError(t, f[0].gate.PlaceWorkspace(t.Context(), "settings-row", root.ID))
		f[0].bus.ClearEvents()
		result := settingsOverlap(t, f, 0, [2]*UpdateWorkspaceRequest{
			{Name: settingsPointer("After move")}, {UnitID: &destination.ID},
		})
		require.NoError(t, result[0].err)
		stored, err := f[0].gate.Repository.GetWorkspace(t.Context(), "settings-row")
		require.NoError(t, err)
		require.Equal(t, destination.ID, stored.UnitID, "omitting UnitID must preserve the admitted move")
		require.Equal(t, "After move", stored.Name)
		for i := range f {
			assertSettingsEvent(t, f[i], result[i].row)
		}
	})
}
