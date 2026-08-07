package acp

import (
	"encoding/base64"
	"strings"

	"github.com/coder/acp-go-sdk"
	"github.com/kandev/kandev/internal/agentctl/acpcompat"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// clientCapabilitiesForAgent builds the ACP client capabilities for one agent.
// The meta map is agent-scoped because cursor-agent reads it to choose a model
// picker mode (see acpcompat.ClientCapabilityMeta); every other agent sees the
// capabilities kandev has always sent.
func clientCapabilitiesForAgent(agentID string) acp.ClientCapabilities {
	return acp.ClientCapabilities{
		Meta: acpcompat.ClientCapabilityMeta(agentID, map[string]any{"terminal_output": true}),
	}
}

// derefStr safely dereferences a pointer to a string or named string type,
// returning empty string if nil.
func derefStr[T ~string](s *T) string {
	if s != nil {
		return string(*s)
	}
	return ""
}

// buildResourceBlock constructs an ACP ResourceBlock from a MessageAttachment.
// Text-based MIME types use TextResourceContents; everything else uses BlobResourceContents.
func buildResourceBlock(att v1.MessageAttachment) acp.ContentBlock {
	uri := att.Name // Use filename as URI if no explicit URI
	if uri == "" {
		uri = "attachment"
	}
	if isTextMimeType(att.MimeType) {
		text := att.Data
		if decoded, err := base64.StdEncoding.DecodeString(att.Data); err == nil {
			text = string(decoded)
		}
		return acp.ResourceBlock(acp.EmbeddedResourceResource{
			TextResourceContents: &acp.TextResourceContents{
				Uri:      uri,
				Text:     text,
				MimeType: acp.Ptr(att.MimeType),
			},
		})
	}
	return acp.ResourceBlock(acp.EmbeddedResourceResource{
		BlobResourceContents: &acp.BlobResourceContents{
			Uri:      uri,
			Blob:     att.Data,
			MimeType: acp.Ptr(att.MimeType),
		},
	})
}

// isTextMimeType returns true for MIME types that represent text content.
func isTextMimeType(mimeType string) bool {
	if strings.HasPrefix(mimeType, "text/") {
		return true
	}
	switch mimeType {
	case "application/json", "application/xml", "application/javascript",
		"application/typescript", "application/x-yaml", "application/toml",
		"application/x-sh", "application/sql":
		return true
	}
	return false
}
