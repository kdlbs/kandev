package changes

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/common/turnchanges"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/sqlite"
)

func TestTurnChangeExportTimeoutPrefixResumesThroughRealRepository(t *testing.T) {
	conn, err := db.OpenSQLite(filepath.Join(t.TempDir(), "turn-export-integration.db"))
	require.NoError(t, err)
	storeDB := sqlx.NewDb(conn, "sqlite3")
	t.Cleanup(func() { _ = storeDB.Close() })
	repo, err := sqlite.NewWithDB(storeDB, storeDB, nil)
	require.NoError(t, err)
	require.NoError(t, seedExportIntegrationRepo(t, repo))

	added, deleted := int64(1), int64(1)
	client := &coordinatorCheckpointClient{
		provenance: true,
		comparisonFiles: []turnchanges.CheckpointFile{
			{PathBytes: []byte("first.txt"), Kind: "modified", Added: &added, Deleted: &deleted},
			{PathBytes: []byte("second.txt"), Kind: "modified", Added: &added, Deleted: &deleted},
		},
	}
	client.exportPageBuilder = func(request turnchanges.ExportRequest) *turnchanges.CheckpointExport {
		file := turnchanges.CheckpointFile{
			PathBytes: []byte("first.txt"), Kind: "modified", Added: &added, Deleted: &deleted,
		}
		if request.Cursor > 0 {
			file.PathBytes = []byte("second.txt")
		}
		complete := request.Cursor > 0
		next := request.Cursor + 1
		page := &turnchanges.CheckpointExport{
			ChangeSetID: request.ChangeSetID, CheckoutID: request.CheckoutID,
			HashAlgorithm: request.HashAlgorithm, StartCommitOID: request.StartCommitOID,
			StartTreeOID: request.StartTreeOID, EndCommitOID: request.EndCommitOID, EndTreeOID: request.EndTreeOID,
			ComparisonVersion: turnchanges.ComparisonVersion2, ComparisonBasis: turnchanges.ComparisonDirect,
			EffectiveStartCommitOID: request.EffectiveStartCommitOID, EffectiveStartTreeOID: request.EffectiveStartTreeOID,
			EffectiveEndCommitOID: request.EffectiveEndCommitOID, EffectiveEndTreeOID: request.EffectiveEndTreeOID,
			ExportBatchVersion: turnchanges.ExportBatchVersion1, NextCursor: next, TotalFiles: 2,
			InventoryFingerprint: "inventory", Complete: complete,
			Files: []turnchanges.CheckpointExportFile{{
				File: file, ContentAvailability: turnchanges.Ready, CanonicalPatch: []byte("patch"), FilteredPatch: []byte("patch"),
			}},
		}
		if !complete {
			page.Reason = turnchanges.ReasonExportTimeout
		}
		return page
	}
	policy := &coordinatorPolicy{result: turnchanges.CapturePolicy{
		SettingsUserID: "user-initiator", Enabled: true, ResolutionKind: turnchanges.PolicyAuthenticatedUser,
	}}
	turns := coordinatorTurnReaderMap{"turn-1": {ID: "turn-1", TaskID: "task-1", TaskSessionID: "session-1"}}
	coordinator := NewCoordinator(repo, turns, policy, NewContentService(repo, nil), nil)
	admission := coordinatorAdmission()
	require.NoError(t, coordinator.Admit(context.Background(), admission, client))
	terminal := exportTerminal(admission)
	require.NoError(t, coordinator.CaptureTerminalEndpoints(context.Background(), terminal, client))
	require.NoError(t, coordinator.ProcessTerminalComparison(context.Background(), terminal, client))

	changeSet, err := repo.GetTurnChangeSetByID(context.Background(), turnChangeSetID(admission.TurnID))
	require.NoError(t, err)
	rows, err := repo.ListTurnRepositoryChanges(context.Background(), changeSet.ID)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	resumed, err := coordinator.preserveTerminalRepositoryContent(context.Background(), changeSet, *rows[0], client)
	require.NoError(t, err)
	require.Equal(t, int64(2), resumed.ExportCursor)
	require.Equal(t, models.TurnChangeExportComplete, resumed.ExportState)
	require.True(t, resumed.ContentComplete)

	files, total, err := repo.ListTurnFileChanges(context.Background(), changeSet.ID, rows[0].ID, 0, 10)
	require.NoError(t, err)
	require.Equal(t, 2, total)
	for _, file := range files {
		require.Equal(t, models.TurnChangeAvailabilityReady, file.ContentAvailability)
	}
}

func seedExportIntegrationRepo(t *testing.T, repo *sqlite.Repository) error {
	t.Helper()
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-export", Name: "Export"}); err != nil {
		return err
	}
	if err := repo.CreateRepository(ctx, &models.Repository{ID: "repo-1", WorkspaceID: "ws-export", Name: "repo"}); err != nil {
		return err
	}
	if err := repo.CreateTask(ctx, &models.Task{ID: "task-1", Title: "Export"}); err != nil {
		return err
	}
	if err := repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{ID: "env-1", TaskID: "task-1"}); err != nil {
		return err
	}
	if err := repo.CreateTaskEnvironmentRepo(ctx, &models.TaskEnvironmentRepo{
		ID: "env-repo-1", TaskEnvironmentID: "env-1", RepositoryID: "repo-1",
		WorkspaceRelativePath: "frontend",
	}); err != nil {
		return err
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "session-1", TaskID: "task-1", TaskEnvironmentID: "env-1",
	}); err != nil {
		return err
	}
	return repo.CreateTurn(ctx, &models.Turn{
		ID: "turn-1", TaskID: "task-1", TaskSessionID: "session-1", StartedAt: time.Now().UTC(),
	})
}

func TestTurnChangeExportTimeoutExhaustionRetainsPrefix(t *testing.T) {
	conn, err := db.OpenSQLite(filepath.Join(t.TempDir(), "turn-export-exhaustion.db"))
	require.NoError(t, err)
	storeDB := sqlx.NewDb(conn, "sqlite3")
	t.Cleanup(func() { _ = storeDB.Close() })
	repo, err := sqlite.NewWithDB(storeDB, storeDB, nil)
	require.NoError(t, err)
	require.NoError(t, seedExportIntegrationRepo(t, repo))

	added, deleted := int64(1), int64(1)
	client := &coordinatorCheckpointClient{
		provenance: true,
		comparisonFiles: []turnchanges.CheckpointFile{
			{PathBytes: []byte("first.txt"), Kind: "modified", Added: &added, Deleted: &deleted},
			{PathBytes: []byte("second.txt"), Kind: "modified", Added: &added, Deleted: &deleted},
		},
	}
	client.exportPageBuilder = func(request turnchanges.ExportRequest) *turnchanges.CheckpointExport {
		page := &turnchanges.CheckpointExport{
			ChangeSetID: request.ChangeSetID, CheckoutID: request.CheckoutID,
			HashAlgorithm: request.HashAlgorithm, StartCommitOID: request.StartCommitOID,
			StartTreeOID: request.StartTreeOID, EndCommitOID: request.EndCommitOID, EndTreeOID: request.EndTreeOID,
			ComparisonVersion: turnchanges.ComparisonVersion2, ComparisonBasis: turnchanges.ComparisonDirect,
			EffectiveStartCommitOID: request.EffectiveStartCommitOID, EffectiveStartTreeOID: request.EffectiveStartTreeOID,
			EffectiveEndCommitOID: request.EffectiveEndCommitOID, EffectiveEndTreeOID: request.EffectiveEndTreeOID,
			ExportBatchVersion: turnchanges.ExportBatchVersion1, TotalFiles: 2,
			InventoryFingerprint: "inventory", Reason: turnchanges.ReasonExportTimeout,
		}
		if request.Cursor == 0 {
			page.NextCursor = 1
			page.Files = []turnchanges.CheckpointExportFile{{
				File: turnchanges.CheckpointFile{
					PathBytes: []byte("first.txt"), Kind: "modified", Added: &added, Deleted: &deleted,
				},
				ContentAvailability: turnchanges.Ready,
				CanonicalPatch:      []byte("patch"), FilteredPatch: []byte("patch"),
			}}
		}
		return page
	}
	policy := &coordinatorPolicy{result: turnchanges.CapturePolicy{
		SettingsUserID: "user-initiator", Enabled: true, ResolutionKind: turnchanges.PolicyAuthenticatedUser,
	}}
	turns := coordinatorTurnReaderMap{"turn-1": {ID: "turn-1", TaskID: "task-1", TaskSessionID: "session-1"}}
	coordinator := NewCoordinator(repo, turns, policy, NewContentService(repo, nil), nil)
	admission := coordinatorAdmission()
	require.NoError(t, coordinator.Admit(context.Background(), admission, client))
	terminal := exportTerminal(admission)
	require.NoError(t, coordinator.CaptureTerminalEndpoints(context.Background(), terminal, client))
	require.NoError(t, coordinator.ProcessTerminalComparison(context.Background(), terminal, client))

	changeSet, err := repo.GetTurnChangeSetByID(context.Background(), turnChangeSetID(admission.TurnID))
	require.NoError(t, err)
	rows, err := repo.ListTurnRepositoryChanges(context.Background(), changeSet.ID)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	failed, err := coordinator.preserveTerminalRepositoryContent(context.Background(), changeSet, *rows[0], client)
	require.NoError(t, err)
	require.Equal(t, models.TurnChangeExportUnavailable, failed.ExportState)
	require.Equal(t, models.TurnChangeReasonExportTimeout, failed.ExportReason)

	files, total, err := repo.ListTurnFileChanges(context.Background(), changeSet.ID, rows[0].ID, 0, 10)
	require.NoError(t, err)
	require.Equal(t, 2, total)
	readiness := map[string]models.TurnChangeAvailability{}
	for _, file := range files {
		readiness[file.Path] = file.ContentAvailability
	}
	require.Equal(t, models.TurnChangeAvailabilityReady, readiness["first.txt"])
	require.NotEqual(t, models.TurnChangeAvailabilityReady, readiness["second.txt"])
}
