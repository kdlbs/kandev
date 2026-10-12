package websocket

import (
	"encoding/json"
	"testing"
	"time"

	gorillaws "github.com/gorilla/websocket"
)

func TestLSPLeaseManagerEvictsOnlyDetachedLeaseAtCapacity(t *testing.T) {
	t.Run("evicts detached", func(t *testing.T) {
		manager := newLSPLeaseManager(1, testLogger())
		lease := newTestLSPLease(manager)
		lease.detachedAt = time.Now().Add(-time.Minute)
		close(lease.readDone)
		if err := manager.add(lease); err != nil {
			t.Fatal(err)
		}
		if err := manager.ensureCapacity(); err != nil {
			t.Fatalf("ensureCapacity() error = %v", err)
		}
		if got := len(manager.leases); got != 0 {
			t.Fatalf("lease count after eviction = %d, want 0", got)
		}
	})

	t.Run("rejects when every lease is attached", func(t *testing.T) {
		manager := newLSPLeaseManager(1, testLogger())
		lease := newTestLSPLease(manager)
		lease.browser = &gorillaws.Conn{}
		if err := manager.add(lease); err != nil {
			t.Fatal(err)
		}
		if err := manager.ensureCapacity(); err != errLSPCapacityExceeded {
			t.Fatalf("ensureCapacity() error = %v, want %v", err, errLSPCapacityExceeded)
		}
		if lease.isClosed() {
			t.Fatal("attached lease was closed to make room")
		}
	})
}

func TestLSPLeaseProgressSnapshotRetainsBeginAndLatestReport(t *testing.T) {
	lease := newTestLSPLease(newLSPLeaseManager(2, testLogger()))
	lease.progressTokens[jsonRPCIDKey([]byte(`"scan"`))] = []byte(`"scan"`)
	if err := lease.updateProgress(json.RawMessage(`{"token":"scan","value":{"kind":"begin","title":"Indexing","percentage":0}}`)); err != nil {
		t.Fatal(err)
	}
	if err := lease.updateProgress(json.RawMessage(`{"token":"scan","value":{"kind":"report","message":"Loading packages","percentage":42}}`)); err != nil {
		t.Fatal(err)
	}

	var snapshot struct {
		Token string `json:"token"`
		Value struct {
			Kind       string `json:"kind"`
			Title      string `json:"title"`
			Message    string `json:"message"`
			Percentage int    `json:"percentage"`
		} `json:"value"`
	}
	if err := json.Unmarshal(lease.progress[jsonRPCIDKey([]byte(`"scan"`))], &snapshot); err != nil {
		t.Fatal(err)
	}
	if snapshot.Value.Kind != "begin" || snapshot.Value.Title != "Indexing" || snapshot.Value.Message != "Loading packages" || snapshot.Value.Percentage != 42 {
		t.Fatalf("progress snapshot = %+v, want the begin title with latest report state", snapshot)
	}
}

func TestLSPLeaseSnapshotSizeCountersTrackDynamicState(t *testing.T) {
	lease := newTestLSPLease(newLSPLeaseManager(2, testLogger()))
	register := json.RawMessage(`{"registrations":[{"id":"completion","method":"textDocument/completion"}]}`)
	if err := lease.updateRegistrations("client/registerCapability", register); err != nil {
		t.Fatal(err)
	}
	if lease.registrationSnapshotBytes == 0 {
		t.Fatal("registration size was not added to the snapshot counter")
	}
	if err := lease.updateRegistrations("client/unregisterCapability", json.RawMessage(`{"unregisterations":[{"id":"completion"}]}`)); err != nil {
		t.Fatal(err)
	}
	if lease.registrationSnapshotBytes != 0 {
		t.Fatalf("registration bytes after unregister = %d, want zero", lease.registrationSnapshotBytes)
	}

	if err := lease.addProgressToken(json.RawMessage(`{"token":"index"}`)); err != nil {
		t.Fatal(err)
	}
	if lease.progressTokenSnapshotBytes == 0 {
		t.Fatal("progress token size was not added to the snapshot counter")
	}
	if err := lease.updateProgress(json.RawMessage(`{"token":"index","value":{"kind":"begin","title":"Index"}}`)); err != nil {
		t.Fatal(err)
	}
	if lease.progressSnapshotBytes == 0 {
		t.Fatal("progress size was not added to the snapshot counter")
	}
	if err := lease.updateProgress(json.RawMessage(`{"token":"index","value":{"kind":"end"}}`)); err != nil {
		t.Fatal(err)
	}
	if lease.progressSnapshotBytes != 0 {
		t.Fatalf("progress bytes after end = %d, want zero", lease.progressSnapshotBytes)
	}
}
