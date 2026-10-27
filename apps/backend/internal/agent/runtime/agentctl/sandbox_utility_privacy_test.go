package client

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/server/utility"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/common/sandbox"
	"github.com/stretchr/testify/require"
)

func TestSandboxUtilityErrorsDoNotExposePrivateReceipts(t *testing.T) {
	content := []byte("fixture")
	digest := sha256.Sum256(content)
	files, err := sandbox.MaterializeRuntime(t.Context(), t.TempDir(), content, hex.EncodeToString(digest[:]), nil, nil)
	require.NoError(t, err)
	defer func() { require.NoError(t, files.Close()) }()
	launch := &sandbox.Launch{Version: 1, AttemptID: "attempt", Generation: 1, PolicyDigest: strings.Repeat("a", 64), PackageDigest: strings.Repeat("b", 64), Root: files.Root, Prefix: []string{files.Executable}, Digests: files.Digests}
	for _, status := range []int{http.StatusInternalServerError, http.StatusOK} {
		t.Run(strconv.Itoa(status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/identity" {
					require.NoError(t, json.NewEncoder(w).Encode(map[string]any{"capabilities": []string{sandbox.LaunchCapability}}))
					return
				}
				w.WriteHeader(status)
				_, _ = w.Write([]byte(`{"sandbox_receipt":{"owner":{"birth_token":"private-birth-identity"}}`))
			}))
			defer server.Close()
			host, portText, err := net.SplitHostPort(server.Listener.Addr().String())
			require.NoError(t, err)
			port, err := strconv.Atoi(portText)
			require.NoError(t, err)
			log, err := logger.NewLogger(logger.LoggingConfig{Level: "error"})
			require.NoError(t, err)
			transport := NewClient(host, port, log)
			cfg := &utility.InferenceConfigDTO{Sandbox: launch}
			_, err = transport.Probe(t.Context(), &utility.ProbeRequest{InferenceConfig: cfg})
			require.Error(t, err)
			require.NotContains(t, err.Error(), "private-birth-identity")
			_, err = transport.InferencePrompt(t.Context(), &utility.PromptRequest{InferenceConfig: cfg})
			require.Error(t, err)
			require.NotContains(t, err.Error(), "private-birth-identity")
		})
	}
}
