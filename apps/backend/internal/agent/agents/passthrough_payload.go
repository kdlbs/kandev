package agents

import (
	"strings"
	"time"
)

const (
	bracketedPasteStart = "\x1b[200~"
	bracketedPasteEnd   = "\x1b[201~"
)

// PassthroughStdinChunk is one PTY stdin write planned for a passthrough prompt.
// DelayBefore is sleep applied before writing this chunk; it is zero on the first
// chunk and inherits PassthroughConfig.SubmitDelay on subsequent chunks. Splitting
// the prompt body from the submit byte with a delay defeats Ink-style paste-burst
// detection in TUIs like Claude Code (see PassthroughConfig.SubmitDelay).
type PassthroughStdinChunk struct {
	Data        string
	DelayBefore time.Duration
}

// PassthroughSubmitSequence returns the byte sequence to append after passthrough stdin
// text. When bracketedPaste is true and SubmitAfterBracketedPaste is set on the config,
// that override is used (Claude Code often needs "\r\n" after paste end).
func PassthroughSubmitSequence(cfg PassthroughConfig, bracketedPaste bool) string {
	if bracketedPaste && cfg.SubmitAfterBracketedPaste != "" {
		return cfg.SubmitAfterBracketedPaste
	}
	return EffectiveSubmitSequence(cfg.SubmitSequence)
}

// BuildPassthroughPayload assembles the bytes for one atomic PTY stdin write.
// Multi-line prompts use bracketed paste so embedded newlines are not treated as
// premature Enter presses, unless DisableBracketedPaste is set (Claude Code).
func BuildPassthroughPayload(prompt string, cfg PassthroughConfig) string {
	if cfg.DisableBracketedPaste || !strings.Contains(prompt, "\n") {
		return prompt + PassthroughSubmitSequence(cfg, false)
	}
	submit := PassthroughSubmitSequence(cfg, true)
	return bracketedPasteStart + prompt + bracketedPasteEnd + submit
}

// PlanPassthroughStdinChunks plans PTY stdin writes for a passthrough prompt.
// When SubmitDelay > 0 (Claude path) the prompt body and submit byte are emitted
// as two chunks with DelayBefore set on the submit chunk so it arrives as a
// discrete keystroke. Otherwise a single atomic write is returned, preserving
// today's behavior for TUIs that handle prompt+submit in one read (Cursor, Codex,
// OpenCode).
func PlanPassthroughStdinChunks(prompt string, cfg PassthroughConfig) []PassthroughStdinChunk {
	if cfg.SubmitDelay > 0 {
		submit := PassthroughSubmitSequence(cfg, false)
		body := prompt
		if !cfg.DisableBracketedPaste && strings.Contains(prompt, "\n") {
			body = bracketedPasteStart + prompt + bracketedPasteEnd
		}
		return []PassthroughStdinChunk{
			{Data: body},
			{Data: submit, DelayBefore: cfg.SubmitDelay},
		}
	}
	return []PassthroughStdinChunk{{Data: BuildPassthroughPayload(prompt, cfg)}}
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
