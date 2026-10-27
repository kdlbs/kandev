package client

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/common/sandbox"
	"github.com/stretchr/testify/require"
)

func TestSandboxUtilityReceiptUsesAuthenticatedBoundedTransport(t *testing.T) {
	testSandboxReceiptTransport(t, false)
}
func TestSandboxPersistentReceiptUsesAuthenticatedBoundedTransport(t *testing.T) {
	testSandboxReceiptTransport(t, true)
}
func testSandboxReceiptTransport(t *testing.T, persistent bool) {
	t.Helper()
	for _, mode := range []string{"valid", "pending", "stale", "oversized", "redirect", "unknown-field"} {
		t.Run(mode, func(t *testing.T) {
			launch := &sandbox.Launch{AttemptID: "attempt", Generation: 2, PolicyDigest: strings.Repeat("a", 64)}
			receipt := sandbox.StartReceipt{Version: 1, AttemptID: launch.AttemptID, Generation: launch.Generation, PolicyDigest: launch.PolicyDigest, Outcome: sandbox.StartNotStarted}
			var called atomic.Bool
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				called.Store(true)
				endpoint := "/api/v1/inference/sandbox/attempt/receipt"
				if persistent {
					endpoint = "/api/v1/agent/sandbox/attempt/receipt"
				}
				require.Equal(t, endpoint, r.URL.Path)
				require.Equal(t, "Bearer synthetic-test-token", r.Header.Get("Authorization"))
				switch mode {
				case "pending":
					w.WriteHeader(http.StatusConflict)
					return
				case "stale":
					receipt.Generation++
				case "oversized":
					_, _ = w.Write([]byte(strings.Repeat("x", 4097)))
					return
				case "redirect":
					w.Header().Set("Location", "http://127.0.0.1:1/private")
					w.WriteHeader(http.StatusTemporaryRedirect)
					return
				case "unknown-field":
					_, _ = w.Write([]byte(`{"version":1,"unknown":true}`))
					return
				}
				require.NoError(t, json.NewEncoder(w).Encode(receipt))
			}))
			defer server.Close()
			host, portText, err := net.SplitHostPort(server.Listener.Addr().String())
			require.NoError(t, err)
			port, err := strconv.Atoi(portText)
			require.NoError(t, err)
			log, err := logger.NewLogger(logger.LoggingConfig{Level: "error"})
			require.NoError(t, err)
			transport := NewClient(host, port, log, WithAuthToken("synthetic-test-token"))
			fetch := transport.UtilitySandboxReceipt
			if persistent {
				fetch = transport.SandboxStartReceipt
			}
			got, err := fetch(t.Context(), launch)
			require.True(t, called.Load())
			switch mode {
			case "valid":
				require.NoError(t, err)
				require.Equal(t, receipt, *got)
			case "pending":
				require.NoError(t, err)
				require.Nil(t, got)
			default:
				require.Error(t, err)
				require.Nil(t, got)
			}
		})
	}
}
