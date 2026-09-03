package sqlite

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/task/models"
)

func TestPermissionResolutionClaimAndFinalizeCAS(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedForMsgTest(t, repo, "task-permission-cas", "session-permission-cas", "turn-permission-cas")
	message := &models.Message{
		ID:            "message-permission-cas",
		TaskID:        "task-permission-cas",
		TaskSessionID: "session-permission-cas",
		TurnID:        "turn-permission-cas",
		AuthorType:    models.MessageAuthorAgent,
		Type:          models.MessageTypePermissionRequest,
		Metadata: map[string]any{
			"request_id":     "request-1",
			"pending_id":     "pending-1",
			"action_details": map[string]any{"env": map[string]any{"API_TOKEN": "secret-canary"}},
		},
	}
	if err := repo.CreateMessage(ctx, message); err != nil {
		t.Fatal(err)
	}
	selectedAt := time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC)
	claim := models.PermissionResolutionClaimRequest{
		TaskID:    message.TaskID,
		SessionID: message.TaskSessionID,
		Audit: models.PermissionResolutionAudit{
			ClaimID:     "claim-1",
			ActorUserID: "user-1",
			ActorKind:   models.PermissionActorPersonalAccessToken,
			Source:      models.PermissionSourceExternalMCP,
			RequestID:   "request-1",
			PendingID:   "pending-1",
			OptionID:    "allow-once",
			OptionKind:  "allow_once",
			SelectedAt:  selectedAt,
		},
	}

	claimed, err := repo.ClaimPermissionResolution(ctx, claim)
	if err != nil || claimed.Outcome != models.PermissionClaimed {
		t.Fatalf("claim = %+v, err=%v", claimed, err)
	}
	competing := claim
	competing.Audit.ClaimID = "claim-2"
	result, err := repo.ClaimPermissionResolution(ctx, competing)
	if err != nil || result.Outcome != models.PermissionClaimInProgress {
		t.Fatalf("competing claim = %+v, err=%v", result, err)
	}

	wrongClaim, err := repo.FinalizePermissionResolution(ctx, models.PermissionResolutionFinalizeRequest{
		TaskID:      message.TaskID,
		SessionID:   message.TaskSessionID,
		RequestID:   "request-1",
		PendingID:   "pending-1",
		ClaimID:     "claim-2",
		Result:      models.PermissionResolutionAccepted,
		Status:      models.PermissionStatusApproved,
		FinalizedAt: selectedAt.Add(time.Second),
	})
	if err != nil || wrongClaim.Outcome != models.PermissionFinalizeClaimMismatch {
		t.Fatalf("wrong claim finalize = %+v, err=%v", wrongClaim, err)
	}

	finalized, err := repo.FinalizePermissionResolution(ctx, models.PermissionResolutionFinalizeRequest{
		TaskID:      message.TaskID,
		SessionID:   message.TaskSessionID,
		RequestID:   "request-1",
		PendingID:   "pending-1",
		ClaimID:     "claim-1",
		Result:      models.PermissionResolutionAccepted,
		Status:      models.PermissionStatusApproved,
		FinalizedAt: selectedAt.Add(time.Second),
	})
	if err != nil || finalized.Outcome != models.PermissionFinalized {
		t.Fatalf("finalize = %+v, err=%v", finalized, err)
	}
	audit, ok := permissionAuditFromMetadata(finalized.Message.Metadata)
	if !ok || audit.ActorUserID != "user-1" || audit.Result != models.PermissionResolutionAccepted || audit.FinalizedAt == nil {
		t.Fatalf("unexpected audit: %+v", audit)
	}
	encoded, err := json.Marshal(audit)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "secret-canary") || strings.Contains(string(encoded), "action_details") {
		t.Fatalf("audit leaked action details: %s", encoded)
	}

	replayed, err := repo.ClaimPermissionResolution(ctx, competing)
	if err != nil || replayed.Outcome != models.PermissionClaimAlreadyFinal {
		t.Fatalf("replayed claim = %+v, err=%v", replayed, err)
	}
	missing := claim
	missing.Audit.RequestID = "request-missing"
	notFound, err := repo.ClaimPermissionResolution(ctx, missing)
	if err != nil || notFound.Outcome != models.PermissionClaimNotFound {
		t.Fatalf("missing claim = %+v, err=%v", notFound, err)
	}
}

func TestPermissionResolutionPostgresExpressionsUseJSONB(t *testing.T) {
	for name, expression := range map[string]string{
		"claim":    permissionClaimJSONExpression("pgx"),
		"finalize": permissionFinalizeJSONExpression("pgx"),
		"extract":  permissionJSONExtract("pgx", "metadata", "permission_resolution", "claim_id"),
	} {
		if strings.Contains(expression, "json_set") || strings.Contains(expression, "json_extract") {
			t.Fatalf("%s expression uses SQLite JSON: %s", name, expression)
		}
		if !strings.Contains(expression, "jsonb") {
			t.Fatalf("%s expression does not use PostgreSQL JSONB: %s", name, expression)
		}
	}
	extract := permissionJSONExtract("pgx", "metadata", "permission_resolution", "claim_id")
	if !strings.Contains(extract, "COALESCE(NULLIF(metadata, ''), '{}')::jsonb") {
		t.Fatalf("extract expression does not guard empty metadata: %s", extract)
	}
}

func TestPermissionResolutionClaimIgnoresInvalidSQLiteMetadataRows(t *testing.T) {
	for _, metadata := range []string{"", "not-json"} {
		t.Run(fmt.Sprintf("metadata_%q", metadata), func(t *testing.T) {
			repo := newRepoForSessionTests(t)
			ctx := context.Background()
			seedForMsgTest(t, repo, "task-invalid-metadata", "session-invalid-metadata", "turn-invalid-metadata")
			if err := repo.CreateMessage(ctx, &models.Message{
				ID: "permission-valid", TaskID: "task-invalid-metadata",
				TaskSessionID: "session-invalid-metadata", TurnID: "turn-invalid-metadata",
				AuthorType: models.MessageAuthorAgent, Type: models.MessageTypePermissionRequest,
				Metadata: map[string]any{"request_id": "request-valid", "pending_id": "pending-valid"},
			}); err != nil {
				t.Fatalf("create valid permission: %v", err)
			}
			// Simulate a legacy database that predates the JSON expression indexes;
			// current indexes reject malformed metadata before the claim path runs.
			for _, index := range []string{"idx_messages_metadata_tool_call_id", "idx_messages_metadata_pending_id"} {
				if _, err := repo.db.Exec("DROP INDEX " + index); err != nil {
					t.Fatalf("drop %s: %v", index, err)
				}
			}
			now := time.Now().UTC()
			if _, err := repo.db.Exec(repo.db.Rebind(`
				INSERT INTO task_session_messages
					(id, task_session_id, task_id, turn_id, author_type, author_id, content,
					 requests_input, type, metadata, created_at, updated_at)
				VALUES (?, ?, ?, ?, 'agent', '', '', 0, 'permission_request', ?, ?, ?)
			`), "permission-invalid", "session-invalid-metadata", "task-invalid-metadata",
				"turn-invalid-metadata", metadata, now, now); err != nil {
				t.Fatalf("seed invalid metadata: %v", err)
			}

			claimed, err := repo.ClaimPermissionResolution(ctx, models.PermissionResolutionClaimRequest{
				TaskID: "task-invalid-metadata", SessionID: "session-invalid-metadata",
				Audit: models.PermissionResolutionAudit{
					ClaimID: "claim-valid", ActorKind: models.PermissionActorSynthetic,
					Source: models.PermissionSourceAutomation, RequestID: "request-valid",
					PendingID: "pending-valid", OptionID: "allow-once", OptionKind: "allow_once",
					SelectedAt: now,
				},
			})
			if err != nil || claimed == nil || claimed.Outcome != models.PermissionClaimed {
				t.Fatalf("claim = %+v, err=%v, want valid permission claimed", claimed, err)
			}
		})
	}
}

func TestGetPermissionResolutionAuditMissingReturnsNil(t *testing.T) {
	repo := newRepoForSessionTests(t)
	seedForMsgTest(t, repo, "task-permission-audit-missing", "session-permission-audit-missing", "turn-permission-audit-missing")

	audit, err := repo.GetPermissionResolutionAudit(
		context.Background(),
		"task-permission-audit-missing",
		"session-permission-audit-missing",
		"request-missing",
		"pending-missing",
	)
	if err != nil || audit != nil {
		t.Fatalf("audit=%+v err=%v, want nil, nil for a missing permission row", audit, err)
	}
}

func TestPermissionResolutionConcurrentClaimsHaveOneWinner(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedForMsgTest(t, repo, "task-permission-race", "session-permission-race", "turn-permission-race")
	if err := repo.CreateMessage(ctx, &models.Message{
		ID:            "message-permission-race",
		TaskID:        "task-permission-race",
		TaskSessionID: "session-permission-race",
		TurnID:        "turn-permission-race",
		AuthorType:    models.MessageAuthorAgent,
		Type:          models.MessageTypePermissionRequest,
		Metadata: map[string]any{
			"request_id": "request-race",
			"pending_id": "pending-race",
		},
	}); err != nil {
		t.Fatal(err)
	}

	outcomes := make(chan models.PermissionResolutionClaimOutcome, 2)
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for _, claimID := range []string{"claim-a", "claim-b"} {
		claimID := claimID
		wg.Add(1)
		go func() {
			defer wg.Done()
			result, err := repo.ClaimPermissionResolution(ctx, models.PermissionResolutionClaimRequest{
				TaskID:    "task-permission-race",
				SessionID: "session-permission-race",
				Audit: models.PermissionResolutionAudit{
					ClaimID:    claimID,
					ActorKind:  models.PermissionActorSynthetic,
					Source:     models.PermissionSourceExternalMCP,
					RequestID:  "request-race",
					PendingID:  "pending-race",
					OptionID:   "allow-once",
					OptionKind: "allow_once",
					SelectedAt: time.Now().UTC(),
				},
			})
			if err != nil {
				errs <- err
				return
			}
			outcomes <- result.Outcome
		}()
	}
	wg.Wait()
	close(outcomes)
	close(errs)
	for err := range errs {
		t.Fatalf("concurrent claim: %v", err)
	}
	counts := map[models.PermissionResolutionClaimOutcome]int{}
	for outcome := range outcomes {
		counts[outcome]++
	}
	if counts[models.PermissionClaimed] != 1 || counts[models.PermissionClaimInProgress] != 1 {
		t.Fatalf("outcomes = %+v, want one claimed and one in progress", counts)
	}
}

// insertMsgWithType inserts a message row with a configurable type column,
// so tests can mix tool_call and plain message rows in the same session.
func insertMsgWithType(t *testing.T, repo *Repository, id, sessionID, turnID, msgType string, ts time.Time) {
	t.Helper()
	_, err := repo.db.Exec(repo.db.Rebind(`
		INSERT INTO task_session_messages
			(id, task_session_id, task_id, turn_id, author_type, author_id, content, requests_input, type, metadata, created_at)
		VALUES (?, ?, '', ?, 'agent', '', '', 0, ?, '{}', ?)
	`), id, sessionID, turnID, msgType, ts)
	if err != nil {
		t.Fatalf("insert message %s: %v", id, err)
	}
}

func TestListMessagesPaginatedFiltersUserAuthors(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedForMsgTest(t, repo, "task-filter", "sess-filter", "turn-filter")
	now := time.Now().UTC()
	for _, message := range []*models.Message{
		{ID: "user-1", TaskSessionID: "sess-filter", TaskID: "task-filter", TurnID: "turn-filter", AuthorType: models.MessageAuthorUser, Type: models.MessageTypeMessage, CreatedAt: now},
		{ID: "agent-1", TaskSessionID: "sess-filter", TaskID: "task-filter", TurnID: "turn-filter", AuthorType: models.MessageAuthorAgent, Type: models.MessageTypeMessage, CreatedAt: now.Add(time.Second)},
		{ID: "user-2", TaskSessionID: "sess-filter", TaskID: "task-filter", TurnID: "turn-filter", AuthorType: models.MessageAuthorUser, Type: models.MessageTypeMessage, CreatedAt: now.Add(2 * time.Second)},
	} {
		require.NoError(t, repo.CreateMessage(ctx, message))
	}

	page, hasMore, err := repo.ListMessagesPaginated(ctx, "sess-filter", models.ListMessagesOptions{
		AuthorType: string(models.MessageAuthorUser), Limit: 2,
	})

	require.NoError(t, err)
	require.False(t, hasMore)
	require.Equal(t, []string{"user-1", "user-2"}, messageIDs(page))
}

func TestListMessagesPaginatedAroundIncludesTarget(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedForMsgTest(t, repo, "task-around", "sess-around", "turn-around")
	now := time.Now().UTC()
	for index, id := range []string{"m1", "m2", "m3", "m4"} {
		require.NoError(t, repo.CreateMessage(ctx, &models.Message{
			ID: id, TaskSessionID: "sess-around", TaskID: "task-around", TurnID: "turn-around",
			AuthorType: models.MessageAuthorAgent, Type: models.MessageTypeMessage,
			CreatedAt: now.Add(time.Duration(index) * time.Second),
		}))
	}

	page, hasMore, err := repo.ListMessagesPaginated(ctx, "sess-around", models.ListMessagesOptions{
		Around: "m2", Limit: 2, Sort: "desc",
	})

	require.NoError(t, err)
	require.True(t, hasMore)
	require.Equal(t, []string{"m3", "m2"}, messageIDs(page))
}

func TestListMessagesByTurnID(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	now := time.Now().UTC()
	seedForMsgTest(t, repo, "task-T", "sess-T", "turn-1")
	seedForMsgTest(t, repo, "task-T2", "sess-T", "turn-2")

	// Two messages on turn-1 (out of insertion order to check created_at sort)
	// and one on turn-2 in the same session.
	insertMsgWithType(t, repo, "m-b", "sess-T", "turn-1", "message", now.Add(2*time.Second))
	insertMsgWithType(t, repo, "m-a", "sess-T", "turn-1", "tool_call", now)
	insertMsgWithType(t, repo, "m-other", "sess-T", "turn-2", "message", now.Add(time.Second))

	got, err := repo.ListMessagesByTurnID(ctx, "turn-1")
	if err != nil {
		t.Fatalf("ListMessagesByTurnID: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 messages for turn-1, got %d", len(got))
	}
	if got[0].ID != "m-a" || got[1].ID != "m-b" {
		t.Errorf("expected [m-a, m-b] ordered by created_at, got [%s, %s]", got[0].ID, got[1].ID)
	}
	for _, m := range got {
		if m.TurnID != "turn-1" {
			t.Errorf("message %s has turn_id %q, want turn-1", m.ID, m.TurnID)
		}
	}

	empty, err := repo.ListMessagesByTurnID(ctx, "turn-missing")
	if err != nil {
		t.Fatalf("ListMessagesByTurnID(missing): %v", err)
	}
	if len(empty) != 0 {
		t.Errorf("expected no messages for unknown turn, got %d", len(empty))
	}
}

func TestUpdateMessageBumpsUpdatedAt(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedForMsgTest(t, repo, "task-U", "sess-U", "turn-U")

	created := time.Now().UTC().Add(-time.Hour)
	msg := &models.Message{
		ID:            "m-u",
		TaskSessionID: "sess-U",
		TurnID:        "turn-U",
		AuthorType:    models.MessageAuthorAgent,
		Content:       "hello",
		Type:          models.MessageTypeMessage,
		CreatedAt:     created,
	}
	if err := repo.CreateMessage(ctx, msg); err != nil {
		t.Fatalf("CreateMessage: %v", err)
	}

	// On insert, updated_at defaults to created_at.
	got, err := repo.GetMessage(ctx, "m-u")
	if err != nil {
		t.Fatalf("GetMessage: %v", err)
	}
	if !got.UpdatedAt.Equal(got.CreatedAt) {
		t.Errorf("after create, updated_at = %v, want == created_at %v", got.UpdatedAt, got.CreatedAt)
	}

	// Update advances updated_at past created_at.
	msg.Content = "hello world"
	if err := repo.UpdateMessage(ctx, msg); err != nil {
		t.Fatalf("UpdateMessage: %v", err)
	}
	got, err = repo.GetMessage(ctx, "m-u")
	if err != nil {
		t.Fatalf("GetMessage after update: %v", err)
	}
	if !got.UpdatedAt.After(got.CreatedAt) {
		t.Errorf("after update, updated_at = %v, want after created_at %v", got.UpdatedAt, got.CreatedAt)
	}
}

func TestCountToolCallMessagesBySession_Empty(t *testing.T) {
	repo := newRepoForSessionTests(t)
	got, err := repo.CountToolCallMessagesBySession(context.Background(), nil)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected empty map, got %d entries", len(got))
	}
}

func TestCountToolCallMessagesBySession_Single(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	now := time.Now().UTC()
	seedForMsgTest(t, repo, "task-A", "sess-A", "turn-A")
	insertMsgWithType(t, repo, "m1", "sess-A", "turn-A", "tool_call", now)
	insertMsgWithType(t, repo, "m2", "sess-A", "turn-A", "tool_call", now.Add(time.Second))
	insertMsgWithType(t, repo, "m3", "sess-A", "turn-A", "message", now.Add(2*time.Second))

	got, err := repo.CountToolCallMessagesBySession(ctx, []string{"sess-A"})
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if got["sess-A"] != 2 {
		t.Errorf("sess-A count = %d, want 2", got["sess-A"])
	}
}

func TestCountToolCallMessagesBySession_Multi(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	now := time.Now().UTC()
	seedForMsgTest(t, repo, "task-1", "s1", "turn-1")
	seedForMsgTest(t, repo, "task-2", "s2", "turn-2")
	seedForMsgTest(t, repo, "task-3", "s3", "turn-3")
	insertMsgWithType(t, repo, "m-s1-a", "s1", "turn-1", "tool_call", now)
	insertMsgWithType(t, repo, "m-s2-a", "s2", "turn-2", "tool_call", now)
	insertMsgWithType(t, repo, "m-s2-b", "s2", "turn-2", "tool_call", now.Add(time.Second))
	insertMsgWithType(t, repo, "m-s2-c", "s2", "turn-2", "tool_call", now.Add(2*time.Second))
	// s3 has only a plain message — must be omitted from the result map.
	insertMsgWithType(t, repo, "m-s3-a", "s3", "turn-3", "message", now)

	got, err := repo.CountToolCallMessagesBySession(ctx, []string{"s1", "s2", "s3"})
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if got["s1"] != 1 {
		t.Errorf("s1 count = %d, want 1", got["s1"])
	}
	if got["s2"] != 3 {
		t.Errorf("s2 count = %d, want 3", got["s2"])
	}
	if _, ok := got["s3"]; ok {
		t.Errorf("s3 should be omitted (zero tool_call rows), got %d", got["s3"])
	}
}

func createPendingActionMessage(
	t *testing.T,
	repo *Repository,
	id string,
	taskID string,
	sessionID string,
	turnID string,
	msgType models.MessageType,
	status string,
	createdAt time.Time,
) {
	t.Helper()
	metadata := map[string]interface{}{}
	if status != "<missing>" {
		metadata["status"] = status
	}
	if err := repo.CreateMessage(context.Background(), &models.Message{
		ID:            id,
		TaskSessionID: sessionID,
		TaskID:        taskID,
		TurnID:        turnID,
		AuthorType:    models.MessageAuthorAgent,
		Content:       id,
		Type:          msgType,
		Metadata:      metadata,
		CreatedAt:     createdAt,
	}); err != nil {
		t.Fatalf("CreateMessage(%s): %v", id, err)
	}
}

func TestGetPendingActionsBySessionIDs(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	now := time.Now().UTC()

	seedForMsgTest(t, repo, "task-clar", "sess-clar", "turn-clar")
	// Transitional history may contain both kinds. Clarification deliberately
	// wins within one session, independent of UNION ALL row order.
	createPendingActionMessage(t, repo, "perm-clar", "task-clar", "sess-clar", "turn-clar", models.MessageTypePermissionRequest, "<missing>", now)
	createPendingActionMessage(t, repo, "clar-clar", "task-clar", "sess-clar", "turn-clar", models.MessageTypeClarificationRequest, "pending", now.Add(time.Second))

	seedForMsgTest(t, repo, "task-resolved", "sess-resolved", "turn-resolved")
	createPendingActionMessage(t, repo, "perm-old", "task-resolved", "sess-resolved", "turn-resolved", models.MessageTypePermissionRequest, "pending", now)
	createPendingActionMessage(t, repo, "perm-new", "task-resolved", "sess-resolved", "turn-resolved", models.MessageTypePermissionRequest, "approved", now.Add(time.Second))

	seedForMsgTest(t, repo, "task-perm", "sess-perm", "turn-perm")
	createPendingActionMessage(t, repo, "perm-pending", "task-perm", "sess-perm", "turn-perm", models.MessageTypePermissionRequest, "pending", now)

	seedForMsgTest(t, repo, "task-perm-tie", "sess-perm-tie", "turn-perm-tie")
	createPendingActionMessage(t, repo, "z-approved", "task-perm-tie", "sess-perm-tie", "turn-perm-tie", models.MessageTypePermissionRequest, "approved", now)
	createPendingActionMessage(t, repo, "a-pending", "task-perm-tie", "sess-perm-tie", "turn-perm-tie", models.MessageTypePermissionRequest, "pending", now)

	seedForMsgTest(t, repo, "task-stale", "sess-stale", "turn-stale")
	createPendingActionMessage(t, repo, "perm-stale", "task-stale", "sess-stale", "turn-stale", models.MessageTypePermissionRequest, "pending", now)
	createPendingActionMessage(t, repo, "clar-stale", "task-stale", "sess-stale", "turn-stale", models.MessageTypeClarificationRequest, "pending", now)
	seedForMsgTest(t, repo, "task-stale", "sess-stale", "turn-current")
	createPendingActionMessage(t, repo, "message-current", "task-stale", "sess-stale", "turn-current", models.MessageTypeMessage, "<missing>", now.Add(time.Second))

	seedForMsgTest(t, repo, "task-reserved", "sess-reserved", "turn-reserved-old")
	createPendingActionMessage(t, repo, "clar-reserved", "task-reserved", "sess-reserved", "turn-reserved-old", models.MessageTypeClarificationRequest, "pending", now)
	if err := repo.CreateTurn(ctx, &models.Turn{
		ID:            "turn-reserved-empty",
		TaskSessionID: "sess-reserved",
		TaskID:        "task-reserved",
		StartedAt:     now.Add(time.Second),
		CreatedAt:     now.Add(time.Second),
	}); err != nil {
		t.Fatalf("CreateTurn(reserved empty): %v", err)
	}

	got, err := repo.GetPendingActionsBySessionIDs(ctx, []string{
		"sess-clar",
		"sess-resolved",
		"sess-perm",
		"sess-perm-tie",
		"sess-stale",
		"sess-reserved",
		"sess-missing",
	})
	if err != nil {
		t.Fatalf("GetPendingActionsBySessionIDs: %v", err)
	}
	if got["sess-clar"] != models.TaskPendingActionClarification {
		t.Fatalf("sess-clar action = %q, want clarification", got["sess-clar"])
	}
	if _, ok := got["sess-resolved"]; ok {
		t.Fatalf("sess-resolved should not have a pending action: %#v", got["sess-resolved"])
	}
	if got["sess-perm"] != models.TaskPendingActionPermission {
		t.Fatalf("sess-perm action = %q, want permission", got["sess-perm"])
	}
	if got["sess-perm-tie"] != models.TaskPendingActionPermission {
		t.Fatalf("sess-perm-tie action = %q, want permission from last inserted row", got["sess-perm-tie"])
	}
	if _, ok := got["sess-stale"]; ok {
		t.Fatalf("sess-stale should not inherit previous turn actions: %#v", got["sess-stale"])
	}
	if _, ok := got["sess-reserved"]; ok {
		t.Fatalf("sess-reserved should not inherit actions while its newest turn is empty: %#v", got["sess-reserved"])
	}
	if _, ok := got["sess-missing"]; ok {
		t.Fatalf("sess-missing should not have a pending action: %#v", got["sess-missing"])
	}
}

// insertPluginMsg inserts a fully-specified message row (task_id, type,
// author, content, created_at all controllable) for ListMessagesForPlugin
// filter tests.
func insertPluginMsg(t *testing.T, repo *Repository, id, sessionID, taskID, turnID, authorType, msgType, content string, ts time.Time) {
	t.Helper()
	_, err := repo.db.Exec(repo.db.Rebind(`
		INSERT INTO task_session_messages
			(id, task_session_id, task_id, turn_id, author_type, author_id, content, requests_input, type, metadata, created_at)
		VALUES (?, ?, ?, ?, ?, '', ?, 0, ?, '{}', ?)
	`), id, sessionID, taskID, turnID, authorType, content, msgType, ts)
	if err != nil {
		t.Fatalf("insert plugin message %s: %v", id, err)
	}
}

func TestListMessagesForPlugin(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	base := time.Date(2026, 7, 20, 9, 0, 0, 0, time.UTC)

	seedForMsgTest(t, repo, "task-A", "sess-A", "turn-A")
	seedForMsgTest(t, repo, "task-B", "sess-B", "turn-B")

	// sess-A / task-A: three messages across three days.
	insertPluginMsg(t, repo, "a1", "sess-A", "task-A", "turn-A", "user", "message", "day one", base)
	insertPluginMsg(t, repo, "a2", "sess-A", "task-A", "turn-A", "agent", "message", "day two", base.Add(24*time.Hour))
	insertPluginMsg(t, repo, "a3", "sess-A", "task-A", "turn-A", "agent", "thinking", "day three", base.Add(48*time.Hour))
	// sess-B / task-B: one message on day one.
	insertPluginMsg(t, repo, "b1", "sess-B", "task-B", "turn-B", "user", "message", "other session", base)

	t.Run("by session id", func(t *testing.T) {
		got, err := repo.ListMessagesForPlugin(ctx, models.PluginMessageFilter{SessionIDs: []string{"sess-A"}})
		if err != nil {
			t.Fatalf("ListMessagesForPlugin: %v", err)
		}
		if len(got) != 3 || got[0].ID != "a1" || got[2].ID != "a3" {
			t.Fatalf("got %d messages ordered %v, want a1,a2,a3", len(got), ids(got))
		}
	})

	t.Run("by task id", func(t *testing.T) {
		got, err := repo.ListMessagesForPlugin(ctx, models.PluginMessageFilter{TaskIDs: []string{"task-B"}})
		if err != nil {
			t.Fatalf("ListMessagesForPlugin: %v", err)
		}
		if len(got) != 1 || got[0].ID != "b1" {
			t.Fatalf("got %v, want [b1]", ids(got))
		}
	})

	t.Run("time range excludes out-of-window", func(t *testing.T) {
		since := base.Add(24 * time.Hour) // inclusive → a2 kept
		until := base.Add(48 * time.Hour) // exclusive → a3 dropped
		got, err := repo.ListMessagesForPlugin(ctx, models.PluginMessageFilter{
			SessionIDs: []string{"sess-A"}, Since: &since, Until: &until,
		})
		if err != nil {
			t.Fatalf("ListMessagesForPlugin: %v", err)
		}
		if len(got) != 1 || got[0].ID != "a2" {
			t.Fatalf("got %v, want [a2] (since inclusive, until exclusive)", ids(got))
		}
	})

	t.Run("type filter", func(t *testing.T) {
		got, err := repo.ListMessagesForPlugin(ctx, models.PluginMessageFilter{Types: []string{"thinking"}})
		if err != nil {
			t.Fatalf("ListMessagesForPlugin: %v", err)
		}
		if len(got) != 1 || got[0].ID != "a3" {
			t.Fatalf("got %v, want [a3]", ids(got))
		}
	})

	t.Run("limit and offset paginate in order", func(t *testing.T) {
		page1, err := repo.ListMessagesForPlugin(ctx, models.PluginMessageFilter{SessionIDs: []string{"sess-A"}, Limit: 2, Offset: 0})
		if err != nil {
			t.Fatalf("page1: %v", err)
		}
		if len(page1) != 2 || page1[0].ID != "a1" || page1[1].ID != "a2" {
			t.Fatalf("page1 = %v, want [a1 a2]", ids(page1))
		}
		page2, err := repo.ListMessagesForPlugin(ctx, models.PluginMessageFilter{SessionIDs: []string{"sess-A"}, Limit: 2, Offset: 2})
		if err != nil {
			t.Fatalf("page2: %v", err)
		}
		if len(page2) != 1 || page2[0].ID != "a3" {
			t.Fatalf("page2 = %v, want [a3]", ids(page2))
		}
	})
}

func ids(msgs []*models.Message) []string {
	out := make([]string, len(msgs))
	for i, m := range msgs {
		out[i] = m.ID
	}
	return out
}
