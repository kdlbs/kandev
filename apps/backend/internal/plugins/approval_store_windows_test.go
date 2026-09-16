//go:build windows

package plugins

import (
	"testing"
	"time"
)

func TestApprovalLedgerGrantWorksOnWindows(t *testing.T) {
	ledger := newApprovalLedger(t.TempDir())
	if _, err := ledger.grant("inst-1", "ws-1", 1, "digest-a", []string{"host.v2.read:tasks"}, "human", "grant", "audit-1", time.Now().UTC()); err != nil {
		t.Fatalf("grant on Windows: %v", err)
	}
}
