// Package shared provides common utilities for transport adapters.
package shared

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

// debugMode controls whether agent messages are logged.
// Enable via KANDEV_DEBUG_AGENT_MESSAGES=true environment variable.
var debugMode = os.Getenv("KANDEV_DEBUG_AGENT_MESSAGES") == "true"

// debugLogDir is the directory where debug log files are written.
// Defaults to the process CWD; override with KANDEV_DEBUG_LOG_DIR.
var debugLogDir = resolveDebugLogDir()

// debugLogMu protects concurrent writes to debug log files.
var debugLogMu sync.Mutex

// initializedFiles tracks which log files have been truncated in this session.
// First write to a file truncates it; subsequent writes append.
var initializedFiles = make(map[string]bool)

// Protocol constants for debug file naming
const (
	ProtocolACP        = "acp"
	ProtocolStreamJSON = "streamjson"
	ProtocolCodex      = "codex"
	ProtocolOpenCode   = "opencode"
)

func resolveDebugLogDir() string {
	if dir := os.Getenv("KANDEV_DEBUG_LOG_DIR"); dir != "" {
		return dir
	}
	if cwd, err := os.Getwd(); err == nil {
		return cwd
	}
	return "."
}

// LogRawEvent logs a raw protocol event without transformation.
// File: raw-{protocol}-{agentId}.jsonl
func LogRawEvent(protocol, agentID, eventType string, rawData json.RawMessage) {
	if !debugMode {
		return
	}

	entry := map[string]any{
		"ts":       time.Now().UnixMilli(),
		"protocol": protocol,
		"agent":    agentID,
		"event":    eventType,
		"data":     json.RawMessage(rawData),
	}

	logFile := filepath.Join(debugLogDir, fmt.Sprintf("raw-%s-%s.jsonl", protocol, agentID))
	writeJSONLine(logFile, entry)
}

// LogNormalizedEvent logs a normalized AgentEvent.
// File: normalized-{protocol}-{agentId}.jsonl
func LogNormalizedEvent(protocol, agentID string, event *streams.AgentEvent) {
	if !debugMode {
		return
	}

	entry := map[string]any{
		"ts":    time.Now().UnixMilli(),
		"event": event,
	}

	logFile := filepath.Join(debugLogDir, fmt.Sprintf("normalized-%s-%s.jsonl", protocol, agentID))
	writeJSONLine(logFile, entry)
}

// writeJSONLine writes a JSON entry as a line to the specified file.
// On first write to a file in this session, the file is truncated.
func writeJSONLine(logFile string, entry any) {
	entryJSON, err := json.Marshal(entry)
	if err != nil {
		log.Printf("[DEBUG] Failed to marshal entry: %v", err)
		return
	}

	debugLogMu.Lock()
	defer debugLogMu.Unlock()

	// Determine open flags: truncate on first write, append on subsequent
	flags := os.O_CREATE | os.O_WRONLY
	if initializedFiles[logFile] {
		flags |= os.O_APPEND
	} else {
		flags |= os.O_TRUNC
		initializedFiles[logFile] = true
	}

	f, err := os.OpenFile(logFile, flags, 0644)
	if err != nil {
		log.Printf("[DEBUG] Failed to open log file %s: %v", logFile, err)
		return
	}
	defer func() { _ = f.Close() }()

	_, _ = f.WriteString(string(entryJSON) + "\n")
}
