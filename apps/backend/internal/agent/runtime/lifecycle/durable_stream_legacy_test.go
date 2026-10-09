package lifecycle

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/jmoiron/sqlx"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/db"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
)

func newDurableStreamSQLiteRepository(t *testing.T) *sqliterepo.Repository {
	t.Helper()
	dbConn, err := db.OpenSQLite(filepath.Join(t.TempDir(), "durable-stream-test.db"))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	dbConn.SetMaxOpenConns(1)
	dbConn.SetMaxIdleConns(1)
	sqlxDB := sqlx.NewDb(dbConn, "sqlite3")
	repository, err := sqliterepo.NewWithDB(sqlxDB, sqlxDB, nil)
	if err != nil {
		_ = sqlxDB.Close()
		t.Fatalf("create sqlite repository: %v", err)
	}
	t.Cleanup(func() { _ = sqlxDB.Close() })
	return repository
}

func TestLegacyStreamRealRepository(t *testing.T) {
	repository := newDurableStreamSQLiteRepository(t)
	var callbacks []string
	sm := NewStreamManager(newTestLogger(), StreamCallbacks{
		OnAgentEvent: func(_ *AgentExecution, event agentctl.AgentEvent) {
			callbacks = append(callbacks, event.Type)
		},
	}, nil, nil)
	execution := &AgentExecution{
		SessionID:        "legacy-session",
		DeliveryMode:     DurableDeliveryLegacy,
		DeliveryStreamID: "legacy-stream",
	}

	for _, event := range []agentctl.AgentEvent{
		{Type: streams.EventTypeMessageChunk, Text: "legacy text"},
		{Type: streams.EventTypeReasoning, ReasoningText: "legacy reasoning"},
		{Type: streams.EventTypeComplete},
	} {
		if err := sm.processAgentEvent(context.Background(), execution, nil, repository, event, 0); err != nil {
			t.Fatalf("process legacy %s event: %v", event.Type, err)
		}
	}

	if got, want := callbacks, []string{
		streams.EventTypeMessageChunk,
		streams.EventTypeReasoning,
		streams.EventTypeComplete,
	}; len(got) != len(want) || got[0] != want[0] || got[1] != want[1] || got[2] != want[2] {
		t.Fatalf("legacy callbacks = %v, want %v", got, want)
	}
}

func TestDurableStreamRejectsMissingIdentity(t *testing.T) {
	repository := newDurableStreamSQLiteRepository(t)
	callbackCalled := false
	sm := NewStreamManager(newTestLogger(), StreamCallbacks{
		OnAgentEvent: func(*AgentExecution, agentctl.AgentEvent) {
			callbackCalled = true
		},
	}, nil, nil)
	execution := &AgentExecution{
		SessionID:                 "durable-session",
		DeliveryMode:              DurableDeliveryV1,
		DeliveryStreamID:          "durable-stream",
		DeliveryIncarnationID:     "incarnation-1",
		DeliveryHarnessGeneration: 1,
	}

	err := sm.processAgentEvent(context.Background(), execution, nil, repository, agentctl.AgentEvent{
		Type: streams.EventTypeMessageChunk,
		Text: "missing durable identity",
	}, 0)
	if !errors.Is(err, ErrUncertainPromptDelivery) {
		t.Fatalf("missing identity error = %v, want ErrUncertainPromptDelivery", err)
	}
	if callbackCalled {
		t.Fatal("durable callback ran for an event without stream identity")
	}
}
