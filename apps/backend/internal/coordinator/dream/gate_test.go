package dream

import (
	"strings"
	"testing"
	"time"
)

var (
	w0 = time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	w1 = w0.Add(24 * time.Hour)
)

func gateIn() GateInput {
	turns := map[string]CitedTurn{
		"t1":    {CoordinatorID: "c", Trigger: "wake", StartedAt: w0.Add(time.Hour)},
		"t2":    {CoordinatorID: "c", Trigger: "message", StartedAt: w0.Add(2 * time.Hour)},
		"other": {CoordinatorID: "x", Trigger: "wake", StartedAt: w0.Add(time.Hour)},
		"dream": {CoordinatorID: "c", Trigger: "dream", StartedAt: w0.Add(time.Hour)},
		"old":   {CoordinatorID: "c", Trigger: "wake", StartedAt: w0.Add(-time.Second)},
		"edge":  {CoordinatorID: "c", Trigger: "wake", StartedAt: w1},
		"start": {CoordinatorID: "c", Trigger: "wake", StartedAt: w0},
	}
	orders := map[string]StandingOrderRef{
		"so1": {CoordinatorID: "c"}, "so-retired": {CoordinatorID: "c", Retired: true}, "so-foreign": {CoordinatorID: "x"},
	}
	return GateInput{
		CoordinatorID: "c", WindowStart: w0, WindowEnd: w1, ContextLimit: 4000,
		Turn:          func(id string) (CitedTurn, bool) { t, ok := turns[id]; return t, ok },
		Order:         func(id string) (StandingOrderRef, bool) { o, ok := orders[id]; return o, ok },
		HasCredential: func(s string) bool { return strings.Contains(s, "kandev_pat_") },
	}
}

func item(kind, text, target string, cited ...string) Item {
	return Item{Kind: kind, Text: text, TargetID: target, CitedTurnIDs: cited}
}

func TestGate(t *testing.T) {
	cases := []struct {
		name string
		it   Item
		want string
	}{
		{"pass", item(KindNoteAdd, "ok", "", "t1", "t2"), GatePass},
		{"window start inclusive", item(KindNoteAdd, "ok", "", "t1", "start"), GatePass},
		{"window end exclusive", item(KindNoteAdd, "ok", "", "t1", "edge"), GateCitationUnresolved},
		{"size", item(KindNoteAdd, strings.Repeat("x", 501), "", "t1", "t2"), GateSize},
		{"size 500 ok", item(KindNoteAdd, strings.Repeat("x", 500), "", "t1", "t2"), GatePass},
		{"context diff uses context limit", item(KindContextDiff, strings.Repeat("x", 3000), "", "t1", "t2"), GatePass},
		{"context diff over limit", item(KindContextDiff, strings.Repeat("x", 4001), "", "t1", "t2"), GateSize},
		{"thin", item(KindNoteAdd, "ok", "", "t1", "t1"), GateThinEvidence},
		{"missing cite", item(KindNoteAdd, "ok", "", "t1", "nope"), GateCitationUnresolved},
		{"foreign cite", item(KindNoteAdd, "ok", "", "t1", "other"), GateCitationUnresolved},
		{"dream cite", item(KindNoteAdd, "ok", "", "t1", "dream"), GateCitationUnresolved},
		{"outside window", item(KindNoteAdd, "ok", "", "t1", "old"), GateCitationUnresolved},
		{"credential", item(KindNoteAdd, "use kandev_pat_abc", "", "t1", "t2"), GateCredential},
		{"forbidden target", item(KindStandingOrderAdd, "ok", "Autonomy", "t1", "t2"), GateForbiddenTarget},
		{"forbidden phrase", item(KindStandingOrderAdd, "Raise the cost ceiling to 5 USD", "", "t1", "t2"), GateForbiddenTarget},
		{"benign text", item(KindStandingOrderAdd, "Ask before creating tasks", "", "t1", "t2"), GatePass},
		{"note update has no target", item(KindNoteUpdate, "ok", "n1", "t1", "t2"), GateNoTarget},
		{"note retire has no target", item(KindNoteRetire, "ok", "n1", "t1", "t2"), GateNoTarget},
		{"retire ok", item(KindStandingOrderRetire, "ok", "so1", "t1", "t2"), GatePass},
		{"retire retired", item(KindStandingOrderRetire, "ok", "so-retired", "t1", "t2"), GateBadTarget},
		{"retire foreign", item(KindStandingOrderRetire, "ok", "so-foreign", "t1", "t2"), GateBadTarget},
		{"retire missing", item(KindStandingOrderRetire, "ok", "nope", "t1", "t2"), GateBadTarget},
		{"retire empty target", item(KindStandingOrderRetire, "ok", "", "t1", "t2"), GateBadTarget},
		{"order: size before thin", item(KindNoteAdd, strings.Repeat("x", 501), "", "t1"), GateSize},
		{"order: thin before citation", item(KindNoteAdd, "ok", "", "nope"), GateThinEvidence},
		{"order: citation before credential", item(KindNoteAdd, "kandev_pat_x", "", "t1", "nope"), GateCitationUnresolved},
		{"order: credential before forbidden", item(KindStandingOrderAdd, "kandev_pat_x", "autonomy", "t1", "t2"), GateCredential},
		{"order: forbidden before no_target", item(KindNoteUpdate, "ok", "ceiling", "t1", "t2"), GateForbiddenTarget},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			in := gateIn()
			if got := Check(c.it, in); got != c.want {
				t.Fatalf("Check = %q, want %q", got, c.want)
			}
			if again := Check(c.it, in); again != c.want {
				t.Fatalf("second Check = %q, not deterministic", again)
			}
		})
	}
}
