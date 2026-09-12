package handlers

import "strings"

// composeInitialTaskBrief keeps the prepared session's original brief in the
// first direct prompt while preserving the user's instruction as a separate
// paragraph. Trimmed equality avoids repeating a brief that the user already
// supplied verbatim.
func composeInitialTaskBrief(brief, instruction string) string {
	brief = strings.TrimSpace(brief)
	instruction = strings.TrimSpace(instruction)
	if brief == "" {
		return instruction
	}
	if instruction == "" || brief == instruction {
		return brief
	}
	return brief + "\n\n" + instruction
}
