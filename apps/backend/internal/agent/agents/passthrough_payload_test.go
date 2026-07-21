package agents

import (
	"testing"
	"time"
)

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

// Claude's PassthroughConfig sets SubmitDelay so the submit byte arrives as a
// discrete keystroke (defeats Ink's paste-burst detection). Single-line and
// multi-line prompts both emit body+submit as two chunks; the submit chunk
// carries DelayBefore = SubmitDelay.
func TestPlanPassthroughStdinChunks_ClaudeSplitsSubmit(t *testing.T) {
	cfg := NewClaudeACP().PassthroughConfig()
	if cfg.SubmitDelay <= 0 {
		t.Fatalf("Claude config must set SubmitDelay > 0 for paste-burst workaround, got %v", cfg.SubmitDelay)
	}

	for _, prompt := range []string{"hello", "### Review Comments\n\n> fix"} {
		chunks := PlanPassthroughStdinChunks(prompt, cfg)
		if len(chunks) != 2 {
			t.Fatalf("prompt %q: got %d chunks, want 2 (body, submit): %#v", prompt, len(chunks), chunks)
		}
		if chunks[0].Data != prompt {
			t.Errorf("prompt %q: body chunk = %q, want verbatim prompt", prompt, chunks[0].Data)
		}
		if chunks[0].DelayBefore != 0 {
			t.Errorf("prompt %q: body chunk DelayBefore = %v, want 0", prompt, chunks[0].DelayBefore)
		}
		if chunks[1].Data != "\r" {
			t.Errorf("prompt %q: submit chunk = %q, want \\r", prompt, chunks[1].Data)
		}
		if chunks[1].DelayBefore != cfg.SubmitDelay {
			t.Errorf("prompt %q: submit DelayBefore = %v, want %v", prompt, chunks[1].DelayBefore, cfg.SubmitDelay)
		}
	}
}

// Non-Claude TUIs (SubmitDelay == 0) keep the single-atomic-write semantics so we
// don't accidentally regress Cursor/Codex/OpenCode by splitting their submit.
func TestPlanPassthroughStdinChunks_AtomicWhenNoDelay(t *testing.T) {
	cfg := PassthroughConfig{SubmitSequence: "\r"}
	chunks := PlanPassthroughStdinChunks("hello", cfg)
	if len(chunks) != 1 {
		t.Fatalf("got %d chunks, want 1 atomic write: %#v", len(chunks), chunks)
	}
	if chunks[0].DelayBefore != 0 {
		t.Errorf("atomic chunk DelayBefore = %v, want 0", chunks[0].DelayBefore)
	}
	if chunks[0].Data != "hello\r" {
		t.Errorf("atomic chunk Data = %q, want \"hello\\r\"", chunks[0].Data)
	}
}

// Sanity: a custom config with SubmitDelay set but no Claude-specific flags
// also splits — the field is the lever, not the Claude struct identity.
func TestPlanPassthroughStdinChunks_SubmitDelayDrivesSplit(t *testing.T) {
	cfg := PassthroughConfig{SubmitSequence: "\r", DisableBracketedPaste: true, SubmitDelay: 50 * time.Millisecond}
	chunks := PlanPassthroughStdinChunks("hi", cfg)
	if len(chunks) != 2 || chunks[1].DelayBefore != 50*time.Millisecond || chunks[1].Data != "\r" {
		t.Fatalf("expected split with 50ms delay before \\r, got %#v", chunks)
	}
}
