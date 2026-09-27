package api

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/server/process"
)

// TestStreamCloseReason pins the "Capability and close reason" bullet of
// system design part 2: a backend stream that agentctl closes with an
// explicit reason (agent_exited or agentctl_shutdown) reaches the WS peer as
// close code 1000 with that exact reason string, not a raw disconnect.
func TestStreamCloseReason(t *testing.T) {
	cases := []struct {
		name   string
		reason string
	}{
		{name: "agent_exited", reason: process.CloseReasonAgentExited},
		{name: "agentctl_shutdown", reason: process.CloseReasonAgentctlShutdown},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := newTestServer(t)
			server := httptest.NewServer(s.router)
			defer server.Close()

			conn := dialAgentStreamWithAttachID(t, server, "")
			defer func() { _ = conn.Close() }()

			if !waitForAttachmentCurrent(t, s, "") {
				t.Fatal("stream never became current")
			}

			closeCode := make(chan int, 1)
			closeReason := make(chan string, 1)
			conn.SetCloseHandler(func(code int, reason string) error {
				closeCode <- code
				closeReason <- reason
				return nil
			})
			go func() {
				for {
					if _, _, err := conn.ReadMessage(); err != nil {
						return
					}
				}
			}()

			s.procMgr.CloseAgentStream(process.CloseCodeNormal, tc.reason)

			select {
			case code := <-closeCode:
				if code != process.CloseCodeNormal {
					t.Fatalf("close code = %d, want %d", code, process.CloseCodeNormal)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("stream was never closed")
			}

			select {
			case reason := <-closeReason:
				if reason != tc.reason {
					t.Fatalf("close reason = %q, want %q", reason, tc.reason)
				}
			case <-time.After(time.Second):
				t.Fatal("close handler never received a reason")
			}
		})
	}
}
