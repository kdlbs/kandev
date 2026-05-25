package agents

import "strings"

const (
	bracketedPasteStart = "\x1b[200~"
	bracketedPasteEnd   = "\x1b[201~"
)

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

// PlanPassthroughStdinWrites returns PTY write chunk(s). Bracketed-paste prompts use one
// atomic write; Claude uses separate writes for the backslash-then-Enter submit workaround.
func PlanPassthroughStdinWrites(prompt string, cfg PassthroughConfig) []string {
	if cfg.DisableBracketedPaste && cfg.SubmitViaBackslashEnter {
		return []string{prompt, "\\", PassthroughSubmitSequence(cfg, false)}
	}
	return []string{BuildPassthroughPayload(prompt, cfg)}
}
