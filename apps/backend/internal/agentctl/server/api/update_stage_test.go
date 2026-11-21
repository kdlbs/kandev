package api

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	ws "github.com/kandev/kandev/pkg/websocket"
)

func TestStreamStageWriterIsOptInAndCountsSuccessfulWrites(t *testing.T) {
	server := &Server{logger: newTestLogger()}
	var writes [][]byte
	write := func(data []byte) error {
		writes = append(writes, append([]byte(nil), data...))
		return nil
	}

	disabled := server.newStreamStageWriter(write, false)
	if disabled.ticks() != nil {
		t.Fatal("disabled stage writer has a heartbeat ticker")
	}
	enabled := server.newStreamStageWriter(write, true)
	if enabled.ticks() == nil {
		t.Fatal("enabled stage writer has no heartbeat ticker")
	}
	defer enabled.stop()

	if err := enabled.writeEvent([]byte(`{"type":"message_chunk"}`)); err != nil {
		t.Fatalf("write event: %v", err)
	}
	if !enabled.heartbeat() {
		t.Fatal("heartbeat write failed")
	}
	if len(writes) != 2 {
		t.Fatalf("write count = %d, want event and heartbeat", len(writes))
	}
	var message ws.Message
	if err := json.Unmarshal(writes[1], &message); err != nil {
		t.Fatalf("decode heartbeat: %v", err)
	}
	if message.Action != streams.UpdateStreamStageAction {
		t.Fatalf("heartbeat action = %q, want %q", message.Action, streams.UpdateStreamStageAction)
	}
	var heartbeat streams.UpdateStreamStageHeartbeat
	if err := message.ParsePayload(&heartbeat); err != nil {
		t.Fatalf("decode heartbeat payload: %v", err)
	}
	if heartbeat.WriterSent != 1 || heartbeat.WriterAgeMillis < 0 {
		t.Fatalf("heartbeat writer stage = %+v, want one successful event write", heartbeat)
	}

	failed := server.newStreamStageWriter(func([]byte) error { return errors.New("closed") }, false)
	if err := failed.writeEvent([]byte("event")); err == nil {
		t.Fatal("failed event write unexpectedly succeeded")
	}
	if failed.sent != 0 {
		t.Fatalf("failed event write count = %d, want 0", failed.sent)
	}
	disabled.stop()
}
