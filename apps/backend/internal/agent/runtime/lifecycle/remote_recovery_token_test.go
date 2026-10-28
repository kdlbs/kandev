package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/common/sessionmcp"
	"github.com/kandev/kandev/internal/task/models"
)

func remoteTokenInstance(t *testing.T, runtime agentruntime.Runtime, observedFP string, control mcpTokenControl) *ExecutorInstance {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprintf(w, `{"capabilities":["session_mcp_token"],"mcp_token_fingerprint":%q}`, observedFP)
	}))
	t.Cleanup(srv.Close)
	host, portStr, _ := net.SplitHostPort(srv.Listener.Addr().String())
	var port int
	_, _ = fmt.Sscanf(portStr, "%d", &port)
	return &ExecutorInstance{
		InstanceID:   "exec-1",
		RuntimeName:  runtime,
		Client:       agentctl.NewClient(host, port, newTestLogger()),
		tokenControl: control,
	}
}

func TestVerifyRetainedSessionTokenByRuntime(t *testing.T) {
	good := sessionmcp.Fingerprint(sessionmcp.Token(launchKey(), "exec-1", "n1"))
	record := &models.ExecutorRunning{Metadata: map[string]interface{}{MetadataKeyMCPTokenNonce: "n1"}}
	m := newLaunchManager(&keyedStore{key: launchKey()})

	for _, rt := range []agentruntime.Runtime{agentruntime.RuntimeKubernetes, agentruntime.RuntimeSSH} {
		t.Run(string(rt)+" refuses retained reuse", func(t *testing.T) {
			ctl := &fakeControl{}
			ri := remoteTokenInstance(t, rt, good, ctl)
			err := m.verifyRetainedSessionToken(context.Background(), record, ri)
			var refusal *LaunchRefusal
			if !errors.As(err, &refusal) || refusal.Code != CodeMCPTokenUnsupported || refusal.Cause != CauseRetainedInstanceDeferred {
				t.Fatalf("err = %v, want unsupported/retained_instance_deferred", err)
			}
			if len(ctl.deleted) != 0 || ctl.revokes != 1 {
				t.Fatalf("deleted=%v revokes=%d, want revoke only", ctl.deleted, ctl.revokes)
			}
		})
	}

	t.Run("matching token adopts", func(t *testing.T) {
		ctl := &fakeControl{}
		if err := m.verifyRetainedSessionToken(context.Background(), record, remoteTokenInstance(t, agentruntime.RuntimeSprites, good, ctl)); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("mismatch deletes with the observed fingerprint and does not adopt", func(t *testing.T) {
		ctl := &fakeControl{}
		err := m.verifyRetainedSessionToken(context.Background(), record, remoteTokenInstance(t, agentruntime.RuntimePluginRemote, "stale", ctl))
		if !errors.Is(err, errRetainedTokenUnverified) || len(ctl.deleted) != 1 || ctl.deleted[0] != "stale" {
			t.Fatalf("err=%v deleted=%v", err, ctl.deleted)
		}
	})
	t.Run("missing client is not adopted", func(t *testing.T) {
		ri := &ExecutorInstance{InstanceID: "exec-1", RuntimeName: agentruntime.RuntimeSprites}
		if err := m.verifyRetainedSessionToken(context.Background(), record, ri); !errors.Is(err, errRetainedTokenUnverified) {
			t.Fatalf("err = %v", err)
		}
	})
}
