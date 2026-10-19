package replay

import (
	"testing"

	"github.com/kandev/kandev/internal/coordinator/replay/stub"
)

func TestKey(t *testing.T) {
	if got := Key(ProposalCreateTask, "ignored", "wf", "  Hello \t WORLD\n"); got != "wf|hello world" {
		t.Fatalf("create_task key = %q", got)
	}
	if got := Key(ProposalMove, "t1", "wf", "title"); got != "move|t1" {
		t.Fatalf("move key = %q", got)
	}
	if got := Key(ProposalMessage, "t1", "", ""); got != "message|t1" {
		t.Fatalf("message key = %q", got)
	}
	if got := Normalise("ÄÖ  Ü"); got != "äö ü" {
		t.Fatalf("unicode lower-casing not applied: %q", got)
	}
	keys, err := stub.Parse(`[{"kind":"create_task","workflow_id":"wf","title":" Hello   WORLD "},{"kind":"move","target_task_id":"t1"}]`)
	if err != nil || len(keys) != 2 || keys[0] != Key(ProposalMove, "t1", "", "") || keys[1] != Key(ProposalCreateTask, "", "wf", " Hello   WORLD ") {
		t.Fatalf("stub keys %v (%v) differ from Key", keys, err)
	}
}
