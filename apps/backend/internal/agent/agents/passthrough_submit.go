package agents

// DefaultPassthroughSubmitSequence is appended after PTY prompt text when an agent
// omits PassthroughConfig.SubmitSequence. Most TUI CLIs expect carriage-return.
const DefaultPassthroughSubmitSequence = "\r"

// EffectiveSubmitSequence returns the bytes to append after passthrough stdin text.
// Agents that leave SubmitSequence empty inherit DefaultPassthroughSubmitSequence.
func EffectiveSubmitSequence(seq string) string {
	if seq == "" {
		return DefaultPassthroughSubmitSequence
	}
	return seq
}
