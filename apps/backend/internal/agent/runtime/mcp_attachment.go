package runtime

import (
	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

// LoadMCPAttachmentHistory decodes persisted evidence through the public runtime
// boundary, preserving the lifecycle schema and current-attempt validation.
func LoadMCPAttachmentHistory(raw any) (streams.MCPAttachmentHistory, bool) {
	return lifecycle.LoadMCPAttachmentHistory(raw)
}
