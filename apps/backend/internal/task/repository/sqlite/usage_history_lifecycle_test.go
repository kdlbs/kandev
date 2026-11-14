package sqlite

import (
	"context"
	"database/sql"
	"testing"
	"time"

	analyticssqlite "github.com/kandev/kandev/internal/analytics/repository/sqlite"
	"github.com/kandev/kandev/internal/task/models"
)

func TestUsageHistoryTaskDeleteBackfillsLegacyAndDetachesLinks(t *testing.T) {
	repo := newUsageEventsTestRepo(t)
	if err := repo.CreateWorkspace(context.Background(), &models.Workspace{ID: "ws-1", Name: "Usage history"}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	analyticsRepo, err := analyticssqlite.NewWithDB(repo.db, repo.db)
	if err != nil {
		t.Fatalf("initialize analytics schema: %v", err)
	}
	repo.SetUsageHistoryRetainer(analyticsRepo)
	ensureRetainedUsageMigration(t, repo.db)
	createUsageEventsTestTask(t, repo, "task-history-delete")
	createUsageEventsTestSession(t, repo, "session-history-delete", "task-history-delete")
	insertUsageEventRow(t, repo, "legacy-delete-event", "task-history-delete", "session-history-delete")
	seedLegacyExternalUsage(t, repo, "task-history-delete", "session-history-delete", "legacy-delete")

	if err := repo.DeleteTask(context.Background(), "task-history-delete"); err != nil {
		t.Fatalf("DeleteTask: %v", err)
	}

	var retained struct {
		Count       int           `db:"count"`
		ScopeID     string        `db:"scope_id"`
		SubjectID   string        `db:"subject_id"`
		ActorID     string        `db:"actor_id"`
		ActorStatus string        `db:"actor_status"`
		Tokens      sql.NullInt64 `db:"input_tokens"`
	}
	if err := repo.db.Get(&retained, `SELECT COUNT(*) AS count, scope_id, subject_id, actor_id, actor_status, input_tokens
		FROM session_usage_buckets WHERE source_record_id = 'legacy-delete-event' GROUP BY scope_id, subject_id, actor_id, actor_status, input_tokens`); err != nil {
		t.Fatalf("read retained native legacy event: %v", err)
	}
	if retained.Count != 1 || retained.ScopeID == "" || retained.SubjectID == "" || retained.ActorID != "" || retained.ActorStatus != "unknown" {
		t.Fatalf("retained legacy native identity = %+v, want one scoped subject with unknown actor", retained)
	}
	var external struct {
		MeasurementScope   string `db:"measurement_scope"`
		MeasurementSubject string `db:"measurement_subject"`
		BucketScope        string `db:"bucket_scope"`
		BucketSubject      string `db:"bucket_subject"`
		MeasurementActor   string `db:"measurement_actor"`
		BucketActor        string `db:"bucket_actor"`
	}
	if err := repo.db.Get(&external, `SELECT m.scope_id AS measurement_scope, m.subject_id AS measurement_subject,
		b.scope_id AS bucket_scope, b.subject_id AS bucket_subject,
		m.actor_id AS measurement_actor, b.actor_id AS bucket_actor
		FROM session_usage_measurements m JOIN session_usage_buckets b ON b.source_record_id = m.source_record_id
		WHERE m.source_record_id = 'legacy-delete'`); err != nil {
		t.Fatalf("read retained legacy external records: %v", err)
	}
	if external.MeasurementScope == "" || external.MeasurementSubject == "" || external.BucketScope == "" || external.BucketSubject == "" ||
		external.MeasurementActor != "" || external.BucketActor != "" {
		t.Fatalf("retained legacy external identity = %+v, want stable scopes/subjects and unknown actors", external)
	}
	var liveLinks int
	if err := repo.db.Get(&liveLinks, `SELECT COUNT(*) FROM usage_live_links WHERE live_task_id = 'task-history-delete'`); err != nil {
		t.Fatalf("count task live links after delete: %v", err)
	}
	if liveLinks != 0 {
		t.Fatalf("live task usage links after delete = %d, want 0", liveLinks)
	}
	var legacyEvents int
	if err := repo.db.Get(&legacyEvents, `SELECT COUNT(*) FROM task_usage_events WHERE usage_event_id = 'legacy-delete-event'`); err != nil {
		t.Fatalf("count source native events after delete: %v", err)
	}
	if legacyEvents != 0 {
		t.Fatalf("native source rows after task delete = %d, want task cascade to remove source after retained copy", legacyEvents)
	}
}

func TestUsageHistorySessionPrunePreservesSubjectBeforeDetachment(t *testing.T) {
	repo := newUsageEventsTestRepo(t)
	if err := repo.CreateWorkspace(context.Background(), &models.Workspace{ID: "ws-1", Name: "Usage history"}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	analyticsRepo, err := analyticssqlite.NewWithDB(repo.db, repo.db)
	if err != nil {
		t.Fatalf("initialize analytics schema: %v", err)
	}
	repo.SetUsageHistoryRetainer(analyticsRepo)
	createUsageEventsTestTask(t, repo, "task-history-prune")
	createUsageEventsTestSession(t, repo, "session-history-prune", "task-history-prune")
	insertUsageEventRow(t, repo, "legacy-prune-event", "task-history-prune", "session-history-prune")
	seedLegacyExternalUsage(t, repo, "task-history-prune", "session-history-prune", "legacy-prune")
	session, err := repo.GetTaskSession(context.Background(), "session-history-prune")
	if err != nil {
		t.Fatalf("load session incarnation: %v", err)
	}
	incarnation := session.QueueIncarnationID

	if err := repo.DeleteTaskSession(context.Background(), session); err != nil {
		t.Fatalf("DeleteTaskSession: %v", err)
	}

	var retained struct {
		ScopeID              string `db:"scope_id"`
		SubjectID            string `db:"subject_id"`
		SessionID            string `db:"session_id"`
		ActorID              string `db:"actor_id"`
		ActorStatus          string `db:"actor_status"`
		SessionIncarnationID string `db:"session_incarnation_id"`
	}
	if err := repo.db.Get(&retained, `SELECT b.scope_id, b.subject_id, COALESCE(b.session_id, '') AS session_id,
		b.actor_id, b.actor_status, s.session_incarnation_id
		FROM session_usage_buckets b JOIN usage_subjects s ON s.subject_id = b.subject_id
		WHERE b.source_record_id = 'legacy-prune-event'`); err != nil {
		t.Fatalf("read retained native event after session prune: %v", err)
	}
	if retained.ScopeID == "" || retained.SubjectID == "" || retained.SessionID != "session-history-prune" ||
		retained.ActorID != "" || retained.ActorStatus != "unknown" || retained.SessionIncarnationID != incarnation {
		t.Fatalf("retained pruned session identity = %+v, want stable incarnation %q and unknown actor", retained, incarnation)
	}
	var liveLinks int
	if err := repo.db.Get(&liveLinks, `SELECT COUNT(*) FROM usage_live_links WHERE live_session_id = 'session-history-prune'`); err != nil {
		t.Fatalf("count pruned live links: %v", err)
	}
	if liveLinks != 0 {
		t.Fatalf("live session usage links after prune = %d, want 0", liveLinks)
	}
}

func TestUsageHistoryTaskIDReuseKeepsPriorIncarnation(t *testing.T) {
	ctx := context.Background()
	repo := newUsageEventsTestRepo(t)
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Usage history"}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	analyticsRepo, err := analyticssqlite.NewWithDB(repo.db, repo.db)
	if err != nil {
		t.Fatalf("initialize analytics schema: %v", err)
	}
	repo.SetUsageHistoryRetainer(analyticsRepo)

	const taskID = "task-history-reused"
	createUsageEventsTestTask(t, repo, taskID)
	firstTaskCreatedAt := time.Date(2020, time.January, 1, 0, 0, 0, 0, time.UTC)
	if _, err := repo.db.Exec(repo.db.Rebind(`UPDATE tasks SET created_at = ? WHERE id = ?`), firstTaskCreatedAt, taskID); err != nil {
		t.Fatalf("seed first task incarnation: %v", err)
	}
	createUsageEventsTestSession(t, repo, "session-history-first", taskID)
	if err := reportExternalUsage(ctx, analyticsRepo, "ws-1", "session-history-first", "reuse", "reuse-first", "model", 11, firstTaskCreatedAt.Add(time.Minute)); err != nil {
		t.Fatalf("write usage for first task incarnation: %v", err)
	}
	var firstSubjectID string
	if err := repo.db.Get(&firstSubjectID, `SELECT subject_id FROM usage_subjects WHERE session_id = 'session-history-first'`); err != nil || firstSubjectID == "" {
		t.Fatalf("first retained subject = %q, want a captured subject", firstSubjectID)
	}

	if err := repo.DeleteTask(ctx, taskID); err != nil {
		t.Fatalf("delete first task incarnation: %v", err)
	}
	var preservedCount int
	if err := repo.db.Get(&preservedCount, `SELECT COUNT(*) FROM session_usage_measurements WHERE source_record_id = 'reuse-first'`); err != nil {
		t.Fatalf("count first retained observation: %v", err)
	}
	if preservedCount != 1 {
		t.Fatalf("first retained observations after deletion = %d, want 1", preservedCount)
	}

	createUsageEventsTestTask(t, repo, taskID)
	secondTaskCreatedAt := time.Date(2020, time.January, 2, 0, 0, 0, 0, time.UTC)
	if _, err := repo.db.Exec(repo.db.Rebind(`UPDATE tasks SET created_at = ? WHERE id = ?`), secondTaskCreatedAt, taskID); err != nil {
		t.Fatalf("seed second task incarnation: %v", err)
	}
	createUsageEventsTestSession(t, repo, "session-history-second", taskID)
	if err := reportExternalUsage(ctx, analyticsRepo, "ws-1", "session-history-second", "reuse", "reuse-second", "model", 17, secondTaskCreatedAt.Add(time.Minute)); err != nil {
		t.Fatalf("write usage for reused task id: %v", err)
	}
	var secondSubjectID string
	if err := repo.db.Get(&secondSubjectID, `SELECT subject_id FROM usage_subjects WHERE session_id = 'session-history-second'`); err != nil || secondSubjectID == "" || secondSubjectID == firstSubjectID {
		t.Fatalf("second task subject = %q, want a new incarnation separate from %q", secondSubjectID, firstSubjectID)
	}

	var retained struct {
		Rows              int    `db:"rows"`
		FirstTaskCreated  string `db:"first_task_created"`
		SecondTaskCreated string `db:"second_task_created"`
		FirstActorID      string `db:"first_actor_id"`
		FirstActorStatus  string `db:"first_actor_status"`
	}
	if err := repo.db.Get(&retained, `SELECT COUNT(*) AS rows,
		MAX(CASE WHEN b.source_record_id = 'reuse-first' THEN s.task_incarnation_id END) AS first_task_created,
		MAX(CASE WHEN b.source_record_id = 'reuse-second' THEN s.task_incarnation_id END) AS second_task_created,
		MAX(CASE WHEN b.source_record_id = 'reuse-first' THEN b.actor_id END) AS first_actor_id,
		MAX(CASE WHEN b.source_record_id = 'reuse-first' THEN b.actor_status END) AS first_actor_status
		FROM session_usage_measurements b JOIN usage_subjects s ON s.subject_id = b.subject_id
		WHERE b.source_record_id IN ('reuse-first', 'reuse-second')`); err != nil {
		t.Fatalf("read retained reused-task subjects: %v", err)
	}
	if retained.Rows != 2 || retained.FirstTaskCreated == retained.SecondTaskCreated || retained.FirstActorID != "" || retained.FirstActorStatus != "unknown" {
		t.Fatalf("retained task-id reuse identities = %+v, want two incarnations and original unknown attribution", retained)
	}
}

func seedLegacyExternalUsage(t *testing.T, repo *Repository, taskID, sessionID, sourceRecordID string) {
	t.Helper()
	now := time.Now().UTC()
	for _, table := range []string{"session_usage_measurements", "session_usage_buckets"} {
		query := `INSERT INTO ` + table + ` (id, workspace_id, task_id, session_id, source, source_record_id,
			usage_identity, model, provider, revision, payload_digest, observed_at, collected_at,
			input_tokens, total_tokens, coverage_key, source_date, source_timezone, actor_status, actor_kind)
			VALUES (?, 'ws-1', ?, ?, 'plugin:legacy', ?, 'legacy-work', 'model-exact', 'provider', 1, 'legacy-digest', ?, ?, 23, 23, '2026-10-28', '2026-10-28', 'UTC', 'unknown', 'unknown')`
		if _, err := repo.db.Exec(repo.db.Rebind(query), table+"-"+sourceRecordID, taskID, sessionID, sourceRecordID, now, now); err != nil {
			t.Fatalf("seed %s legacy usage: %v", table, err)
		}
	}
}
