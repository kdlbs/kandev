package mcp

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/mcp/plugintools"
	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
	"github.com/stretchr/testify/require"
)

// serializedPluginToolBackend delays the first catalog request so two list
// requests can overlap deterministically, and serves one snapshot per call.
type serializedPluginToolBackend struct {
	mu           sync.Mutex
	calls        int
	firstStarted chan struct{}
	releaseFirst chan struct{}
	snapshots    []plugintools.Snapshot
}

func newSerializedPluginToolBackend(snapshots ...plugintools.Snapshot) *serializedPluginToolBackend {
	return &serializedPluginToolBackend{
		firstStarted: make(chan struct{}),
		releaseFirst: make(chan struct{}),
		snapshots:    snapshots,
	}
}

func (b *serializedPluginToolBackend) RequestPayload(ctx context.Context, _ string, _ interface{}, result interface{}) error {
	b.mu.Lock()
	index := b.calls
	b.calls++
	b.mu.Unlock()
	if index == 0 {
		close(b.firstStarted)
		select {
		case <-b.releaseFirst:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	if index >= len(b.snapshots) {
		index = len(b.snapshots) - 1
	}
	encoded, err := json.Marshal(b.snapshots[index])
	if err != nil {
		return err
	}
	return json.Unmarshal(encoded, result)
}

func automationCatalogTool(name string) plugintools.Definition {
	return plugintools.Definition{
		PluginID: "kandev-security", LocalName: name,
		ExposedName: plugintools.ExposedName("kandev-security", name),
		Description: name, InputSchema: []byte(`{"type":"object"}`),
		Surfaces: []string{plugintools.SurfaceAutomation},
	}
}

func waitForSync(t *testing.T, done <-chan struct{}) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("sync did not finish")
	}
}

func TestAutomationPluginToolsDiscardDelayedOlderResponse(t *testing.T) {
	oldTool := automationCatalogTool("list_findings")
	newTool := automationCatalogTool("complete_report")
	backend := newSerializedPluginToolBackend(
		plugintools.Snapshot{Generation: "automation", Revision: 1, Tools: []plugintools.Definition{oldTool}},
		plugintools.Snapshot{Generation: "automation", Revision: 1, Tools: []plugintools.Definition{newTool}},
	)
	s := NewWithProfile(backend, "automation-session", "automation-task", 10005, newTestLogger(t), "", false, mcpprofile.NewAutomation())

	firstDone := make(chan struct{})
	go func() {
		defer close(firstDone)
		s.syncPluginTools(context.Background())
	}()
	<-backend.firstStarted

	secondDone := make(chan struct{})
	go func() {
		defer close(secondDone)
		s.syncPluginTools(context.Background())
	}()
	waitForSync(t, secondDone)
	close(backend.releaseFirst)
	waitForSync(t, firstDone)

	require.Contains(t, s.mcpServer.ListTools(), newTool.ExposedName)
	require.NotContains(t, s.mcpServer.ListTools(), oldTool.ExposedName, "a delayed older response must not overwrite the newer catalog")
}

func TestAutomationPluginToolsInvalidationDiscardsInFlightFetch(t *testing.T) {
	oldTool := automationCatalogTool("list_findings")
	backend := newSerializedPluginToolBackend(
		plugintools.Snapshot{Generation: "automation", Revision: 1, Tools: []plugintools.Definition{oldTool}},
	)
	s := NewWithProfile(backend, "automation-session", "automation-task", 10005, newTestLogger(t), "", false, mcpprofile.NewAutomation())

	done := make(chan struct{})
	go func() {
		defer close(done)
		s.syncPluginTools(context.Background())
	}()
	<-backend.firstStarted

	require.NoError(t, s.SetPluginTools(plugintools.Snapshot{Generation: "global", Revision: 2}))
	close(backend.releaseFirst)
	waitForSync(t, done)

	require.NotContains(t, s.mcpServer.ListTools(), oldTool.ExposedName, "an invalidation must win over an in-flight filtered fetch")
}
