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

// PlanPassthroughStdinWrites returns PTY write chunk(s). Today this is always a single
// atomic write so bracketed-paste sequences are not split across syscalls.
func PlanPassthroughStdinWrites(prompt string, cfg PassthroughConfig) []string {
	return []string{BuildPassthroughPayload(prompt, cfg)}
}
