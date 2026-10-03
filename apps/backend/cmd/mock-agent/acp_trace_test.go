package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestTraceACPIsOptInAndRecordsRequestIdentity(t *testing.T) {
	path := filepath.Join(t.TempDir(), "acp.jsonl")
	t.Setenv("E2E_MOCK_AGENT_ACP_TRACE_FILE", path)

	traceACP("set_mode", "native-session", map[string]string{"mode_id": "plan-mock"})

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read ACP trace: %v", err)
	}
	var record map[string]string
	if err := json.Unmarshal(data, &record); err != nil {
		t.Fatalf("decode ACP trace: %v", err)
	}
	if record["event"] != "set_mode" || record["session_id"] != "native-session" || record["mode_id"] != "plan-mock" ||
		record["process_id"] == "" || record["connection_id"] == "" {
		t.Fatalf("ACP trace record = %#v", record)
	}
}

func TestTraceACPDoesNothingWithoutOptInPath(t *testing.T) {
	t.Setenv("E2E_MOCK_AGENT_ACP_TRACE_FILE", "")
	traceACP("session_load", "native-session", nil)
}
