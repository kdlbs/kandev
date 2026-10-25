// Package shared provides common utilities for transport adapters.
package shared

import (
	"encoding/json"
	"os"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

// debugMode controls whether agent messages are logged.
// Enable via KANDEV_DEBUG_AGENT_MESSAGES=true environment variable.
var debugMode = os.Getenv(envDebugMessages) == "true"

// acpLog is the process-wide managed writer registry. It is allocated even
// when debug mode is off (cheap, no file handles) so the janitor and tail
// endpoint can reference a stable instance; nothing is written unless
// debugMode is true.
var acpLog = newACPLogManager(acpLogConfigFromEnv())

// ProtocolACP names the ACP transport in debug file naming (raw-acp-*.jsonl,
// normalized-acp-*.jsonl). It is the only supported protocol.
const ProtocolACP = "acp"

// ACPDebugEnabled reports whether agent-message debug logging is on.
func ACPDebugEnabled() bool { return debugMode }

// ACPLogDir returns the directory debug log files are written to. Exported so
// the debug reader (internal/debug) can discover per-session files. It
// resolves live from the environment: the reader runs in the backend process
// while the writer runs in agentctl, but both start from the same forwarded
// env so they resolve to the same directory.
func ACPLogDir() string { return resolveACPLogDir() }

// FlushACPLogs makes retained raw/normalized files complete before an
// explicit diagnostic bundle request reads them. It is intentionally not used
// on the frame hot path.
func FlushACPLogs() {
	if !debugMode {
		return
	}
	acpLog.flushAll()
}

// ACPRingTail returns up to n most recent normalized events for a session as
// raw JSON lines, for the dev-only live-tail endpoint. Returns nil when the
// session is unknown.
func ACPRingTail(sessionID string, n int) []json.RawMessage {
	return acpLog.ringTail(sessionID, n)
}

// LogRawEvent logs a raw protocol event without transformation.
// File: raw-{protocol}-{agentID}-{sessionID}.jsonl
func LogRawEvent(protocol, agentID, sessionID, eventType string, rawData json.RawMessage) {
	if !debugMode {
		return
	}
	acpLog.writeRaw(protocol, agentID, sessionID, eventType, rawData)
}

// LogNormalizedEvent logs a normalized AgentEvent.
// File: normalized-{protocol}-{agentID}-{sessionID}.jsonl
func LogNormalizedEvent(protocol, agentID, sessionID string, event *streams.AgentEvent) {
	if !debugMode {
		return
	}
	acpLog.writeNormalized(protocol, agentID, sessionID, event)
}
