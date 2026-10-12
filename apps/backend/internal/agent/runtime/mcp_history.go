package runtime

import (
	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

// LoadMCPAttachmentHistory decodes persisted MCP attachment metadata through
// the public runtime seam.
func LoadMCPAttachmentHistory(raw any) (streams.MCPAttachmentHistory, bool) {
	return lifecycle.LoadMCPAttachmentHistory(raw)
}
