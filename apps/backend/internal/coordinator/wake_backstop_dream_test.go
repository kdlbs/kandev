package coordinator

import (
	"context"
	"testing"
)

func TestBackstop_DreamDutyRunsBeforeTurnDutiesAndAloneForDreamOnlyCoordinators(t *testing.T) {
	e := newWakeEnv(t)
	log := &callLog{}
	e.svc.SetBackstopHooks(Hooks{
		Dream:      log.duty("dream"),
		TurnDuties: log.duty("turns"),
		Lowering:   log.duty("lowering"),
		Deliver:    log.duty("deliver"),
	})
	// A Shadow-on coordinator with autonomy off is brought in by the dream
	// query alone, so it gets the dream tick and no other duty.
	shadowOnly := newTestCoordinator(t, e.store, "ws-dream")
	if err := e.store.SetShadowDreamEnabled(context.Background(), shadowOnly.WorkspaceID, shadowOnly.ID, true); err != nil {
		t.Fatal(err)
	}
	e.pass(t)

	calls := log.got()
	var forShadow []string
	for _, c := range calls {
		if len(c) > len(shadowOnly.ID) && c[len(c)-len(shadowOnly.ID):] == shadowOnly.ID {
			forShadow = append(forShadow, c)
		}
	}
	if !equalStrings(forShadow, []string{"dream:" + shadowOnly.ID}) {
		t.Fatalf("calls for the dream-only coordinator = %v, want the dream tick alone", forShadow)
	}
	// The visited coordinator runs dream first, then its other duties.
	first := ""
	for _, c := range calls {
		if c == "dream:"+e.fix.c.ID || c == "turns:"+e.fix.c.ID {
			first = c
			break
		}
	}
	if first != "" && first != "dream:"+e.fix.c.ID {
		t.Fatalf("first duty for the visited coordinator = %s, want dream", first)
	}
}
