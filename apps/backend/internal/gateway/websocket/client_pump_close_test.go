package websocket

import (
	"context"
	"testing"
	"time"

	gorillaws "github.com/gorilla/websocket"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/common/logger"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestReadPumpCloseLogging(t *testing.T) {
	tests := []struct {
		name      string
		code      int
		wantError bool
	}{
		{name: "normal", code: gorillaws.CloseNormalClosure},
		{name: "unexpected", code: gorillaws.CloseInternalServerErr, wantError: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			core, recorded := observer.New(zap.DebugLevel)
			log, err := logger.NewFromZap(zap.New(core, zap.AddStacktrace(zap.ErrorLevel)))
			if err != nil {
				t.Fatalf("create logger: %v", err)
			}
			hub := NewHub(nil, log)
			hubCtx, cancelHub := context.WithCancel(context.Background())
			hubDone := make(chan struct{})
			go func() {
				defer close(hubDone)
				hub.Run(hubCtx)
			}()
			t.Cleanup(func() {
				cancelHub()
				joinWithin(t, hubDone, "websocket hub")
			})

			browser, server := newTerminalWSPair(t)
			client := NewClient("close-test", authn.Identity{}, server, hub, log)
			hub.Register(client)
			waitForClientCount(t, hub, client.ID)

			pumpDone := make(chan struct{})
			go func() {
				defer close(pumpDone)
				client.ReadPump(context.Background())
			}()
			t.Cleanup(func() {
				_ = browser.Close()
				_ = server.Close()
				joinWithin(t, pumpDone, "read pump")
			})

			if err := browser.WriteControl(
				gorillaws.CloseMessage,
				gorillaws.FormatCloseMessage(test.code, "test close"),
				time.Now().Add(wsTestTimeout),
			); err != nil {
				t.Fatalf("write close frame: %v", err)
			}
			joinWithin(t, pumpDone, "read pump")

			records := recorded.FilterMessage("WebSocket read error").All()
			wantRecords := 0
			if test.wantError {
				wantRecords = 1
			}
			if len(records) != wantRecords {
				t.Fatalf("read error records = %d, want %d; records = %+v", len(records), wantRecords, records)
			}
			if !test.wantError {
				return
			}
			if records[0].Stack == "" {
				t.Fatal("unexpected close error did not include a stack trace")
			}
		})
	}
}
