package acp

import "github.com/kandev/kandev/internal/agentctl/types/streams"

// enrichCursorPayload is intentionally minimal: Cursor ACP currently emits
// generic titles ("Read File") without locations or structured rawInput on
// updates. We do not infer paths from titles — the UI falls back to title +
// expandable rawOutput.
func enrichCursorPayload(payload *streams.NormalizedPayload, _ EnrichFrame) {
	_ = payload
}
