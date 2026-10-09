package hostutility

import (
	"fmt"
	"testing"
)

func TestProfileContextGenerationsAreBoundedAndEvictedRevisionsBecomeStale(t *testing.T) {
	manager := &Manager{profileGenerations: map[string]uint64{"codex-acp": 7}}
	profileRevision := "profile-a"
	profileGeneration, ok := manager.profileContextGeneration("codex-acp", profileRevision, 7, true)
	if !ok {
		t.Fatal("profile A context generation was rejected")
	}

	for i := 0; i < profileCapabilityCacheMaxEntries+1; i++ {
		if _, ok := manager.profileContextGeneration("codex-acp", fmt.Sprintf("draft-%d", i), 7, false); !ok {
			t.Fatalf("draft %d context generation was rejected", i)
		}
	}

	if got := len(manager.profileContextGenerations); got > profileCapabilityCacheMaxEntries {
		t.Fatalf("profile context generation entries = %d, want at most %d", got, profileCapabilityCacheMaxEntries)
	}
	if manager.profileContextIsCurrent("codex-acp", profileRevision, 7, profileGeneration) {
		t.Fatal("evicted profile context generation remained current")
	}
}
