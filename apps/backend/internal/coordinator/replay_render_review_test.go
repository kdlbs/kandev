package coordinator

import (
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/coordinator/replay"
)

func TestReplayRenderDoesNotReplaceMarkerInExistingOrderText(t *testing.T) {
	in := renderInput()
	marker := "(added 0001-01-01, id candidate)"
	in.Orders[0].Text = "Keep this literal " + marker
	got, err := ReplayRender(in, replay.Override{Kind: replay.KindStandingOrderAdd, Text: "new rule"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got.CandidateText, in.Orders[0].Text) ||
		!strings.Contains(got.CandidateText, "3. (added candidate, id candidate) new rule") {
		t.Fatalf("existing order changed or synthetic order misdated:\n%s", got.CandidateText)
	}
}
