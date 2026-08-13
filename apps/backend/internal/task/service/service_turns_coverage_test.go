package service

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
)

func TestGetAndListTurns(t *testing.T) {
	svc, _, repo := newMessageTestService(t)
	ctx := context.Background()
	seedTurn(t, repo, "turn-second", "sess-msg", "task-msg")

	turn, err := svc.GetTurn(ctx, "turn-msg")
	if err != nil {
		t.Fatalf("GetTurn: %v", err)
	}
	if turn.ID != "turn-msg" || turn.TaskSessionID != "sess-msg" || turn.TaskID != "task-msg" {
		t.Fatalf("turn = %+v", turn)
	}

	turns, err := svc.ListTurnsBySession(ctx, "sess-msg")
	if err != nil {
		t.Fatalf("ListTurnsBySession: %v", err)
	}
	if len(turns) != 2 {
		t.Fatalf("turns = %d, want 2", len(turns))
	}
	ids := map[string]bool{turns[0].ID: true, turns[1].ID: true}
	if !ids["turn-msg"] || !ids["turn-second"] {
		t.Fatalf("turn ids = %v, want both seeded turns", ids)
	}

	empty, err := svc.ListTurnsBySession(ctx, "sess-none")
	if err != nil {
		t.Fatalf("ListTurnsBySession for an unknown session: %v", err)
	}
	if len(empty) != 0 {
		t.Fatalf("turns for an unknown session = %d, want 0", len(empty))
	}
}

func TestUpdateTurnPersistsChangesAndIgnoresNil(t *testing.T) {
	svc, _, repo := newMessageTestService(t)
	ctx := context.Background()

	if err := svc.UpdateTurn(ctx, nil); err != nil {
		t.Fatalf("UpdateTurn(nil) must be a no-op, got %v", err)
	}

	turn, err := svc.GetTurn(ctx, "turn-msg")
	if err != nil {
		t.Fatalf("GetTurn: %v", err)
	}
	turn.Metadata = map[string]interface{}{"model": "opus"}
	if err := svc.UpdateTurn(ctx, turn); err != nil {
		t.Fatalf("UpdateTurn: %v", err)
	}
	stored, err := repo.GetTurn(ctx, "turn-msg")
	if err != nil {
		t.Fatalf("GetTurn: %v", err)
	}
	if stored.Metadata["model"] != "opus" {
		t.Fatalf("metadata = %v, want the persisted model", stored.Metadata)
	}
}

func TestCompleteTurnPublishesHadOutput(t *testing.T) {
	svc, bus, repo := newMessageTestService(t)
	ctx := context.Background()

	// An empty turn: no agent output.
	if err := svc.CompleteTurn(ctx, "turn-msg"); err != nil {
		t.Fatalf("CompleteTurn: %v", err)
	}
	completed := singleTurnCompletedEvent(t, bus)
	if completed["had_output"] != false {
		t.Fatalf("had_output = %v, want false for a turn with no agent output", completed["had_output"])
	}
	if completed["id"] != "turn-msg" || completed["session_id"] != "sess-msg" {
		t.Fatalf("turn event identity = %v/%v, want turn-msg/sess-msg", completed["id"], completed["session_id"])
	}
	if completed["completed_at"] == nil {
		t.Fatal("turn.completed payload must carry completed_at")
	}
	stored, err := repo.GetTurn(ctx, "turn-msg")
	if err != nil {
		t.Fatalf("GetTurn: %v", err)
	}
	if stored.CompletedAt == nil {
		t.Fatal("completed_at must be set")
	}

	// A turn carrying agent text reports had_output.
	seedTurn(t, repo, "turn-output", "sess-msg", "task-msg")
	seedMessage(t, repo, &models.Message{
		ID: "msg-output", TurnID: "turn-output", AuthorType: models.MessageAuthorAgent, Content: "done",
	})
	bus.ClearEvents()
	if err := svc.CompleteTurn(ctx, "turn-output"); err != nil {
		t.Fatalf("CompleteTurn: %v", err)
	}
	if got := singleTurnCompletedEvent(t, bus)["had_output"]; got != true {
		t.Fatalf("had_output = %v, want true", got)
	}
}

func TestCompleteTurnIgnoresEmptyIDAndMissingTurn(t *testing.T) {
	svc, bus, _ := newMessageTestService(t)
	ctx := context.Background()

	if err := svc.CompleteTurn(ctx, ""); err != nil {
		t.Fatalf("CompleteTurn(\"\") must be a no-op, got %v", err)
	}
	if published := bus.GetPublishedEvents(); len(published) != 0 {
		t.Fatalf("empty turn id published %v, want nothing", eventTypes(published))
	}
}

func TestCompleteTurnClosesPendingToolCalls(t *testing.T) {
	svc, _, repo := newMessageTestService(t)
	ctx := context.Background()
	seedMessage(t, repo, &models.Message{
		ID: "msg-pending-tool", TurnID: "turn-msg", Type: models.MessageTypeToolExecute,
		AuthorType: models.MessageAuthorAgent, Content: "run",
		Metadata: map[string]interface{}{"tool_call_id": "tc-pending", "status": "running"},
	})

	if err := svc.CompleteTurn(ctx, "turn-msg"); err != nil {
		t.Fatalf("CompleteTurn: %v", err)
	}
	stored, err := repo.GetMessage(ctx, "msg-pending-tool")
	if err != nil {
		t.Fatalf("GetMessage: %v", err)
	}
	if stored.Metadata["status"] != "complete" {
		t.Fatalf("tool status = %v, want the safety net to complete it", stored.Metadata["status"])
	}
}

// singleTurnCompletedEvent returns the one turn.completed payload.
func singleTurnCompletedEvent(t *testing.T, bus *MockEventBus) map[string]interface{} {
	t.Helper()
	published := bus.GetPublishedEvents()
	var matched []map[string]interface{}
	for _, event := range published {
		if event.Type != events.TurnCompleted {
			continue
		}
		data, ok := event.Data.(map[string]interface{})
		if !ok {
			t.Fatalf("event data = %T, want a map payload", event.Data)
		}
		matched = append(matched, data)
	}
	if len(matched) != 1 {
		t.Fatalf("published %v, want exactly one %s", eventTypes(published), events.TurnCompleted)
	}
	return matched[0]
}

func TestTurnHadAgentOutputAllowlist(t *testing.T) {
	cases := []struct {
		name string
		msg  *models.Message
		want bool
	}{
		{"nil message", nil, false},
		{"other turn", &models.Message{
			TurnID: "other", AuthorType: models.MessageAuthorAgent, Type: models.MessageTypeToolCall,
		}, false},
		{"user message", &models.Message{
			TurnID: "turn-1", AuthorType: models.MessageAuthorUser, Content: "hi",
		}, false},
		{"tool call", &models.Message{
			TurnID: "turn-1", AuthorType: models.MessageAuthorAgent, Type: models.MessageTypeToolCall,
		}, true},
		{"permission request", &models.Message{
			TurnID: "turn-1", AuthorType: models.MessageAuthorAgent, Type: models.MessageTypePermissionRequest,
		}, true},
		{"agent plan", &models.Message{
			TurnID: "turn-1", AuthorType: models.MessageAuthorAgent, Type: models.MessageTypeAgentPlan,
		}, true},
		{"blank text", &models.Message{
			TurnID: "turn-1", AuthorType: models.MessageAuthorAgent, Type: models.MessageTypeMessage, Content: "  ",
		}, false},
		{"non-empty text", &models.Message{
			TurnID: "turn-1", AuthorType: models.MessageAuthorAgent, Type: models.MessageTypeMessage, Content: "hi",
		}, true},
		{"untyped text counts", &models.Message{
			TurnID: "turn-1", AuthorType: models.MessageAuthorAgent, Content: "hi",
		}, true},
		{"status notice does not count", &models.Message{
			TurnID: "turn-1", AuthorType: models.MessageAuthorAgent,
			Type: models.MessageType("status"), Content: "starting",
		}, false},
	}
	for _, tc := range cases {
		if got := turnHadAgentOutput([]*models.Message{tc.msg}, "turn-1"); got != tc.want {
			t.Fatalf("%s: turnHadAgentOutput = %v, want %v", tc.name, got, tc.want)
		}
	}
	if turnHadAgentOutput(nil, "turn-1") {
		t.Fatal("an empty message list must report no output")
	}
}

func TestStringConfigOptionsNormalizesShapes(t *testing.T) {
	typed := stringConfigOptions(map[string]string{"a": "1"})
	if len(typed) != 1 || typed["a"] != "1" {
		t.Fatalf("typed map = %v", typed)
	}

	loose := stringConfigOptions(map[string]interface{}{"a": "1", "b": 2, "empty": ""})
	if loose["a"] != "1" {
		t.Fatalf("loose map = %v, want string values carried over", loose)
	}
	if _, ok := loose["empty"]; ok {
		t.Fatalf("loose map = %v, want empty values dropped", loose)
	}

	if got := stringConfigOptions("not a map"); got != nil {
		t.Fatalf("unsupported shape = %v, want nil", got)
	}
	if got := stringConfigOptions(nil); got != nil {
		t.Fatalf("nil = %v, want nil", got)
	}
}

func TestGitSnapshotAndCommitReads(t *testing.T) {
	svc, _, repo := newMessageTestService(t)
	ctx := context.Background()

	// No snapshots yet: the cumulative diff is a valid empty state.
	diff, err := svc.GetCumulativeDiff(ctx, "sess-msg")
	if err != nil {
		t.Fatalf("GetCumulativeDiff with no snapshots: %v", err)
	}
	if diff != nil {
		t.Fatalf("cumulative diff = %+v, want nil for a fresh session", diff)
	}

	// Explicit created_at values pin first-vs-latest ordering without depending
	// on wall-clock spacing between two inserts.
	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	first := seedGitSnapshot(t, repo, "snap-1", "sess-msg", "base-sha", "head-1", base)
	latest := seedGitSnapshot(t, repo, "snap-2", "sess-msg", "base-sha-2", "head-2", base.Add(time.Minute))

	snapshots, err := svc.GetGitSnapshots(ctx, "sess-msg", 10)
	if err != nil {
		t.Fatalf("GetGitSnapshots: %v", err)
	}
	if len(snapshots) != 2 {
		t.Fatalf("snapshots = %d, want 2", len(snapshots))
	}

	gotFirst, err := svc.GetFirstGitSnapshot(ctx, "sess-msg")
	if err != nil {
		t.Fatalf("GetFirstGitSnapshot: %v", err)
	}
	if gotFirst.ID != first.ID {
		t.Fatalf("first snapshot = %q, want %q", gotFirst.ID, first.ID)
	}
	gotLatest, err := svc.GetLatestGitSnapshot(ctx, "sess-msg")
	if err != nil {
		t.Fatalf("GetLatestGitSnapshot: %v", err)
	}
	if gotLatest.ID != latest.ID {
		t.Fatalf("latest snapshot = %q, want %q", gotLatest.ID, latest.ID)
	}

	if err := repo.CreateSessionCommit(ctx, &models.SessionCommit{
		ID: "commit-1", SessionID: "sess-msg", CommitSHA: "sha-1", CommitMessage: "first",
		CommittedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("CreateSessionCommit: %v", err)
	}
	commits, err := svc.GetSessionCommits(ctx, "sess-msg")
	if err != nil {
		t.Fatalf("GetSessionCommits: %v", err)
	}
	if len(commits) != 1 || commits[0].CommitSHA != "sha-1" {
		t.Fatalf("commits = %+v", commits)
	}
	latestCommit, err := svc.GetLatestSessionCommit(ctx, "sess-msg")
	if err != nil {
		t.Fatalf("GetLatestSessionCommit: %v", err)
	}
	if latestCommit.ID != "commit-1" {
		t.Fatalf("latest commit = %+v", latestCommit)
	}

	diff, err = svc.GetCumulativeDiff(ctx, "sess-msg")
	if err != nil {
		t.Fatalf("GetCumulativeDiff: %v", err)
	}
	if diff == nil {
		t.Fatal("cumulative diff must be present once snapshots exist")
	}
	if diff.BaseCommit != "base-sha" {
		t.Fatalf("base commit = %q, want the first snapshot's base", diff.BaseCommit)
	}
	if diff.HeadCommit != "head-2" {
		t.Fatalf("head commit = %q, want the latest snapshot's head", diff.HeadCommit)
	}
	if diff.TotalCommits != 1 {
		t.Fatalf("total commits = %d, want 1", diff.TotalCommits)
	}
	if diff.SessionID != "sess-msg" {
		t.Fatalf("session id = %q", diff.SessionID)
	}
	if _, ok := diff.Files["main.go"]; !ok {
		t.Fatalf("files = %v, want the latest snapshot's files", diff.Files)
	}
}

func seedGitSnapshot(t *testing.T, repo *sqliterepo.Repository, id, sessionID, baseCommit, headCommit string, createdAt time.Time) *models.GitSnapshot {
	t.Helper()
	snapshot := &models.GitSnapshot{
		ID: id, SessionID: sessionID, SnapshotType: models.SnapshotTypeStatusUpdate,
		Branch: "main", HeadCommit: headCommit, BaseCommit: baseCommit,
		Files:     map[string]interface{}{"main.go": map[string]interface{}{"status": "modified"}},
		CreatedAt: createdAt,
	}
	if err := repo.CreateGitSnapshot(context.Background(), snapshot); err != nil {
		t.Fatalf("CreateGitSnapshot %s: %v", id, err)
	}
	return snapshot
}

func TestPersistSessionRuntimeModeStoresOverrideAndMetadata(t *testing.T) {
	svc, _, repo := newMessageTestService(t)
	ctx := context.Background()

	if err := svc.PersistSessionRuntimeMode(ctx, "sess-msg", ""); err != nil {
		t.Fatalf("empty mode must be a no-op, got %v", err)
	}
	session, err := repo.GetTaskSession(ctx, "sess-msg")
	if err != nil {
		t.Fatalf("GetTaskSession: %v", err)
	}
	if _, ok := session.Metadata[models.SessionMetaKeySessionMode]; ok {
		t.Fatalf("metadata = %v, want no mode written for an empty mode", session.Metadata)
	}

	if err := svc.PersistSessionRuntimeMode(ctx, "sess-msg", "plan"); err != nil {
		t.Fatalf("PersistSessionRuntimeMode: %v", err)
	}
	session, err = repo.GetTaskSession(ctx, "sess-msg")
	if err != nil {
		t.Fatalf("GetTaskSession: %v", err)
	}
	if session.Metadata[models.SessionMetaKeySessionMode] != "plan" {
		t.Fatalf("session mode = %v, want plan", session.Metadata[models.SessionMetaKeySessionMode])
	}
	overrides, _ := models.LoadSessionRuntimeConfigOverrides(session.Metadata)
	if overrides.Mode != "plan" {
		t.Fatalf("runtime overrides = %+v, want mode plan", overrides)
	}

	if err := svc.PersistSessionRuntimeMode(ctx, "sess-unknown", "plan"); err == nil {
		t.Fatal("persisting a mode on an unknown session must fail")
	}
}
