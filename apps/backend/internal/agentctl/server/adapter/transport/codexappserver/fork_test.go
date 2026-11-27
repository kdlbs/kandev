package codexappserver

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/server/adapter/transport/shared"
	"github.com/kandev/kandev/internal/common/logger"
	protocol "github.com/kandev/kandev/pkg/codexappserver"
)

type nativeForkCapability interface {
	ForkSession(context.Context, string, string) (string, error)
}

func TestForkSessionPreservesSourceAndRejectsActiveWork(t *testing.T) {
	forkCalls := 0
	server := newProtocolServer(t, func(req map[string]json.RawMessage, write func(any) error) error {
		if method := readString(req, "method"); method != "thread/fork" {
			return write(errorFrame(req["id"], -32601, "unexpected method"))
		}
		forkCalls++
		var params struct {
			ThreadID   string `json:"threadId"`
			LastTurnID string `json:"lastTurnId"`
		}
		if err := json.Unmarshal(req["params"], &params); err != nil {
			return err
		}
		if params.ThreadID != "source-thread" || params.LastTurnID != "completed-turn" {
			t.Errorf("thread/fork params = %#v", params)
		}
		return write(resultFrame(req["id"], map[string]any{"thread": map[string]any{"id": "forked-thread"}}))
	})
	defer server.close()

	adapter := NewAdapter(&shared.Config{}, logger.Default())
	defer func() { _ = adapter.Close() }()
	if err := adapter.Connect(server.clientWriter, server.clientReader); err != nil {
		t.Fatal(err)
	}
	adapter.threadID = "source-thread"
	capability, ok := any(adapter).(nativeForkCapability)
	if !ok {
		t.Fatal("native Codex adapter does not expose conversation forking")
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	forkedID, err := capability.ForkSession(ctx, "source-thread", "completed-turn")
	if err != nil {
		t.Fatalf("ForkSession: %v", err)
	}
	if forkedID != "forked-thread" || adapter.GetSessionID() != "source-thread" {
		t.Fatalf("forked ID = %q, active source ID = %q", forkedID, adapter.GetSessionID())
	}

	adapter.turnID = "active-turn"
	if _, err := capability.ForkSession(ctx, "source-thread", "completed-turn"); !errors.Is(err, protocol.ErrForkPrecondition) {
		t.Fatalf("ForkSession with active turn error = %v, want typed pre-provider refusal", err)
	}
	if forkCalls != 1 {
		t.Fatalf("provider fork calls = %d, want no RPC for the refused request", forkCalls)
	}
}

func TestUnboundChildTurnDoesNotSetRootTurnID(t *testing.T) {
	adapter := NewAdapter(&shared.Config{}, logger.Default())
	defer func() { _ = adapter.Close() }()
	adapter.threadID = "root-thread"

	adapter.handleNotification(context.Background(), "turn/started", json.RawMessage(`{"threadId":"child-thread","turn":{"id":"child-turn","status":"inProgress"}}`))
	if got := adapter.GetOperationID(); got != "" {
		t.Fatalf("root turn ID after unbound child turn started = %q, want empty", got)
	}

	adapter.handleNotification(context.Background(), "turn/completed", json.RawMessage(`{"threadId":"child-thread","turn":{"id":"child-turn","status":"completed"}}`))
	adapter.mu.RLock()
	activeChild := hasActiveChild(adapter.childStatuses, adapter.earlyChildActivities)
	adapter.mu.RUnlock()
	if activeChild {
		t.Fatal("completed unbound child remained active")
	}
}
