package sqlite

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	managed "github.com/kandev/kandev/internal/task/repository/managedconversation"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

func TestDeliveryRetentionMetadataPagination(t *testing.T) {
	for _, engine := range []string{"SQLite", "Postgres"} {
		t.Run(engine, func(t *testing.T) {
			repo := newRetentionTestRepo(t, engine)
			seedAgentDeliveryTestSession(t, repo, "retention-page-session", "retention-page-incarnation")
			ctx := context.Background()
			base := time.Date(2026, time.October, 9, 12, 0, 0, 0, time.UTC)
			states := []models.DeliverySubmissionState{
				models.DeliverySubmissionPrepared,
				models.DeliverySubmissionAccepted,
				models.DeliverySubmissionDispatching,
				models.DeliverySubmissionInterruptedUnknown,
				models.DeliverySubmissionCompleted,
			}
			for index, state := range states {
				submission := &models.AgentDeliverySubmission{
					ID:        "prompt:retention-page-" + string(rune('a'+index)),
					SessionID: "retention-page-session", IncarnationID: "retention-page-incarnation",
					HarnessGeneration: 4, OwnerGeneration: 3, DispatchAttemptID: "attempt-page",
					PayloadHash: "hash-page-" + string(rune('a'+index)), Payload: []byte("large prompt payload"),
					State: state, Outcome: "retained outcome", CreatedAt: base.Add(time.Duration(index) * time.Second),
				}
				if _, err := repo.PrepareAgentDeliverySubmission(ctx, submission); err != nil {
					t.Fatalf("prepare submission %d: %v", index, err)
				}
			}
			prunedAt := base.Add(time.Hour)
			if _, err := repo.db.Exec(repo.db.Rebind(`
				UPDATE agent_delivery_submissions SET payload = ?, payload_pruned_at = ?, payload_bytes = ?
				WHERE id IN (?, ?)`), []byte{}, prunedAt, len("large prompt payload"),
				"prompt:retention-page-a", "prompt:retention-page-b"); err != nil {
				t.Fatalf("prune prompt fixture: %v", err)
			}

			var gotIDs []string
			var afterCreatedAt *time.Time
			afterID := ""
			for {
				page, err := repo.ListAgentDeliverySubmissionsPage(ctx,
					"retention-page-session", afterCreatedAt, afterID, 2)
				if err != nil {
					t.Fatalf("read metadata page: %v", err)
				}
				if len(page) > 2 {
					t.Fatalf("metadata page has %d rows, want at most 2", len(page))
				}
				for _, submission := range page {
					if len(submission.Payload) != 0 || submission.PayloadHash == "" || submission.SessionID != "retention-page-session" {
						t.Fatalf("metadata page exposed or omitted fields: %+v", submission)
					}
					gotIDs = append(gotIDs, submission.ID)
				}
				if len(page) < 2 {
					break
				}
				last := page[len(page)-1]
				createdAt := last.CreatedAt
				afterCreatedAt, afterID = &createdAt, last.ID
			}
			if len(gotIDs) != len(states) {
				t.Fatalf("paged submission IDs = %v, want %d rows", gotIDs, len(states))
			}
			for index, id := range gotIDs {
				want := "prompt:retention-page-" + string(rune('a'+index))
				if id != want {
					t.Errorf("page item %d ID = %q, want %q", index, id, want)
				}
			}

			metadata, err := repo.GetAgentDeliverySubmissionMetadata(ctx, "prompt:retention-page-a")
			if err != nil || metadata.PayloadPrunedAt == nil || metadata.PayloadBytes != int64(len("large prompt payload")) {
				t.Fatalf("read pruned submission metadata = %+v, err %v", metadata, err)
			}
			if _, err := repo.GetAgentDeliverySubmission(ctx, metadata.ID); !errors.Is(err, repoerrors.ErrAgentDeliveryPayloadPruned) {
				t.Fatalf("full-payload read error = %v, want pruned-payload error", err)
			}
			all, err := repo.ListAgentDeliverySubmissions(ctx, "retention-page-session")
			if err != nil || len(all) != len(states) {
				t.Fatalf("legacy metadata list = %d rows, err %v", len(all), err)
			}
			for _, submission := range all {
				if len(submission.Payload) != 0 {
					t.Fatalf("legacy list loaded prompt body for %q", submission.ID)
				}
			}
		})
	}
}

func TestDeliveryRetentionCompatibilityReader(t *testing.T) {
	for _, engine := range []string{"SQLite", "Postgres"} {
		t.Run(engine, func(t *testing.T) {
			repo := newRetentionTestRepo(t, engine)
			seedAgentDeliveryTestSession(t, repo, "retention-compat-session", "retention-compat-incarnation")
			ctx := context.Background()
			first := &models.AgentDeliveryEvent{
				SessionID: "retention-compat-session", IncarnationID: "retention-compat-incarnation",
				HarnessGeneration: 2, StreamID: "retention-compat-stream", Sequence: 1,
				EventType: "assistant_message", Payload: []byte("legacy payload"),
			}
			if inserted, err := repo.ReceiveAgentDeliveryEvent(ctx, first, 2); err != nil || !inserted {
				t.Fatalf("write format-1 event = %v, err %v", inserted, err)
			}
			if _, err := repo.db.Exec(repo.db.Rebind(`
				UPDATE agent_delivery_inbox SET content_hash = '', hash_version = 0
				WHERE stream_id = ? AND sequence = 1`), first.StreamID); err != nil {
				t.Fatalf("shape legacy receipt: %v", err)
			}
			legacyReplay := *first
			if inserted, err := repo.ReceiveAgentDeliveryEvent(ctx, &legacyReplay, 2); err != nil || inserted {
				t.Fatalf("read old un-hashed receipt = %v, err %v", inserted, err)
			}

			if _, err := repo.db.Exec(repo.db.Rebind(`
				UPDATE agent_delivery_cursors SET retired_sequence = 1 WHERE stream_id = ?`), first.StreamID); err != nil {
				t.Fatalf("set retained checkpoint: %v", err)
			}
			if _, err := repo.db.Exec(repo.db.Rebind(`
				INSERT INTO agent_delivery_cleanup_jobs
				(session_id, incarnation_id, retired_at, phase, created_at, updated_at)
				VALUES (?, ?, ?, ?, ?, ?)`), first.SessionID, first.IncarnationID, time.Now().UTC(), "pending", time.Now().UTC(), time.Now().UTC()); err != nil {
				t.Fatalf("seed pending retirement job: %v", err)
			}
			seedAgentDeliveryTestSession(t, repo, "retention-compat-other-session", "retention-compat-other-incarnation")
			second := *first
			second.SessionID = "retention-compat-other-session"
			second.IncarnationID = "retention-compat-other-incarnation"
			second.StreamID = "retention-compat-other-stream"
			second.Sequence = 1
			second.Payload = []byte("new format-1 payload")
			if inserted, err := repo.ReceiveAgentDeliveryEvent(ctx, &second, 2); err != nil || !inserted {
				t.Fatalf("write after rollback-shaped state = %v, err %v", inserted, err)
			}
			var hashVersion int
			if err := repo.db.QueryRowx(repo.db.Rebind(`
					SELECT hash_version FROM agent_delivery_inbox WHERE stream_id = ? AND sequence = 1`), second.StreamID).Scan(&hashVersion); err != nil {
				t.Fatalf("read new event hash version: %v", err)
			}
			if hashVersion != agentDeliveryPayloadHashVersion {
				t.Errorf("new event hash version = %d, want %d", hashVersion, agentDeliveryPayloadHashVersion)
			}
			assertRetentionRowCount(t, repo, "agent_delivery_cleanup_jobs", "session_id = ?", first.SessionID, 1)
			cursor, err := repo.GetAgentDeliveryCursor(ctx, second.StreamID)
			if err != nil || cursor.RetiredSequence != 0 || cursor.ReceivedSequence != 1 {
				t.Fatalf("cursor after new write = %+v, err %v", cursor, err)
			}
		})
	}
}

func TestDeliveryRetentionRetirementAdmission(t *testing.T) {
	for _, engine := range []string{"SQLite", "Postgres"} {
		t.Run(engine, func(t *testing.T) {
			repo := newRetentionTestRepo(t, engine)
			ctx := context.Background()
			const (
				taskID        = "retention-admission-task"
				sessionID     = "retention-admission-session"
				incarnationID = "retention-admission-incarnation"
			)
			if err := repo.CreateTask(ctx, &models.Task{ID: taskID, Title: "retention admission"}); err != nil {
				t.Fatalf("create task: %v", err)
			}
			seedRetentionAdmissionJob(t, repo, sessionID, incarnationID)
			staleSubmission := &models.AgentDeliverySubmission{
				ID: "prompt:retention-retired-recreation", SessionID: sessionID,
				IncarnationID: incarnationID, HarnessGeneration: 1, OwnerGeneration: 1,
				PayloadHash: "retired-prompt-hash", Payload: []byte("retired prompt"),
				State: models.DeliverySubmissionPrepared,
			}
			if _, err := repo.PrepareAgentDeliverySubmission(ctx, staleSubmission); !errors.Is(err, repoerrors.ErrAgentDeliveryIdentityRetiring) {
				t.Fatalf("prepare prompt for retired session = %v, want retiring identity", err)
			}
			staleEvent := &models.AgentDeliveryEvent{
				SessionID: sessionID, IncarnationID: incarnationID, HarnessGeneration: 1,
				StreamID: "retention-retired-stream", Sequence: 1,
				EventType: "assistant_message", Payload: []byte("retired event"),
			}
			if inserted, err := repo.ReceiveAgentDeliveryEvent(ctx, staleEvent, 1); !errors.Is(err, repoerrors.ErrAgentDeliveryIdentityRetiring) || inserted {
				t.Fatalf("receive for retired session = %v, err %v", inserted, err)
			}
			assertRetentionRowCount(t, repo, "agent_delivery_submissions", "id = ?", staleSubmission.ID, 0)
			assertRetentionRowCount(t, repo, "agent_delivery_inbox", "stream_id = ?", staleEvent.StreamID, 0)
			assertRetentionRowCount(t, repo, "agent_delivery_cursors", "stream_id = ?", staleEvent.StreamID, 0)
			creators := []struct {
				name   string
				create func(*models.TaskSession) error
			}{
				{name: "ordinary", create: func(session *models.TaskSession) error { return repo.CreateTaskSession(ctx, session) }},
				{name: "initial_seed", create: func(session *models.TaskSession) error {
					return repo.CreateTaskSessionWithInitialRuntimeSeed(ctx, session)
				}},
			}
			for _, creator := range creators {
				t.Run(creator.name, func(t *testing.T) {
					err := creator.create(&models.TaskSession{ID: sessionID, TaskID: taskID})
					if !errors.Is(err, repoerrors.ErrAgentDeliveryIdentityRetiring) {
						t.Fatalf("create retired session identity error = %v", err)
					}
				})
			}
			assertRetentionRowCount(t, repo, "task_sessions", "id = ?", sessionID, 0)
			t.Run("managed_primary", func(t *testing.T) {
				const workspaceID, managedTaskID, primaryID = "retention-managed-workspace", "retention-managed-task", "retention-managed-primary"
				if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: workspaceID, Name: "Retention"}); err != nil {
					t.Fatalf("create managed workspace: %v", err)
				}
				seedRetentionAdmissionJob(t, repo, primaryID, "retention-managed-incarnation")
				identity := managed.Identity{
					TaskID: managedTaskID, InstallationID: "retention-installation",
					WorkspaceID: workspaceID, InstanceKey: "retention-instance",
				}
				managedTask := &models.Task{
					ID: managedTaskID, WorkspaceID: workspaceID, Title: "Managed retention",
					Metadata: map[string]interface{}{
						models.MetaKeyManagedRetained: true, "kandev.ephemeral": true,
						models.MetaKeyManagedInstallationID: identity.InstallationID,
						"kandev.workspace_id":               identity.WorkspaceID,
						models.MetaKeyManagedInstanceKey:    identity.InstanceKey,
					},
				}
				_, err := repo.EnsureManagedConversation(ctx, managed.EnsureRequest{
					Identity: identity, Task: managedTask, PrimaryID: primaryID,
					Configuration: managed.Configuration{BasePrompt: "managed retention"},
					OperationID:   "retention-create", PayloadDigest: "retention-digest",
				})
				if !errors.Is(err, repoerrors.ErrAgentDeliveryIdentityRetiring) {
					t.Fatalf("managed primary creation error = %v, want retiring identity", err)
				}
				assertRetentionRowCount(t, repo, "tasks", "id = ?", managedTaskID, 0)
				assertRetentionRowCount(t, repo, "task_sessions", "id = ?", primaryID, 0)
			})
		})
	}
}

func seedRetentionAdmissionJob(t *testing.T, repo *Repository, sessionID, incarnationID string) {
	t.Helper()
	now := time.Now().UTC()
	if _, err := repo.db.Exec(repo.db.Rebind(`
		INSERT INTO agent_delivery_cleanup_jobs
		(session_id, incarnation_id, retired_at, phase, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)`), sessionID, incarnationID, now, "pending", now, now); err != nil {
		t.Fatalf("seed retirement job for %s: %v", sessionID, err)
	}
}
