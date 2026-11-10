package websocket

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	gorillaws "github.com/gorilla/websocket"
	agentctlclient "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	agentctltypes "github.com/kandev/kandev/internal/agentctl/types"
)

func TestHandleRemoteAgentTerminalSanitizesUpstreamFailure(t *testing.T) {
	for _, privateEndpoint := range []bool{true, false} {
		name := "plain handshake error"
		if privateEndpoint {
			name = "credential and private path"
		}
		t.Run(name, func(t *testing.T) {
			log, recorded := observedTerminalLogger(t)
			handler, manager := newBridgeTestHandler(t, log)
			requests := make(chan *http.Request, 2)
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests <- r.Clone(r.Context())
				if r.URL.Path == "/api/v1/passthrough" {
					_ = json.NewEncoder(w).Encode(agentctltypes.PassthroughSnapshot{
						PassthroughTarget: agentctltypes.PassthroughTarget{ProcessID: "remote-proc", Generation: 1},
						Version:           agentctltypes.PassthroughProtocolVersion, InstanceID: "exec-privacy-session",
						SessionID: "privacy-session", Status: agentctltypes.ProcessStatusRunning,
					})
					return
				}
				w.WriteHeader(http.StatusForbidden)
			}))
			t.Cleanup(upstream.Close)
			execution := remoteExecutionFor(t, manager, "privacy-session", upstream.URL, log)
			execution.ExecutorType = "ssh"
			execution.PassthroughProcessID = "remote-proc"
			if _, err := manager.EnsurePassthroughExecution(t.Context(), execution.SessionID); err != nil {
				t.Fatalf("prime actual remote passthrough: %v", err)
			}
			if request := <-requests; request.Header.Get("Authorization") != "Bearer shell-token" {
				t.Fatal("snapshot request lost upstream authorization")
			}
			if privateEndpoint {
				execution.SetAgentCtlClientForTesting(agentctlclient.NewClient(
					"bad-host/password=PRIVACY_DUMMY_CREDENTIAL/home/PRIVACY_ACCOUNT/private.sock%xx", 0, log,
					agentctlclient.WithAuthToken("shell-token"),
				))
			}
			server, served := serveOnce(t, func(c *gin.Context) { handler.handleRemoteAgentTerminal(c, execution) })
			status, body := getTerminal(t, server, "/terminal/session/privacy-session?mode=agent")
			joinWithin(t, served, "handleRemoteAgentTerminal")
			if status != http.StatusServiceUnavailable {
				t.Fatalf("status=%d, want 503", status)
			}
			var response map[string]string
			if err := json.Unmarshal([]byte(body), &response); err != nil || len(response) != 1 || response["error"] == "" {
				t.Fatalf("expected unchanged error:string JSON shape: %s (%v)", body, err)
			}
			if strings.Contains(body, "PRIVACY_DUMMY_CREDENTIAL") || strings.Contains(body, "PRIVACY_ACCOUNT") {
				t.Errorf("client-facing error exposed upstream credentials or private path: %s", body)
			}
			warnings := recorded.FilterMessage("failed to acquire remote passthrough stream").All()
			if len(warnings) != 1 {
				t.Fatalf("warning count=%d, want 1", len(warnings))
			}
			fields := warnings[0].ContextMap()
			if fields["stage"] != "remote_stream_acquisition" || fields["session_id"] != execution.SessionID || fields["error"] != response["error"] {
				t.Errorf("warning and response differ or lost stream/session context: %+v / %+v", fields, response)
			}
			if !privateEndpoint {
				if response["error"] != gorillaws.ErrBadHandshake.Error() {
					t.Errorf("plain error changed: %q", response["error"])
				}
				request := <-requests
				if request.URL.Path != "/api/v1/passthrough/stream" || request.Header.Get("Authorization") != "Bearer shell-token" || request.URL.Query().Get("process_id") != "remote-proc" || request.URL.Query().Get("generation") != "1" {
					t.Errorf("stream request lost target or authorization: %s %+v", request.URL, request.Header)
				}
			}
		})
	}
}
