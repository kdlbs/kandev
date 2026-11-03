package plugins

import (
	"context"
	"errors"
	goruntime "runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/plugins/manifest"
	"github.com/stretchr/testify/require"
)

func TestInvokeAgentToolWaitsForNewlyAdvertisedConfigRestart(t *testing.T) {
	svc, _, rt := newTestService(t)
	svc.SetSecrets(newFakeSecretRevealer())
	const id = "kandev-plugin-github"
	installRequiredConfigToolPlugin(t, svc, id)
	require.Empty(t, catalogPluginIDsFor(t, svc))
	rt.setStartErr(id, errors.New("controlled restart failure"))
	started, release := rt.blockNextStart()
	var releaseOnce sync.Once
	releaseRestart := func() { releaseOnce.Do(release) }
	var workers sync.WaitGroup
	t.Cleanup(func() {
		releaseRestart()
		workers.Wait()
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	updated := make(chan error, 1)
	workers.Go(func() {
		updated <- svc.UpdateConfig(ctx, id, map[string]any{"github_token": "placeholder-token"})
	})
	select {
	case <-started:
	case <-ctx.Done():
		t.Fatal("config save did not reach the blocked replacement handshake")
	}
	require.Contains(t, catalogPluginIDsFor(t, svc), id, "the committed config advertises the tool before restart finishes")
	invoked := make(chan error, 1)
	workers.Go(func() {
		_, err := svc.InvokeAgentTool(ctx, id, "pr", nil, AgentToolInvocationContext{
			InvocationID: "after-config-advertisement", TaskID: "task-1", SessionID: "session-1",
			Surface: manifest.AgentToolSurfaceKanban,
		})
		invoked <- err
	})
	awaitAgentToolConfigDispatch(t, ctx, invoked)
	releaseRestart()
	require.ErrorContains(t, <-updated, "controlled restart failure")
	require.ErrorContains(t, <-invoked, "is not active")
	workers.Wait()
}

// The stack is a scheduling barrier for a mutex wait, which synctest does not
// treat as durable blocking. The invocation result remains the behavior oracle.
func awaitAgentToolConfigDispatch(t *testing.T, ctx context.Context, invoked <-chan error) {
	t.Helper()
	buffer := make([]byte, 1<<20)
	for {
		select {
		case err := <-invoked:
			t.Fatalf("newly advertised invocation reached the absent runtime before restart finished: %v", err)
		case <-ctx.Done():
			t.Fatal("invocation never joined the config restart dispatch lease")
		default:
		}
		stacks := string(buffer[:goruntime.Stack(buffer, true)])
		for _, stack := range strings.Split(stacks, "\n\n") {
			if strings.Contains(stack, ".(*Service).InvokeAgentTool(") && strings.Contains(stack, ".(*contextRWMutex).RLock(") {
				return
			}
		}
		goruntime.Gosched()
	}
}
