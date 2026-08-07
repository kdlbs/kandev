package acp

import (
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/server/adapter/transport/shared"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/common/logger"
)

func TestAdapterCloseUnblocksDirectUpdateSender(t *testing.T) {
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	if err != nil {
		t.Fatalf("logger: %v", err)
	}
	a := NewAdapter(&shared.Config{AgentID: "close-test", WorkDir: t.TempDir()}, log)
	for index := 0; index < cap(a.updatesCh); index++ {
		a.updatesCh <- AgentEvent{Type: streams.EventTypeReasoning}
	}

	senderDone := make(chan struct{})
	go func() {
		a.sendUpdate(AgentEvent{Type: streams.EventTypeReasoning})
		close(senderDone)
	}()

	if err := a.Close(); err != nil {
		t.Fatalf("close adapter: %v", err)
	}
	select {
	case <-senderDone:
	case <-time.After(2 * time.Second):
		t.Fatal("direct sendUpdate caller remained blocked during close")
	}
}
