package instructionrefs_test

import (
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/common/instructionrefs"
)

func TestRewrite_BacktickWrapped(t *testing.T) {
	in := "Read `./HEARTBEAT.md` for the checklist."
	out := instructionrefs.Rewrite(in, "/runtime/instructions/abc")
	want := "Read `/runtime/instructions/abc/HEARTBEAT.md` for the checklist."
	if out != want {
		t.Errorf("Rewrite(...) = %q, want %q", out, want)
	}
}

func TestRewrite_MultipleSiblings(t *testing.T) {
	in := "See ./HEARTBEAT.md and ./SOUL.md for context."
	out := instructionrefs.Rewrite(in, "/abs/dir")
	if !strings.Contains(out, "/abs/dir/HEARTBEAT.md") {
		t.Errorf("HEARTBEAT not rewritten: %q", out)
	}
	if !strings.Contains(out, "/abs/dir/SOUL.md") {
		t.Errorf("SOUL not rewritten: %q", out)
	}
	if strings.Contains(out, "./HEARTBEAT.md") || strings.Contains(out, "./SOUL.md") {
		t.Errorf("relative refs leaked through: %q", out)
	}
}

func TestRewrite_LeavesParentTraversalAlone(t *testing.T) {
	// `../FOO.md` must not be rewritten — only `./X.md` siblings are.
	in := "../FOO.md and ./BAR.md"
	out := instructionrefs.Rewrite(in, "/abs/dir")
	if !strings.Contains(out, "../FOO.md") {
		t.Errorf("parent traversal rewritten: %q", out)
	}
	if !strings.Contains(out, "/abs/dir/BAR.md") {
		t.Errorf("sibling not rewritten: %q", out)
	}
}

func TestRewrite_EmptyDir(t *testing.T) {
	in := "Read ./HEARTBEAT.md."
	if got := instructionrefs.Rewrite(in, ""); got != in {
		t.Errorf("empty dir should leave content unchanged: got %q", got)
	}
}

func TestRewrite_NoMatches(t *testing.T) {
	in := "No sibling refs here."
	if got := instructionrefs.Rewrite(in, "/abs"); got != in {
		t.Errorf("Rewrite changed content with no refs: %q", got)
	}
}
