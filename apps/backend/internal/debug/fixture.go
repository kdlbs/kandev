package debug

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/agentctl/server/adapter/transport/acp"
	"github.com/kandev/kandev/internal/agentctl/server/adapter/transport/streamjson"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// Protocol and file type constants.
const (
	protocolACP        = "acp"
	protocolStreamJSON = "streamjson"
	filetypeUnknown    = "unknown"
)

// DiscoveredFile represents a discovered fixture file.
type DiscoveredFile struct {
	Path         string `json:"path"`
	Protocol     string `json:"protocol"`
	Agent        string `json:"agent,omitempty"`
	MessageCount int    `json:"message_count"`
	FileType     string `json:"file_type"` // "raw", "normalized", or "testdata"
}

// NormalizedFixture represents a test fixture with its normalized payload.
type NormalizedFixture struct {
	Protocol string                     `json:"protocol"`
	ToolName string                     `json:"tool_name"`
	ToolType string                     `json:"tool_type"`
	Input    map[string]any             `json:"input"`
	Payload  *streams.NormalizedPayload `json:"payload"`
}

// normalizedEventEntry represents a single entry in a normalized events file.
type normalizedEventEntry struct {
	Ts    int64               `json:"ts"`
	Event *streams.AgentEvent `json:"event"`
}

// fixtureInput represents a raw fixture from JSONL files.
type fixtureInput struct {
	Input    map[string]any `json:"input"`
	Expected map[string]any `json:"expected"`
}

// normalizer defines the interface for protocol-specific normalizers.
type normalizer interface {
	NormalizeToolCall(toolName string, args map[string]any) *streams.NormalizedPayload
}

// discoverFixtureFiles finds all fixture JSONL files in standard and custom locations.
func discoverFixtureFiles(baseDir string) ([]DiscoveredFile, error) {
	var allMatches []string

	// Pattern 1: Standard location - transport/*/testdata/*-messages.jsonl
	pattern1 := filepath.Join(baseDir, "transport", "*", "testdata", "*-messages.jsonl")
	if matches, err := filepath.Glob(pattern1); err == nil {
		allMatches = append(allMatches, matches...)
	}

	// Pattern 2: Backend root - any *.jsonl files (for debug files)
	backendRoot := filepath.Join(baseDir, "..", "..", "..", "..")
	pattern2 := filepath.Join(backendRoot, "*.jsonl")
	if matches, err := filepath.Glob(pattern2); err == nil {
		allMatches = append(allMatches, matches...)
	}

	var files []DiscoveredFile
	seen := make(map[string]bool)

	for _, fullPath := range allMatches {
		absPath, _ := filepath.Abs(fullPath)
		if seen[absPath] {
			continue
		}
		seen[absPath] = true

		filename := filepath.Base(fullPath)
		fileType, protocol, agent := parseDebugFilename(filename)
		count := countLines(fullPath)
		relPath, err := filepath.Rel(baseDir, fullPath)
		if err != nil {
			relPath = fullPath
		}
		files = append(files, DiscoveredFile{
			Path:         relPath,
			Protocol:     protocol,
			Agent:        agent,
			MessageCount: count,
			FileType:     fileType,
		})
	}
	return files, nil
}

// discoverNormalizedFiles finds all normalized event files (normalized-*.jsonl).
func discoverNormalizedFiles(baseDir string) ([]DiscoveredFile, error) {
	backendRoot := filepath.Join(baseDir, "..", "..", "..", "..")
	pattern := filepath.Join(backendRoot, "normalized-*.jsonl")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil, err
	}

	var files []DiscoveredFile
	for _, fullPath := range matches {
		filename := filepath.Base(fullPath)
		_, protocol, agent := parseDebugFilename(filename)
		count := countLines(fullPath)
		relPath, err := filepath.Rel(baseDir, fullPath)
		if err != nil {
			relPath = fullPath
		}
		files = append(files, DiscoveredFile{
			Path:         relPath,
			Protocol:     protocol,
			Agent:        agent,
			MessageCount: count,
			FileType:     "normalized",
		})
	}
	return files, nil
}

// readNormalizedEventsAsMessages reads a normalized events file and returns v1.Message objects.
func readNormalizedEventsAsMessages(filePath string) ([]*v1.Message, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open %s: %w", filePath, err)
	}
	defer func() { _ = file.Close() }()

	var messages []*v1.Message
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	index := 0
	for scanner.Scan() {
		var entry normalizedEventEntry
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			continue // Skip malformed lines
		}
		if entry.Event != nil {
			msg := agentEventToMessage(entry.Event, entry.Ts, index)
			if msg != nil {
				messages = append(messages, msg)
			}
			index++
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading %s: %w", filePath, err)
	}

	return messages, nil
}

// agentEventToMessage converts an AgentEvent to a v1.Message.
func agentEventToMessage(event *streams.AgentEvent, ts int64, index int) *v1.Message {
	if event == nil {
		return nil
	}

	// Map AgentEvent type to Message type
	msgType := mapEventTypeToMessageType(event.Type)
	if msgType == "" {
		return nil // Skip events we don't want to display
	}

	// Build content based on event type
	content := buildMessageContent(event)

	// Build metadata
	metadata := buildMessageMetadata(event)

	// Use timestamp from entry, or current time as fallback
	createdAt := time.Now()
	if ts > 0 {
		createdAt = time.UnixMilli(ts)
	}

	// Generate a unique ID for the message using index to ensure uniqueness
	msgID := fmt.Sprintf("msg-%d-%d", ts, index)

	return &v1.Message{
		ID:            msgID,
		TaskSessionID: event.SessionID,
		AuthorType:    "agent",
		Type:          msgType,
		Content:       content,
		Metadata:      metadata,
		CreatedAt:     createdAt,
	}
}

// mapEventTypeToMessageType converts AgentEvent type to Message type.
func mapEventTypeToMessageType(eventType string) string {
	switch eventType {
	case streams.EventTypeMessageChunk:
		return "message"
	case streams.EventTypeReasoning:
		return "thinking"
	case streams.EventTypeToolCall, streams.EventTypeToolUpdate:
		return "tool_call"
	case streams.EventTypePlan:
		return "todo"
	case streams.EventTypeError:
		return "error"
	case streams.EventTypePermissionRequest:
		return "permission_request"
	case streams.EventTypeComplete:
		return "status"
	case streams.EventTypeSessionStatus, streams.EventTypeContextWindow:
		return "" // Skip these events
	default:
		return "message"
	}
}

// buildMessageContent extracts the appropriate content from an AgentEvent.
func buildMessageContent(event *streams.AgentEvent) string {
	switch event.Type {
	case streams.EventTypeMessageChunk:
		return event.Text
	case streams.EventTypeReasoning:
		return event.ReasoningText
	case streams.EventTypeToolCall, streams.EventTypeToolUpdate:
		if event.ToolTitle != "" {
			return event.ToolTitle
		}
		return event.ToolName
	case streams.EventTypeError:
		return event.Error
	case streams.EventTypeComplete:
		return "Turn completed"
	case streams.EventTypePermissionRequest:
		return event.PermissionTitle
	default:
		return event.Text
	}
}

// buildMessageMetadata builds the metadata map for a Message from an AgentEvent.
func buildMessageMetadata(event *streams.AgentEvent) map[string]any {
	metadata := make(map[string]any)

	// Add tool-related metadata
	if event.ToolCallID != "" {
		metadata["tool_call_id"] = event.ToolCallID
	}
	if event.ToolName != "" {
		metadata["tool_name"] = event.ToolName
	}
	if event.ToolStatus != "" {
		metadata["status"] = event.ToolStatus
	}
	if event.Diff != "" {
		metadata["diff"] = event.Diff
	}

	// Add normalized payload if present
	if event.NormalizedPayload != nil {
		metadata["normalized"] = event.NormalizedPayload
	}

	// Add plan entries if present
	if len(event.PlanEntries) > 0 {
		metadata["plan_entries"] = event.PlanEntries
	}

	// Add permission request details
	if event.PendingID != "" {
		metadata["pending_id"] = event.PendingID
	}
	if len(event.PermissionOptions) > 0 {
		metadata["permission_options"] = event.PermissionOptions
	}
	if event.ActionType != "" {
		metadata["action_type"] = event.ActionType
	}
	if event.ActionDetails != nil {
		metadata["action_details"] = event.ActionDetails
	}

	// Add context window info if present
	if event.ContextWindowSize > 0 {
		metadata["context_window_size"] = event.ContextWindowSize
		metadata["context_window_used"] = event.ContextWindowUsed
		metadata["context_window_remaining"] = event.ContextWindowRemaining
	}

	return metadata
}

// parseDebugFilename extracts file type, protocol, and agent from debug filenames.
// Patterns:
//   - "raw-{protocol}-{agent}.jsonl" → ("raw", protocol, agent)
//   - "normalized-{protocol}-{agent}.jsonl" → ("normalized", protocol, agent)
//   - "{protocol}-messages.jsonl" → ("testdata", protocol, "")
//   - "{protocol}-{agent}.jsonl" → ("unknown", protocol, agent) [legacy]
func parseDebugFilename(filename string) (fileType, protocol, agent string) {
	name := strings.TrimSuffix(filename, ".jsonl")

	// Check for raw- prefix
	if rest, found := strings.CutPrefix(name, "raw-"); found {
		protocol, agent = splitProtocolAgent(rest)
		return "raw", protocol, agent
	}

	// Check for normalized- prefix
	if rest, found := strings.CutPrefix(name, "normalized-"); found {
		protocol, agent = splitProtocolAgent(rest)
		return "normalized", protocol, agent
	}

	// Check for testdata pattern: {protocol}-messages
	if protocol, found := strings.CutSuffix(name, "-messages"); found {
		return "testdata", protocol, ""
	}

	// Legacy pattern: {protocol}-{agent}
	protocol, agent = splitProtocolAgent(name)
	if isKnownProtocol(protocol) {
		return filetypeUnknown, protocol, agent
	}

	return filetypeUnknown, filetypeUnknown, ""
}

// splitProtocolAgent splits "{protocol}-{agent}" into protocol and agent.
func splitProtocolAgent(s string) (protocol, agent string) {
	idx := strings.Index(s, "-")
	if idx <= 0 {
		return s, ""
	}
	return s[:idx], s[idx+1:]
}

// parseProtocolFromFilename extracts protocol from filename patterns (legacy).
func parseProtocolFromFilename(filename string) string {
	_, protocol, _ := parseDebugFilename(filename)
	return protocol
}

// isKnownProtocol checks if the protocol is one of the known protocols.
func isKnownProtocol(protocol string) bool {
	switch protocol {
	case protocolACP, protocolStreamJSON, "codex", "opencode":
		return true
	default:
		return false
	}
}

// countLines counts the number of lines in a file.
func countLines(path string) int {
	file, err := os.Open(path)
	if err != nil {
		return 0
	}
	defer func() { _ = file.Close() }()

	count := 0
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		count++
	}
	return count
}

// getNormalizerForProtocol returns the appropriate normalizer for the protocol.
func getNormalizerForProtocol(protocol string) normalizer {
	switch protocol {
	case protocolACP:
		return acp.NewNormalizer()
	case protocolStreamJSON:
		return streamjson.NewNormalizer()
	default:
		return nil
	}
}

// normalizeFixtureFile normalizes a specific fixture file.
func normalizeFixtureFile(filePath string) ([]NormalizedFixture, error) {
	protocol := parseProtocolFromFilename(filepath.Base(filePath))
	norm := getNormalizerForProtocol(protocol)
	if norm == nil {
		return nil, fmt.Errorf("unknown protocol: %s", protocol)
	}

	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open %s: %w", filePath, err)
	}
	defer func() { _ = file.Close() }()

	var fixtures []NormalizedFixture
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for scanner.Scan() {
		var input fixtureInput
		if err := json.Unmarshal(scanner.Bytes(), &input); err != nil {
			continue // Skip malformed lines
		}

		fixture := NormalizedFixture{
			Protocol: protocol,
			Input:    input.Input,
		}

		// Extract tool info and normalize based on protocol
		switch protocol {
		case protocolACP:
			args, _ := input.Input["args"].(map[string]any)
			kind, _ := args["kind"].(string)
			fixture.ToolName = kind
			fixture.ToolType = acp.DetectToolOperationType(kind, args)
			fixture.Payload = norm.NormalizeToolCall(kind, args)

		case protocolStreamJSON:
			toolName, _ := input.Input["tool_name"].(string)
			args, _ := input.Input["args"].(map[string]any)
			fixture.ToolName = toolName
			fixture.ToolType = streamjson.DetectStreamJSONToolType(toolName)
			fixture.Payload = norm.NormalizeToolCall(toolName, args)
		}

		fixtures = append(fixtures, fixture)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading %s: %w", filePath, err)
	}

	return fixtures, nil
}
