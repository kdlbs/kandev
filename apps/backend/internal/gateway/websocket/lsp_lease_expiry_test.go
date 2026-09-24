package websocket

import (
	"testing"
	"time"
)

// @covers AC-PLATFORM-LSP-FILE-INTELLIGENCE-002.10
func TestLSPLeaseAdmissionReleasesExpiredDetachedLeaseBelowCapacity(t *testing.T) {
	manager := newLSPLeaseManager(2, testLogger())
	lease := newTestLSPLease(manager)
	lease.detachedAt = time.Now().Add(-time.Hour - time.Second)
	if err := manager.add(lease); err != nil {
		t.Fatal(err)
	}
	if err := manager.ensureCapacity(); err != nil {
		t.Fatal(err)
	}
	if manager.hasActiveSession(lease.sessionID) {
		t.Fatal("expired detached lease still pins its task host")
	}
}

func TestLSPLeaseDetachedDeadlineReleasesLease(t *testing.T) {
	manager := newLSPLeaseManager(2, testLogger())
	manager.detachedTimeout = 20 * time.Millisecond
	lease := newTestLSPLease(manager)
	if err := manager.add(lease); err != nil {
		t.Fatal(err)
	}
	browser, _ := newLSPTestWebSocketPair(t)
	generation, _, err := lease.attach(browser)
	if err != nil {
		t.Fatal(err)
	}
	lease.detach(generation)

	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	deadline := time.After(time.Second)
	for {
		manager.mu.Lock()
		remaining := len(manager.leases)
		manager.mu.Unlock()
		if remaining == 0 {
			break
		}
		select {
		case <-ticker.C:
		case <-deadline:
			t.Fatal("detached lease did not expire")
		}
	}
	if manager.hasActiveSession(lease.sessionID) {
		t.Fatal("expired lease still pins its task host")
	}
}

func TestLSPLeaseOldDetachedDeadlineCannotCloseReattachedLease(t *testing.T) {
	manager := newLSPLeaseManager(2, testLogger())
	lease := newTestLSPLease(manager)
	if err := manager.add(lease); err != nil {
		t.Fatal(err)
	}
	first, _ := newLSPTestWebSocketPair(t)
	generation, _, err := lease.attach(first)
	if err != nil {
		t.Fatal(err)
	}
	lease.detach(generation)
	oldDetachment := lease.detachedTime()

	second, _ := newLSPTestWebSocketPair(t)
	secondGeneration, _, err := lease.attach(second)
	if err != nil {
		t.Fatal(err)
	}
	manager.detachedTimeout = 0 // Make the old deadline due without waiting an hour.
	lease.expireDetached(oldDetachment)
	if lease.isClosed() {
		t.Fatal("old deadline closed a reattached lease")
	}
	manager.detachedTimeout = time.Hour
	lease.detach(secondGeneration)
	manager.detachedTimeout = 0
	lease.expireDetached(oldDetachment)
	if lease.isClosed() {
		t.Fatal("old deadline closed a newly detached lease")
	}
	lease.terminate(lspCloseTransport, "test cleanup", lspLeaseReleaseStop)
}
