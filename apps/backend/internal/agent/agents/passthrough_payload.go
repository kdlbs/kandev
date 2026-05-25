package agents

import "strings"

const (
	bracketedPasteStart = "\x1b[200~"
	bracketedPasteEnd   = "\x1b[201~"
)

// PassthroughSubmitSequence returns the byte sequence to append after passthrough stdin
// text. When bracketedPaste is true and SubmitAfterBracketedPaste is set on the config,
// that override is used (Claude Code expects "\n" after paste end, not "\r").
func PassthroughSubmitSequence(cfg PassthroughConfig, bracketedPaste bool) string {
	if bracketedPaste && cfg.SubmitAfterBracketedPaste != "" {
		return cfg.SubmitAfterBracketedPaste
	}
	return EffectiveSubmitSequence(cfg.SubmitSequence)
}

// PlanPassthroughStdinWrites returns one or more chunks to write to the PTY.
// Single-line prompts are one write (text + submit). Multi-line prompts use bracketed
// paste for the body; the submit key is a separate write so TUIs that mishandle paste+submit
// in a single read (notably Claude Code) still see Enter as a distinct event.
func PlanPassthroughStdinWrites(prompt string, cfg PassthroughConfig) []string {
	if !strings.Contains(prompt, "\n") {
		return []string{prompt + PassthroughSubmitSequence(cfg, false)}
	}
	submit := PassthroughSubmitSequence(cfg, true)
	body := bracketedPasteStart + prompt + bracketedPasteEnd
	return []string{body, submit}
}

// BuildPassthroughPayload joins PlanPassthroughStdinWrites for callers that need a
// single string (e.g. tests). Prefer PlanPassthroughStdinWrites for live PTY writes.
func BuildPassthroughPayload(prompt string, cfg PassthroughConfig) string {
	chunks := PlanPassthroughStdinWrites(prompt, cfg)
	return strings.Join(chunks, "")
}
