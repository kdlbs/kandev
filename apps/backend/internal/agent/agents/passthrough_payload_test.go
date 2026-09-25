package agents

import (
	"strings"
	"testing"
	"time"
	"unicode/utf8"
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
// discrete keystroke. The body is one chunk and the submit byte another; a
// short single-line prompt keeps its exact bytes, a multi-line prompt is framed
// as a bracketed paste so the TUI absorbs it as one input.
func TestPlanPassthroughStdinChunks_ClaudeSplitsSubmit(t *testing.T) {
	cfg := NewClaudeACP().PassthroughConfig()
	if cfg.SubmitDelay <= 0 {
		t.Fatalf("Claude config must set SubmitDelay > 0 for paste-burst workaround, got %v", cfg.SubmitDelay)
	}

	cases := map[string]string{
		"hello":                        "hello",
		"### Review Comments\n\n> fix": bracketedPasteStart + "### Review Comments\n\n> fix" + bracketedPasteEnd,
	}
	for prompt, wantBody := range cases {
		chunks := PlanPassthroughStdinChunks(prompt, cfg)
		if len(chunks) != 2 {
			t.Fatalf("prompt %q: got %d chunks, want 2 (body, submit): %#v", prompt, len(chunks), chunks)
		}
		if chunks[0].Data != wantBody {
			t.Errorf("prompt %q: body chunk = %q, want %q", prompt, chunks[0].Data, wantBody)
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

// A body larger than one terminal read must travel as one framed write, with the
// prompt recoverable byte-for-byte from between the markers.
func TestPlanPassthroughStdinChunks_FramesBodyLargerThanOneRead(t *testing.T) {
	cfg := NewClaudeACP().PassthroughConfig()
	prompt := strings.Repeat("строка контекста для агента\n", 400)
	if len(prompt) < 8000 {
		t.Fatalf("fixture too small to exercise multi-read delivery: %d bytes", len(prompt))
	}

	chunks := PlanPassthroughStdinChunks(prompt, cfg)
	if len(chunks) != 2 {
		t.Fatalf("got %d chunks, want 2 (framed body, submit): lengths %v", len(chunks), chunkLengths(chunks))
	}
	body := chunks[0].Data
	if !strings.HasPrefix(body, bracketedPasteStart) || !strings.HasSuffix(body, bracketedPasteEnd) {
		t.Fatalf("body is not bracketed: prefix=%q suffix=%q", body[:6], body[len(body)-6:])
	}
	inner := strings.TrimSuffix(strings.TrimPrefix(body, bracketedPasteStart), bracketedPasteEnd)
	if inner != prompt {
		t.Errorf("framed body lost content: got %d bytes, want %d", len(inner), len(prompt))
	}
}

// A single-line prompt past the raw-safe write size is just as lossy unframed,
// so length alone must trigger framing.
func TestPlanPassthroughStdinChunks_FramesLongSingleLine(t *testing.T) {
	cfg := NewClaudeACP().PassthroughConfig()
	prompt := strings.Repeat("x", passthroughRawSafeWriteBytes+1)

	body := PlanPassthroughStdinChunks(prompt, cfg)[0].Data
	if !strings.HasPrefix(body, bracketedPasteStart) {
		t.Fatalf("long single-line body was not framed: %q...", body[:20])
	}
}

// A marker inside the prompt would end the paste early and let the remainder
// reach the TUI as terminal input outside the prompt.
func TestPlanPassthroughStdinChunks_NeutralizesFramingTerminators(t *testing.T) {
	cfg := NewClaudeACP().PassthroughConfig()
	prompt := "before\n" + bracketedPasteEnd + "rm -rf /\n" + bracketedPasteStart + "after"

	body := PlanPassthroughStdinChunks(prompt, cfg)[0].Data
	if strings.Count(body, bracketedPasteStart) != 1 || strings.Count(body, bracketedPasteEnd) != 1 {
		t.Fatalf("markers from the prompt survived framing: %q", body)
	}
	inner := strings.TrimSuffix(strings.TrimPrefix(body, bracketedPasteStart), bracketedPasteEnd)
	if strings.Contains(inner, "\x1b[") {
		t.Errorf("escape sequence left inside the framed body: %q", inner)
	}
	for _, want := range []string{"before", "rm -rf /", "after"} {
		if !strings.Contains(inner, want) {
			t.Errorf("neutralization dropped prompt text %q from %q", want, inner)
		}
	}
}

func TestPlanPassthroughStdinChunks_UnframedPreservesFramingMarkers(t *testing.T) {
	cfg := PassthroughConfig{SubmitSequence: "\r", DisableBracketedPaste: true}
	prompt := "literal " + bracketedPasteStart + " marker " + bracketedPasteEnd

	chunks := PlanPassthroughStdinChunks(prompt, cfg)
	if len(chunks) != 1 {
		t.Fatalf("got %d chunks, want one atomic write: %#v", len(chunks), chunks)
	}
	if got, want := chunks[0].Data, prompt+"\r"; got != want {
		t.Errorf("unframed payload = %q, want %q", got, want)
	}
	if got := BuildPassthroughPayload(prompt, cfg); got != prompt+"\r" {
		t.Errorf("BuildPassthroughPayload = %q, want literal markers preserved", got)
	}
}

func chunkLengths(chunks []PassthroughStdinChunk) []int {
	out := make([]int, len(chunks))
	for i, c := range chunks {
		out[i] = len(c.Data)
	}
	return out
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

// An agent that cannot accept paste framing must still never receive a burst
// larger than one terminal read, so its body is written in paced pieces.
func TestPlanPassthroughStdinChunks_PacesUnframedBody(t *testing.T) {
	cfg := PassthroughConfig{SubmitSequence: "\r", DisableBracketedPaste: true, SubmitDelay: 150 * time.Millisecond}
	prompt := strings.Repeat("unframed body line\n", 200)

	chunks := PlanPassthroughStdinChunks(prompt, cfg)
	if len(chunks) < 3 {
		t.Fatalf("got %d chunks for a %d-byte body, want it paced: %v", len(chunks), len(prompt), chunkLengths(chunks))
	}

	var body strings.Builder
	for i, chunk := range chunks[:len(chunks)-1] {
		if len(chunk.Data) > passthroughRawSafeWriteBytes {
			t.Errorf("body chunk %d is %d bytes, want at most %d", i, len(chunk.Data), passthroughRawSafeWriteBytes)
		}
		wantDelay := passthroughRawWriteInterval
		if i == 0 {
			wantDelay = 0
		}
		if chunk.DelayBefore != wantDelay {
			t.Errorf("body chunk %d DelayBefore = %v, want %v", i, chunk.DelayBefore, wantDelay)
		}
		body.WriteString(chunk.Data)
	}
	if body.String() != prompt {
		t.Errorf("paced body lost content: got %d bytes, want %d", body.Len(), len(prompt))
	}

	submit := chunks[len(chunks)-1]
	if submit.Data != "\r" || submit.DelayBefore != cfg.SubmitDelay {
		t.Errorf("submit chunk = %q after %v, want %q after %v", submit.Data, submit.DelayBefore, "\r", cfg.SubmitDelay)
	}
}

// Splitting by bytes would corrupt non-ASCII prompts.
func TestPlanPassthroughStdinChunks_UnframedSplitsOnRuneBoundaries(t *testing.T) {
	cfg := PassthroughConfig{SubmitSequence: "\r", DisableBracketedPaste: true, SubmitDelay: 150 * time.Millisecond}
	prompt := strings.Repeat("сообщение агенту ", 200)

	for i, chunk := range PlanPassthroughStdinChunks(prompt, cfg) {
		if !utf8.ValidString(chunk.Data) {
			t.Fatalf("chunk %d ends inside a multi-byte character: %q", i, chunk.Data)
		}
	}
}

// A TUI with no submit delay still gets a paced body; its submit sequence rides
// on the final write so prompt and submit stay one read for short prompts.
func TestPlanPassthroughStdinChunks_PacesUnframedBodyWithoutSubmitDelay(t *testing.T) {
	cfg := PassthroughConfig{SubmitSequence: "\r", DisableBracketedPaste: true}
	prompt := strings.Repeat("x", passthroughRawSafeWriteBytes*3)

	chunks := PlanPassthroughStdinChunks(prompt, cfg)
	if len(chunks) != 3 {
		t.Fatalf("got %d chunks, want 3 paced writes: %v", len(chunks), chunkLengths(chunks))
	}
	var joined strings.Builder
	for _, chunk := range chunks {
		joined.WriteString(chunk.Data)
	}
	if joined.String() != prompt+"\r" {
		t.Errorf("paced atomic path lost content: got %d bytes, want %d", joined.Len(), len(prompt)+1)
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
