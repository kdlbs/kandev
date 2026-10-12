package main

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	acp "github.com/coder/acp-go-sdk"
)

var acpTraceMu sync.Mutex
var mockAgentConnectionID = fmt.Sprintf("%d-%d", os.Getpid(), time.Now().UnixNano())

// traceACP writes mock ACP requests only when an E2E test explicitly provides
// a trace path. The file gives browser tests evidence of the peer's observed
// native session identity and selector operations.
func traceACP(event, sessionID string, fields map[string]string) {
	tracePath := os.Getenv("E2E_MOCK_AGENT_ACP_TRACE_FILE")
	if tracePath == "" {
		return
	}
	record := map[string]string{
		"event":         event,
		"session_id":    sessionID,
		"process_id":    fmt.Sprintf("%d", os.Getpid()),
		"connection_id": mockAgentConnectionID,
	}
	for key, value := range fields {
		record[key] = value
	}
	encoded, err := json.Marshal(record)
	if err != nil {
		return
	}
	acpTraceMu.Lock()
	defer acpTraceMu.Unlock()
	file, err := os.OpenFile(tracePath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	_, _ = file.Write(append(encoded, '\n'))
	_ = file.Close()
}

type acpPromptBlockTrace struct {
	Type       string `json:"type"`
	MimeType   string `json:"mime_type,omitempty"`
	Name       string `json:"name,omitempty"`
	URI        string `json:"uri,omitempty"`
	ByteLength int    `json:"byte_length,omitempty"`
	SHA256     string `json:"sha256,omitempty"`
}

func summarizeACPPromptBlocks(blocks []acp.ContentBlock) []acpPromptBlockTrace {
	result := make([]acpPromptBlockTrace, 0, len(blocks))
	for _, block := range blocks {
		switch {
		case block.Text != nil:
			result = append(result, summarizePromptPayload("text", "", block.Text.Text))
		case block.Image != nil:
			payload, err := base64.StdEncoding.DecodeString(block.Image.Data)
			if err == nil {
				result = append(result, summarizePromptPayload("image", block.Image.MimeType, string(payload)))
			}
		case block.Audio != nil:
			payload, err := base64.StdEncoding.DecodeString(block.Audio.Data)
			if err == nil {
				result = append(result, summarizePromptPayload("audio", block.Audio.MimeType, string(payload)))
			}
		case block.ResourceLink != nil:
			summary := acpPromptBlockTrace{
				Type: "resource_link", Name: block.ResourceLink.Name, URI: block.ResourceLink.Uri,
			}
			if block.ResourceLink.MimeType != nil {
				summary.MimeType = *block.ResourceLink.MimeType
			}
			result = append(result, summary)
		case block.Resource != nil:
			resource := block.Resource.Resource
			switch {
			case resource.TextResourceContents != nil:
				text := resource.TextResourceContents
				summary := summarizePromptPayload("resource_text", stringValue(text.MimeType), text.Text)
				summary.URI = text.Uri
				result = append(result, summary)
			case resource.BlobResourceContents != nil:
				blob := resource.BlobResourceContents
				payload, err := base64.StdEncoding.DecodeString(blob.Blob)
				if err == nil {
					summary := summarizePromptPayload("resource_blob", stringValue(blob.MimeType), string(payload))
					summary.URI = blob.Uri
					result = append(result, summary)
				}
			}
		}
	}
	return result
}

func summarizePromptPayload(kind, mimeType, payload string) acpPromptBlockTrace {
	digest := sha256.Sum256([]byte(payload))
	return acpPromptBlockTrace{
		Type: kind, MimeType: mimeType, ByteLength: len(payload), SHA256: fmt.Sprintf("%x", digest),
	}
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}
