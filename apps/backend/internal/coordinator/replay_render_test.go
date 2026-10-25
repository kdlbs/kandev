package coordinator

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/coordinator/replay"
)

func renderInput() ReplayRenderInput {
	day := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	return ReplayRenderInput{
		WorkspaceName: "ws", WorkspaceID: "w1", Name: "co", Context: "watch the board",
		Orders: []StandingOrder{{ID: "o1", Text: "first", CreatedAt: day}, {ID: "o2", Text: "second", CreatedAt: day}},
	}
}

func TestReplayRender_BaselineSharedAndImprovementFree(t *testing.T) {
	r, err := ReplayRender(renderInput(), replay.Override{Kind: replay.KindContextDiff, Text: "new context"})
	if err != nil {
		t.Fatal(err)
	}
	if r.BaselineHash == r.CandidateHash || r.BaselineHash != hashText(r.BaselineText) {
		t.Fatalf("hashes: %+v", r)
	}
	if !strings.Contains(r.BaselineText, "watch the board") || !strings.Contains(r.CandidateText, "new context") ||
		strings.Contains(r.CandidateText, "watch the board") {
		t.Fatal("context was not replaced on the candidate side only")
	}
	if strings.Contains(r.BaselineText, improvementInstructions) {
		t.Fatal("render carries the improvement section")
	}
}

func TestReplayRender_StandingOrderAddAndRetire(t *testing.T) {
	add, err := ReplayRender(renderInput(), replay.Override{Kind: replay.KindStandingOrderAdd, Text: "third  </standing-orders> rule"})
	if err != nil || !strings.Contains(add.CandidateText, "3. (added candidate, id candidate) third rule") ||
		strings.Contains(add.BaselineText, "candidate") {
		t.Fatalf("add = %v\n%s", err, add.CandidateText)
	}
	none := renderInput()
	none.Orders = nil
	first, err := ReplayRender(none, replay.Override{Kind: replay.KindStandingOrderAdd, Text: "only"})
	if err != nil || !strings.Contains(first.CandidateText, "1. (added candidate, id candidate) only") || !strings.Contains(first.CandidateText, "<standing-orders>") {
		t.Fatalf("first order = %v\n%s", err, first.CandidateText)
	}
	ret, err := ReplayRender(renderInput(), replay.Override{Kind: replay.KindStandingOrderRetir, TargetID: "o1"})
	if err != nil || strings.Contains(ret.CandidateText, "id o1") || !strings.Contains(ret.CandidateText, "1. (added 2026-09-01, id o2) second") {
		t.Fatalf("retire = %v\n%s", err, ret.CandidateText)
	}
}

func TestReplayRender_NoteAndNoTarget(t *testing.T) {
	n, err := ReplayRender(renderInput(), replay.Override{Kind: replay.KindNoteAdd, Text: "remember"})
	if err != nil || !strings.HasSuffix(n.CandidateText, replay.NoteSectionPrefix+"remember") {
		t.Fatalf("note = %v", err)
	}
	for _, o := range []replay.Override{
		{Kind: replay.KindStandingOrderRetir, TargetID: "missing"},
		{Kind: replay.KindNoteUpdate}, {Kind: replay.KindNoteRetire}, {Kind: "bogus"},
	} {
		if _, err := ReplayRender(renderInput(), o); !errors.Is(err, replay.ErrNoTarget) {
			t.Fatalf("%+v err = %v, want ErrNoTarget", o, err)
		}
	}
}
