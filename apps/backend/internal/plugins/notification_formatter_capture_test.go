package plugins

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/plugins/manifest"
	"github.com/stretchr/testify/require"
)

func TestFormatterCaptureAllowsDispatchWhileConfigUpdateWaits(t *testing.T) {
	svc, _, _ := newTestService(t)
	record := installTestPlugin(t, svc, "capture-formatter")
	record.NotificationFormatters = []manifest.NotificationFormatter{{Key: "text", ContractVersion: 1, ProviderTypes: []string{"apprise"}}}
	svc.registry.Add(record)
	release, err := svc.CaptureFormatterLifecycle(t.Context())
	require.NoError(t, err)
	done := make(chan error, 1)
	go func() { done <- svc.UpdateConfig(t.Context(), record.ID, map[string]any{}) }()
	t.Cleanup(func() { release(); <-done })
	gate := svc.formatterLifecycleLock(record.ID)
	dispatch := svc.dispatchLocks.lockFor(record.ID)
	require.Eventually(t, func() bool {
		gate.mu.Lock()
		waiting := gate.waitingWriters
		gate.mu.Unlock()
		dispatch.mu.Lock()
		waiting += dispatch.waitingWriters
		dispatch.mu.Unlock()
		return waiting > 0
	}, time.Second, time.Millisecond)
	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()
	_, finish, err := svc.beginPluginDispatchContext(ctx, record.ID, dispatchGeneration(record))
	require.NoError(t, err, "capture and a queued configuration change must not stall normal RPC admission")
	finish()
}
