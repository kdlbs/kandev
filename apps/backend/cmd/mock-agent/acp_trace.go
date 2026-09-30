package main

import (
	"encoding/json"
	"os"
	"sync"
)

var acpTraceMu sync.Mutex

// traceACP writes mock ACP requests only when an E2E test explicitly provides
// a trace path. The file gives browser tests evidence of the peer's observed
// native session identity and selector operations.
func traceACP(event, sessionID string, fields map[string]string) {
	tracePath := os.Getenv("E2E_MOCK_AGENT_ACP_TRACE_FILE")
	if tracePath == "" {
		return
	}
	record := map[string]string{"event": event, "session_id": sessionID}
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
