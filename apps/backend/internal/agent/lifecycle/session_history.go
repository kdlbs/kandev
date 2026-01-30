// Package lifecycle provides session history management for ACP agents.
// This enables session context injection for agents that don't support session/load.
package lifecycle

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"

	agentctl "github.com/kandev/kandev/internal/agentctl/client"
	"github.com/kandev/kandev/internal/common/logger"
)

// SessionHistoryManager stores and retrieves session history for context injection.
// This enables the fork_session pattern for ACP agents that don't support session/load.
//
// Session history is stored as JSONL files at: {baseDir}/{sessionID}.jsonl
// Each line is a HistoryEntry containing the event type, timestamp, and relevant content.
type SessionHistoryManager struct {
	baseDir string
	logger  *logger.Logger
	mu      sync.RWMutex
}

// HistoryEntry represents a single entry in the session history.
type HistoryEntry struct {
	Timestamp   time.Time              `json:"timestamp"`
	Type        string `json:"type"` // "user_message", "agent_message", "tool_call", "tool_result"
	Role        string `json:"role,omitempty"`
	Content     string `json:"content,omitempty"`
	ToolName    string `json:"tool_name,omitempty"`
	ToolCallID  string `json:"tool_call_id,omitempty"`
	ToolStatus  string `json:"tool_status,omitempty"`
	SessionID   string `json:"session_id,omitempty"`
	OperationID string `json:"operation_id,omitempty"`
}

// NewSessionHistoryManager creates a new SessionHistoryManager.
// The baseDir is the directory where session history files will be stored.
// If baseDir is empty, defaults to ~/.kandev/sessions/
func NewSessionHistoryManager(baseDir string, log *logger.Logger) (*SessionHistoryManager, error) {
	if baseDir == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("failed to get home directory: %w", err)
		}
		baseDir = filepath.Join(homeDir, ".kandev", "sessions")
	}

	// Ensure directory exists
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create session history directory: %w", err)
	}

	return &SessionHistoryManager{
		baseDir: baseDir,
		logger:  log.WithFields(zap.String("component", "session-history")),
	}, nil
}

// historyFilePath returns the path to the history file for a session.
func (m *SessionHistoryManager) historyFilePath(sessionID string) string {
	// Sanitize session ID for use as filename
	safeID := strings.ReplaceAll(sessionID, "/", "_")
	safeID = strings.ReplaceAll(safeID, "\\", "_")
	return filepath.Join(m.baseDir, safeID+".jsonl")
}

// AppendEntry appends a single history entry to the session's history file.
func (m *SessionHistoryManager) AppendEntry(sessionID string, entry HistoryEntry) error {
	if sessionID == "" {
		return fmt.Errorf("session ID is required")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	filePath := m.historyFilePath(sessionID)
	f, err := os.OpenFile(filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open history file: %w", err)
	}
	defer func() { _ = f.Close() }()

	// Set timestamp if not already set
	if entry.Timestamp.IsZero() {
		entry.Timestamp = time.Now().UTC()
	}
	entry.SessionID = sessionID

	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("failed to marshal history entry: %w", err)
	}

	if _, err := f.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("failed to write history entry: %w", err)
	}

	m.logger.Debug("appended history entry",
		zap.String("session_id", sessionID),
		zap.String("type", entry.Type))

	return nil
}

// AppendUserMessage appends a user message to the session history.
func (m *SessionHistoryManager) AppendUserMessage(sessionID, message string) error {
	return m.AppendEntry(sessionID, HistoryEntry{
		Type:    "user_message",
		Role:    "user",
		Content: message,
	})
}

// AppendAgentMessage appends an agent message to the session history.
func (m *SessionHistoryManager) AppendAgentMessage(sessionID, message string) error {
	return m.AppendEntry(sessionID, HistoryEntry{
		Type:    "agent_message",
		Role:    "assistant",
		Content: message,
	})
}

// AppendToolCall appends a tool call to the session history.
func (m *SessionHistoryManager) AppendToolCall(sessionID string, event agentctl.AgentEvent) error {
	return m.AppendEntry(sessionID, HistoryEntry{
		Type:       "tool_call",
		ToolCallID: event.ToolCallID,
		ToolName:   event.ToolName,
		ToolStatus: event.ToolStatus,
	})
}

// AppendToolResult appends a tool result to the session history.
func (m *SessionHistoryManager) AppendToolResult(sessionID string, event agentctl.AgentEvent) error {
	return m.AppendEntry(sessionID, HistoryEntry{
		Type:       "tool_result",
		ToolCallID: event.ToolCallID,
		ToolName:   event.ToolName,
		ToolStatus: event.ToolStatus,
	})
}

// ReadHistory reads all history entries for a session.
func (m *SessionHistoryManager) ReadHistory(sessionID string) ([]HistoryEntry, error) {
	if sessionID == "" {
		return nil, fmt.Errorf("session ID is required")
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	filePath := m.historyFilePath(sessionID)
	f, err := os.Open(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // No history yet
		}
		return nil, fmt.Errorf("failed to open history file: %w", err)
	}
	defer func() { _ = f.Close() }()

	var entries []HistoryEntry
	scanner := bufio.NewScanner(f)
	// Increase buffer size for long lines (tool results can be large)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024) // 1MB max line size

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var entry HistoryEntry
		if err := json.Unmarshal(line, &entry); err != nil {
			m.logger.Warn("failed to parse history entry, skipping",
				zap.String("session_id", sessionID),
				zap.Error(err))
			continue
		}
		entries = append(entries, entry)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed to read history file: %w", err)
	}

	return entries, nil
}

// GenerateResumeContext generates a "RESUME CONTEXT" prompt from session history.
// This is injected into the first prompt when forking a session.
func (m *SessionHistoryManager) GenerateResumeContext(sessionID, newPrompt string) (string, error) {
	entries, err := m.ReadHistory(sessionID)
	if err != nil {
		return newPrompt, err
	}

	if len(entries) == 0 {
		return newPrompt, nil // No history to inject
	}

	// Format history as conversation context
	var historyBuilder strings.Builder
	for _, entry := range entries {
		switch entry.Type {
		case "user_message":
			fmt.Fprintf(&historyBuilder, "\n[USER]: %s\n", truncateForContext(entry.Content, 2000))
		case "agent_message":
			fmt.Fprintf(&historyBuilder, "\n[ASSISTANT]: %s\n", truncateForContext(entry.Content, 2000))
		case "tool_call":
			fmt.Fprintf(&historyBuilder, "\n[TOOL CALL: %s]\n", entry.ToolName)
		case "tool_result":
			fmt.Fprintf(&historyBuilder, "\n[TOOL RESULT: %s] %s\n", entry.ToolName, truncateForContext(entry.Content, 500))
		}
	}

	history := historyBuilder.String()
	if history == "" {
		return newPrompt, nil
	}

	// Build the resume context prompt
	resumePrompt := fmt.Sprintf(`RESUME CONTEXT FOR CONTINUING TASK

=== EXECUTION HISTORY ===
The following is a summary of the previous conversation in this session:
%s

=== CURRENT REQUEST ===
%s

=== INSTRUCTIONS ===
You are continuing work on the above task. This is a continuation of a previous session.
Please continue from where the previous execution left off, taking into account all the context provided above.
Do not repeat work that was already completed. Build on the existing progress.
`, history, newPrompt)

	m.logger.Info("generated resume context",
		zap.String("session_id", sessionID),
		zap.Int("history_entries", len(entries)),
		zap.Int("context_length", len(resumePrompt)))

	return resumePrompt, nil
}

// HasHistory checks if a session has any stored history.
func (m *SessionHistoryManager) HasHistory(sessionID string) bool {
	if sessionID == "" {
		return false
	}

	filePath := m.historyFilePath(sessionID)
	info, err := os.Stat(filePath)
	if err != nil {
		return false
	}
	return info.Size() > 0
}

// DeleteHistory deletes the history file for a session.
func (m *SessionHistoryManager) DeleteHistory(sessionID string) error {
	if sessionID == "" {
		return fmt.Errorf("session ID is required")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	filePath := m.historyFilePath(sessionID)
	if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete history file: %w", err)
	}

	m.logger.Debug("deleted session history", zap.String("session_id", sessionID))
	return nil
}

// truncateForContext truncates a string to a maximum length for context injection.
// This prevents the resume context from becoming too large.
func truncateForContext(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "... [truncated]"
}
