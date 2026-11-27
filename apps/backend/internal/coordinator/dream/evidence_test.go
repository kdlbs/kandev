package dream

import (
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestEvidenceCapsAndDropsOldest(t *testing.T) {
	var turns []Turn
	for i := 0; i < 300; i++ {
		turns = append(turns, Turn{ID: fmt.Sprintf("t%03d", i), Trigger: "wake", Verdict: "ok", Calls: map[string]int{"b": 1, "a": 2}})
	}
	e := BuildEvidence(turns, nil)
	if len(e.TurnIDs) != MaxEvidenceTurns || e.TurnIDs[0] != "t000" || e.TurnIDs[199] != "t199" {
		t.Fatalf("turn ids = %d first %s", len(e.TurnIDs), e.TurnIDs[0])
	}
	if !strings.Contains(e.Message, "calls=[a=2 b=1]") {
		t.Fatalf("calls not sorted: %s", e.Message[:200])
	}
}

func TestEvidenceCharCap(t *testing.T) {
	big := map[string]int{}
	for i := 0; i < 200; i++ {
		big[fmt.Sprintf("action_%03d", i)] = i
	}
	var turns []Turn
	for i := 0; i < 100; i++ {
		turns = append(turns, Turn{ID: fmt.Sprintf("t%03d", i), Trigger: "wake", Calls: big})
	}
	var decs []Decision
	for i := 0; i < 200; i++ {
		decs = append(decs, Decision{ProposalID: fmt.Sprintf("p%d", i), Title: strings.Repeat("é", 300)})
	}
	e := BuildEvidence(turns, decs)
	if n := utf8.RuneCountInString(e.Message); n > MaxEvidenceChars {
		t.Fatalf("message has %d runes", n)
	}
	if n := utf8.RuneCountInString(Frame + e.Message); n > MaxEvidenceChars {
		t.Fatalf("frame plus message has %d runes, cap %d", n, MaxEvidenceChars)
	}
	if len(e.TurnIDs) == 0 || len(e.TurnIDs) >= 100 {
		t.Fatalf("turns kept = %d, want some but not all", len(e.TurnIDs))
	}
	if strings.Contains(e.Message, strings.Repeat("é", 81)) {
		t.Fatal("title not truncated to 80 characters")
	}
}

func TestEvidenceIsData(t *testing.T) {
	e := BuildEvidence(nil, []Decision{{ProposalID: "p", Title: "x"}})
	if !strings.HasPrefix(e.Message, "<data") || !strings.HasSuffix(e.Message, "</data>\n") {
		t.Fatalf("message not wrapped: %q", e.Message)
	}
	if !strings.Contains(e.Message, "cost_subcents=unknown") {
		t.Fatal("nil cost must read unknown")
	}
}

func TestInputHashVaries(t *testing.T) {
	e := BuildEvidence([]Turn{{ID: "a"}}, nil)
	base := InputHash(e, "m1")
	if base != InputHash(e, "m1") {
		t.Fatal("hash not stable")
	}
	if base == InputHash(e, "m2") || base == InputHash(BuildEvidence([]Turn{{ID: "b"}}, nil), "m1") {
		t.Fatal("hash must cover model and evidence")
	}
}
