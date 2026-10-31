package acp

import (
	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

// newOpenCodeACPDialect describes the tested OpenCode ACP conversation
// contract. OpenCode keeps one initialized native session usable after a
// temporary provider admission refusal, so capacity recovery may continue in
// that same conversation. It is not a label for the CLI release or provider
// assertion, and it grants no transport-loss restoration support.
func newOpenCodeACPDialect() acpDialect {
	return acpDialect{
		capacityContinuationSupport: streams.CapacityContinuationOpenCodeLiveSessionV1,
	}
}
