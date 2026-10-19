package lifecycle

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSSHResumeDetectsRemotePassthroughWithoutACP(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/passthrough" {
			_, _ = fmt.Fprint(w, `{"version":1,"session_id":"session-1","instance_id":"instance-1","generation":1,"process_id":"remote-pty","status":"running"}`)
			return
		}
		_, _ = fmt.Fprint(w, `{"agent_status":"stopped"}`)
	}))
	t.Cleanup(server.Close)
	parsed, err := url.Parse(server.URL)
	require.NoError(t, err)
	port, err := strconv.Atoi(parsed.Port())
	require.NoError(t, err)
	executor := &SSHExecutor{logger: newTestRegistryLogger()}
	client, reusing, probeErr := executor.newResumedAgentctlClient(context.Background(), port, &ExecutorCreateRequest{InstanceID: "instance-1", SessionID: "session-1", AuthToken: "token", IsPassthrough: true})
	if probeErr != nil {
		t.Fatalf("resume inspection: %v", probeErr)
	}
	t.Cleanup(client.Close)
	require.True(t, reusing, "a live remote PTY must not be replaced because ACP is stopped")
}
