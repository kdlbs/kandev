package sqlite

import (
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestTaskFlagNoteSurvivesFullSnapshotWriteAndSchemaReplay(t *testing.T) {
	for _, engine := range []string{"sqlite", "postgres"} {
		t.Run(engine, func(t *testing.T) {
			ctx := t.Context()
			repo := newRepoForSidebarConformance(t, engine)
			seedWorkspace(t, repo, "flag-workspace")
			require.NoError(t, repo.CreateTask(ctx, &models.Task{
				ID: "flag-task", Title: "Original", WorkspaceID: "flag-workspace",
			}))
			snapshot, err := repo.GetTask(ctx, "flag-task")
			require.NoError(t, err)
			require.Empty(t, snapshot.FlagNote)

			note := "Check links"
			_, err = repo.UpdateTaskFieldsWithParentAdmission(ctx, snapshot.ID, models.TaskFieldUpdate{FlagNote: &note}, nil)
			require.NoError(t, err)
			snapshot.Title = "Updated from an earlier snapshot"
			require.NoError(t, repo.UpdateTask(ctx, snapshot))

			reopened, err := NewWithDB(repo.db, repo.ro, nil)
			require.NoError(t, err)
			page, err := reopened.QuerySidebarTaskPage(ctx, snapshot.WorkspaceID, sidebarTaskQuery(1), models.SidebarTaskViewPreferences{})
			require.NoError(t, err)
			require.Len(t, page.Tasks, 1)
			require.Equal(t, snapshot.Title, page.Tasks[0].Title)
			require.Equal(t, note, page.Tasks[0].FlagNote)
		})
	}
}
