package handlers

import (
	"fmt"
	"strconv"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
	ws "github.com/kandev/kandev/pkg/websocket"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestRegisterHandlers_LogsMeasuredRegistrationDelta(t *testing.T) {
	core, logs := observer.New(zap.InfoLevel)
	log, err := logger.NewFromZap(zap.New(core))
	if err != nil {
		t.Fatalf("create observer logger: %v", err)
	}

	dispatcher := ws.NewDispatcher()
	before := dispatcher.HandlerCount()
	(&Handlers{logger: log}).RegisterHandlers(dispatcher)
	delta := dispatcher.HandlerCount() - before

	entries := logs.FilterMessage("registered MCP handlers").All()
	if len(entries) != 1 {
		t.Fatalf("registration log entries = %d, want 1", len(entries))
	}
	count, err := strconv.Atoi(fmt.Sprint(entries[0].ContextMap()["count"]))
	if err != nil {
		t.Fatalf("registration log count field = %#v, want numeric value: %v", entries[0].ContextMap()["count"], err)
	}
	if count != delta {
		t.Fatalf("logged handler delta = %d, measured dispatcher delta = %d", count, delta)
	}
}
