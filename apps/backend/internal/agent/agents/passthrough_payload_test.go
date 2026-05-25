package agents

import "testing"

func TestPlanPassthroughStdinWrites_SingleAtomicWrite(t *testing.T) {
	cfg := PassthroughConfig{SubmitSequence: "\r"}
	got := PlanPassthroughStdinWrites("line1\nline2", cfg)
	if len(got) != 1 {
		t.Fatalf("got %d chunks, want 1 atomic write: %#v", len(got), got)
	}
	want := "\x1b[200~line1\nline2\x1b[201~\r"
	if got[0] != want {
		t.Errorf("payload = %q, want %q", got[0], want)
	}
}

func TestPlanPassthroughStdinWrites_SingleLine(t *testing.T) {
	cfg := PassthroughConfig{SubmitSequence: "\r"}
	got := PlanPassthroughStdinWrites("hello", cfg)
	if len(got) != 1 || got[0] != "hello\r" {
		t.Fatalf("got %#v, want [\"hello\\r\"]", got)
	}
}

func TestPlanPassthroughStdinWrites_ClaudeMultilineUsesCRLFSubmit(t *testing.T) {
	cfg := NewClaudeACP().PassthroughConfig()
	got := PlanPassthroughStdinWrites("### Review Comments\n\n> fix", cfg)
	if len(got) != 1 {
		t.Fatalf("got %d chunks, want 1: %#v", len(got), got)
	}
	if !stringsHasSuffix(got[0], "\r\n") {
		t.Errorf("Claude payload must end with \\r\\n submit, got suffix %q", got[0][len(got[0])-4:])
	}
	if !stringsContains(got[0], "\x1b[200~") {
		t.Error("expected bracketed paste wrapper")
	}
}

func stringsHasSuffix(s, suffix string) bool {
	return len(s) >= len(suffix) && s[len(s)-len(suffix):] == suffix
}

func stringsContains(s, sub string) bool {
	return len(sub) == 0 || (len(s) >= len(sub) && indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
