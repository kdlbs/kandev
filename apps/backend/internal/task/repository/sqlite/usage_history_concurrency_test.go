package sqlite

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/analytics/models"
	analyticssqlite "github.com/kandev/kandev/internal/analytics/repository/sqlite"
	taskmodels "github.com/kandev/kandev/internal/task/models"
)

func TestUsageHistoryDeleteSerializesNativeAndExternalAdmission(t *testing.T) {
	for _, source := range []string{"native", "external"} {
		t.Run(source, func(t *testing.T) {
			ctx := context.Background()
			repo := newUsageEventsTestRepo(t)
			if err := repo.CreateWorkspace(ctx, &taskmodels.Workspace{ID: "ws-1", Name: "Usage history"}); err != nil {
				t.Fatalf("create workspace: %v", err)
			}
			analyticsRepo, err := analyticssqlite.NewWithDB(repo.db, repo.db)
			if err != nil {
				t.Fatalf("initialize analytics repository: %v", err)
			}
			repo.SetUsageHistoryRetainer(analyticsRepo)
			createUsageEventsTestTask(t, repo, "task-delete-write-race")
			createUsageEventsTestSession(t, repo, "session-delete-write-race", "task-delete-write-race")

			start := make(chan struct{})
			ready := make(chan struct{}, 2)
			var wait sync.WaitGroup
			wait.Add(2)
			var writeErr, deleteErr error
			go func() {
				defer wait.Done()
				ready <- struct{}{}
				<-start
				if source == "native" {
					writeErr = repo.CreateTaskUsageEvent(ctx, newTestUsageEvent("race-native-event", "task-delete-write-race", "session-delete-write-race"))
					return
				}
				_, writeErr = analyticsRepo.UpsertSessionUsage(ctx, []models.SessionUsageMeasurement{{
					WorkspaceID: "ws-1", TaskID: "task-delete-write-race", SessionID: "session-delete-write-race",
					Source: "plugin:race", SourceRecordID: "race-external-record", UsageIdentity: "race-work",
					Model: "model-race", Provider: "provider-race", Revision: 1, PayloadDigest: "race-payload",
					ObservedAt: time.Now().UTC(), CollectedAt: time.Now().UTC(),
					InputTokens: models.Int64(12), TotalTokens: models.Int64(12),
				}})
			}()
			go func() {
				defer wait.Done()
				ready <- struct{}{}
				<-start
				deleteErr = repo.DeleteTask(ctx, "task-delete-write-race")
			}()
			<-ready
			<-ready
			close(start)
			wait.Wait()
			if deleteErr != nil {
				t.Fatalf("DeleteTask: %v", deleteErr)
			}

			var retained int
			var sourceRows int
			switch source {
			case "native":
				if err := repo.db.Get(&retained, `SELECT COUNT(*) FROM session_usage_buckets WHERE source = 'native' AND source_record_id = 'race-native-event'`); err != nil {
					t.Fatalf("count native retained rows: %v", err)
				}
				if err := repo.db.Get(&sourceRows, `SELECT COUNT(*) FROM task_usage_events WHERE usage_event_id = 'race-native-event'`); err != nil {
					t.Fatalf("count native source rows: %v", err)
				}
			default:
				if err := repo.db.Get(&retained, `SELECT COUNT(*) FROM session_usage_measurements WHERE source_record_id = 'race-external-record' AND scope_id <> '' AND subject_id <> ''`); err != nil {
					t.Fatalf("count external retained rows: %v", err)
				}
				if err := repo.db.Get(&sourceRows, `SELECT COUNT(*) FROM usage_live_links WHERE observation_id IN (SELECT id FROM session_usage_measurements WHERE source_record_id = 'race-external-record') AND live_task_id IS NOT NULL`); err != nil {
					t.Fatalf("count external live references: %v", err)
				}
			}
			if sourceRows != 0 {
				t.Fatalf("live source rows/references after task deletion = %d, want 0", sourceRows)
			}
			if writeErr != nil && !strings.Contains(writeErr.Error(), "usage writes are fenced") &&
				!strings.Contains(writeErr.Error(), "resolve usage preservation task") &&
				!strings.Contains(writeErr.Error(), "resolve trusted task ownership") &&
				!strings.Contains(writeErr.Error(), "does not exist") {
				t.Fatalf("concurrent %s write failed outside the deletion admission fence: %v", source, writeErr)
			}
			if writeErr == nil && retained != 1 {
				t.Fatalf("accepted %s write has %d retained observations, want 1", source, retained)
			}
			if writeErr != nil && retained != 0 {
				t.Fatalf("rejected %s write left %d retained observations, want 0", source, retained)
			}
		})
	}
}
