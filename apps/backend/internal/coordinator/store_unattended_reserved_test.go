package coordinator

import (
	"context"
	"testing"
)

func TestRecordUnattendedDenial_UnboundRowWithReservedIDDeniesOnlyThatTurn(t *testing.T) {
	cases := []struct {
		name     string
		reserved any
		active   string
		want     bool
	}{
		{"carries the reserved id", "rt", "rt", true},
		{"another turn", "rt", "manager-turn", false},
		{"null reserved id", nil, "manager-turn", true},
		{"empty reserved id", "", "manager-turn", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := newTestStore(t)
			c := newTestCoordinator(t, s, "ws-1")
			insertBoundTurn(t, s, c, "turn-1", "conv", "sess", nil, nil)
			mustExec(t, s, `UPDATE coordinator_unattended_turns SET reserved_turn_id = ? WHERE id = ?`, tc.reserved, "turn-1")
			_, matched, err := s.RecordUnattendedDenial(context.Background(), "conv", "sess", "pending-1", tc.active)
			if err != nil || matched != tc.want {
				t.Fatalf("matched=%v err=%v, want %v", matched, err, tc.want)
			}
			wantCount := 0
			if tc.want {
				wantCount = 1
			}
			if got := deniedCount(t, s, "turn-1"); got != wantCount {
				t.Fatalf("denied_permissions = %d, want %d", got, wantCount)
			}
		})
	}
}

func TestRecordUnattendedDenial_BoundRowIgnoresTheReservedID(t *testing.T) {
	s := newTestStore(t)
	c := newTestCoordinator(t, s, "ws-1")
	insertBoundTurn(t, s, c, "turn-1", "conv", "sess", "st-1", nil)
	mustExec(t, s, `UPDATE coordinator_unattended_turns SET reserved_turn_id = ? WHERE id = ?`, "st-1", "turn-1")
	if _, matched, err := s.RecordUnattendedDenial(context.Background(), "conv", "sess", "pending-1", "st-1"); err != nil || !matched {
		t.Fatalf("matched=%v err=%v, want a match", matched, err)
	}
	if _, matched, _ := s.RecordUnattendedDenial(context.Background(), "conv", "sess", "pending-2", "other"); matched {
		t.Fatal("a bound row must not match another turn")
	}
}
