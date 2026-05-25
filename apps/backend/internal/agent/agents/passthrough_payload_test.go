package agents

import "testing"

func TestPlanPassthroughStdinWrites_SingleLine(t *testing.T) {
	cfg := PassthroughConfig{SubmitSequence: "\r"}
	got := PlanPassthroughStdinWrites("hello", cfg)
	if len(got) != 1 || got[0] != "hello\r" {
		t.Fatalf("got %#v, want [\"hello\\r\"]", got)
	}
}

func TestPlanPassthroughStdinWrites_MultilineDefaultSubmit(t *testing.T) {
	cfg := PassthroughConfig{SubmitSequence: "\r"}
	got := PlanPassthroughStdinWrites("line1\nline2", cfg)
	if len(got) != 2 {
		t.Fatalf("got %d chunks, want 2: %#v", len(got), got)
	}
	wantBody := "\x1b[200~line1\nline2\x1b[201~"
	if got[0] != wantBody {
		t.Errorf("body = %q, want %q", got[0], wantBody)
	}
	if got[1] != "\r" {
		t.Errorf("submit = %q, want \\r", got[1])
	}
}

func TestPlanPassthroughStdinWrites_ClaudeMultilineUsesNewlineSubmit(t *testing.T) {
	cfg := NewClaudeACP().PassthroughConfig()
	got := PlanPassthroughStdinWrites("### Review Comments\n\n> fix", cfg)
	if len(got) != 2 {
		t.Fatalf("got %d chunks, want 2: %#v", len(got), got)
	}
	if got[1] != "\n" {
		t.Errorf("Claude submit after bracketed paste = %q, want \\n", got[1])
	}
}

func TestBuildPassthroughPayload_JoinsChunks(t *testing.T) {
	cfg := PassthroughConfig{SubmitSequence: "\r"}
	joined := BuildPassthroughPayload("a\nb", cfg)
	plan := PlanPassthroughStdinWrites("a\nb", cfg)
	if joined != plan[0]+plan[1] {
		t.Errorf("BuildPassthroughPayload = %q, want %q", joined, plan[0]+plan[1])
	}
}
