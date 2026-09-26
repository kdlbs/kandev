package agents

import (
	"strings"
	"time"
	"unicode/utf8"
)

const (
	bracketedPasteStart = "\x1b[200~"
	bracketedPasteEnd   = "\x1b[201~"

	// passthroughRawSafeWriteBytes caps one unframed PTY write below the host's
	// single-read capacity (1022 bytes on macOS). A TUI can drop whole reads of
	// an unframed burst without reporting it, so an unframed write never exceeds
	// one read. Larger bodies are framed as a bracketed paste, which the TUI
	// absorbs in one piece at any length.
	passthroughRawSafeWriteBytes = 400

	// passthroughRawWriteInterval spaces consecutive unframed writes to give the
	// TUI time to drain one read before the next arrives. PTY writes provide no
	// acknowledgement from the receiving TUI, so this is an empirical pacing
	// policy rather than a delivery receipt.
	passthroughRawWriteInterval = 30 * time.Millisecond
)

// PassthroughStdinChunk is one PTY stdin write planned for a passthrough prompt.
// DelayBefore is the pause applied before writing this chunk. The first chunk
// carries none; continuation body chunks on the unframed path carry
// passthroughRawWriteInterval; a separate submit chunk carries
// PassthroughConfig.SubmitDelay so the submit byte arrives as a discrete keystroke
// rather than being absorbed into an Ink-style paste burst.
type PassthroughStdinChunk struct {
	Data        string
	DelayBefore time.Duration
}

// PassthroughSubmitSequence returns the byte sequence to append after passthrough
// stdin text. Empty SubmitSequence inherits DefaultPassthroughSubmitSequence ("\r").
func PassthroughSubmitSequence(cfg PassthroughConfig) string {
	return EffectiveSubmitSequence(cfg.SubmitSequence)
}

// stripPasteMarkers removes bracketed-paste delimiters from a prompt before it
// is delivered. A marker carried inside the body would end the paste early and
// let the remainder reach the TUI as terminal input outside the prompt, which
// for a CLI agent means unintended commands.
//
// Replace until stable: a single pass can be evaded by nesting a marker inside
// itself (e.g. "\x1b[2\x1b[201~01~" collapses to a live marker after one pass).
func stripPasteMarkers(value string) string {
	for strings.Contains(value, bracketedPasteStart) || strings.Contains(value, bracketedPasteEnd) {
		value = strings.ReplaceAll(value, bracketedPasteStart, "")
		value = strings.ReplaceAll(value, bracketedPasteEnd, "")
	}
	return value
}

// framePassthroughBody reports whether the body travels as a bracketed paste.
// Framing is what lets a body outgrow the host's single-read capacity: the TUI
// absorbs a delimited paste in one piece, while an equally large unframed burst
// loses whole reads. A short single-line body is written unframed.
func framePassthroughBody(prompt string, cfg PassthroughConfig) bool {
	if cfg.DisableBracketedPaste {
		return false
	}
	return strings.Contains(prompt, "\n") || len(prompt) > passthroughRawSafeWriteBytes
}

// planBodyChunks plans the PTY writes that carry the prompt body, excluding the
// submit sequence. A framed body is one write at any length; an unframed body is
// paced so no single write exceeds what the TUI can absorb in one read.
func planBodyChunks(prompt string, cfg PassthroughConfig) []PassthroughStdinChunk {
	if framePassthroughBody(prompt, cfg) {
		body := stripPasteMarkers(prompt)
		return []PassthroughStdinChunk{{Data: bracketedPasteStart + body + bracketedPasteEnd}}
	}
	return paceRawBody(prompt)
}

// paceRawBody splits an unframed body into writes within the raw-safe size,
// cutting on rune boundaries so no write ends inside a multi-byte character.
// Every continuation carries the inter-write delay; the first carries none.
func paceRawBody(body string) []PassthroughStdinChunk {
	if len(body) <= passthroughRawSafeWriteBytes {
		return []PassthroughStdinChunk{{Data: body}}
	}
	var chunks []PassthroughStdinChunk
	for len(body) > 0 {
		cut := passthroughRawSafeWriteBytes
		if cut >= len(body) {
			cut = len(body)
		} else {
			for cut > 0 && !utf8.RuneStart(body[cut]) {
				cut--
			}
			if cut == 0 {
				// Not valid UTF-8; fall back to the byte budget rather than
				// looping forever on a prompt we cannot split cleanly.
				cut = passthroughRawSafeWriteBytes
			}
		}
		var delay time.Duration
		if len(chunks) > 0 {
			delay = passthroughRawWriteInterval
		}
		chunks = append(chunks, PassthroughStdinChunk{Data: body[:cut], DelayBefore: delay})
		body = body[cut:]
	}
	return chunks
}

// BuildPassthroughPayload assembles the bytes for one atomic PTY stdin write.
// The body is framed as a bracketed paste unless the agent cannot accept the
// delimiters, so embedded newlines are not treated as premature Enter presses
// and a long body is not split across terminal reads the TUI can drop.
func BuildPassthroughPayload(prompt string, cfg PassthroughConfig) string {
	if framePassthroughBody(prompt, cfg) {
		body := stripPasteMarkers(prompt)
		return bracketedPasteStart + body + bracketedPasteEnd + PassthroughSubmitSequence(cfg)
	}
	return prompt + PassthroughSubmitSequence(cfg)
}

// PlanPassthroughStdinChunks plans PTY stdin writes for a passthrough prompt.
// When SubmitDelay > 0 (Claude path) the submit byte is emitted as its own chunk
// with DelayBefore set, so it arrives as a discrete keystroke rather than
// trailing bytes of the body. Otherwise the submit sequence rides on the final
// body write, preserving the single-write behavior for TUIs that handle
// prompt+submit in one read (Cursor, Codex, OpenCode).
func PlanPassthroughStdinChunks(prompt string, cfg PassthroughConfig) []PassthroughStdinChunk {
	chunks := planBodyChunks(prompt, cfg)
	submit := PassthroughSubmitSequence(cfg)
	if cfg.SubmitDelay > 0 {
		return append(chunks, PassthroughStdinChunk{Data: submit, DelayBefore: cfg.SubmitDelay})
	}
	chunks[len(chunks)-1].Data += submit
	return chunks
}

// PlanPassthroughStdinWrites is the legacy string-only view of PlanPassthroughStdinChunks.
// Retained for tests and call sites that don't need to honor per-chunk delays.
func PlanPassthroughStdinWrites(prompt string, cfg PassthroughConfig) []string {
	chunks := PlanPassthroughStdinChunks(prompt, cfg)
	out := make([]string, len(chunks))
	for i, c := range chunks {
		out[i] = c.Data
	}
	return out
}
