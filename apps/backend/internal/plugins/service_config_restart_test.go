package plugins

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRestartForConfigChangeRefreshesBothNotificationsOnFailure(t *testing.T) {
	svc, _, fsStore, rt := newTestServiceWithDir(t)
	installTestPlugin(t, svc, "kandev-plugin-slack")

	// Wire start failure through the service's existing fakeRuntime.
	rt.setStartErr("kandev-plugin-slack", errors.New("restart handshake failed"))

	deliverer := &fakeDeliverer{}
	svc.SetDeliverer(deliverer)
	catalogListener := &countingAgentToolCatalogListener{}
	svc.SetAgentToolCatalogListener(catalogListener)

	// Restart failure: UpdateConfig should fail, but both notifications must fire.
	err := svc.UpdateConfig(context.Background(), "kandev-plugin-slack", map[string]any{
		"new_key": "new_value",
	})
	if err == nil {
		t.Fatal("UpdateConfig expected error due to restart failure")
	}

	if deliverer.refreshCount != 1 {
		t.Errorf("deliverer.Refresh() = %d, want 1", deliverer.refreshCount)
	}
	if catalogListener.calls.Load() != 2 {
		// UpdateConfig calls notifyAgentToolCatalogChanged once before the
		// restart; restart failure calls it a second time.
		t.Errorf("catalog NotifyAgentToolCatalogChanged() = %d, want 2", catalogListener.calls.Load())
	}

	// Confirm status moved to StatusError (setStatusAndDiagnostic succeeds in this path).
	got, getErr := svc.Get("kandev-plugin-slack")
	if getErr != nil {
		t.Fatalf("Get() after restart failure: %v", getErr)
	}
	if got.Status != StatusError {
		t.Errorf("status = %q, want %q", got.Status, StatusError)
	}

	// Verify the diagnostic reflects the restart error.
	if got.LastError == "" || !strings.Contains(got.LastError, "restart handshake failed") {
		t.Errorf("LastError = %q, want restart error text", got.LastError)
	}

	// Confirm persistence: FSStore.Get returns (*store.Record, error).
	onDisk, fsErr := fsStore.Get("kandev-plugin-slack")
	if fsErr != nil {
		t.Fatalf("fsStore.Get() after restart failure: %v", fsErr)
	}
	if onDisk.Status != StatusError {
		t.Errorf("persisted status = %q, want %q", onDisk.Status, StatusError)
	}
}

func TestRestartForConfigChangeStatusWriteFailureStillInvalidates(t *testing.T) {
	svc, _, fsStore, rt := newTestServiceWithDir(t)
	const id = "kandev-plugin-slack"
	installTestPlugin(t, svc, id)
	rt.setStartErr(id, errors.New("restart handshake failed"))
	failing := &failingSaveStore{Store: fsStore}
	failing.armFailNextSave()
	svc.store = failing
	deliverer := &fakeDeliverer{}
	svc.SetDeliverer(deliverer)
	listener := &countingAgentToolCatalogListener{}
	svc.SetAgentToolCatalogListener(listener)

	err := svc.UpdateConfig(context.Background(), id, map[string]any{"new_key": "new_value"})
	require.ErrorContains(t, err, "restart handshake failed")
	require.False(t, rt.Running(id))
	config, err := fsStore.GetConfig(id)
	require.NoError(t, err)
	require.Equal(t, "new_value", config["new_key"])
	record, err := svc.Get(id)
	require.NoError(t, err)
	require.Equal(t, StatusActive, record.Status, "a failed record write preserves the persisted status")
	onDisk, err := fsStore.Get(id)
	require.NoError(t, err)
	require.Equal(t, StatusActive, onDisk.Status)
	require.EqualValues(t, 2, listener.calls.Load(), "invalidate both before restart and after its failure")
	require.Equal(t, 1, deliverer.refreshCount)
}
