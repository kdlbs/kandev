package main

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	acp "github.com/coder/acp-go-sdk"
)

func TestInitializeCanFailBeforeSessionCreationForE2E(t *testing.T) {
	t.Setenv("E2E_MOCK_AGENT_FAIL_INITIALIZE", "true")
	agent := &mockAgent{}
	response, err := agent.Initialize(context.Background(), acp.InitializeRequest{})
	if err == nil {
		t.Fatal("Initialize error = nil, want deterministic fixture failure")
	}
	if response.ProtocolVersion != 0 {
		t.Fatalf("Initialize response = %#v, want empty response on fixture failure", response)
	}
}

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

func TestSummarizeACPPromptBlocksFingerprintsObservedPayloads(t *testing.T) {
	image := []byte{0xff, 0x00, 0x01, 0x80}
	pathText := "The user attached a writable file: report.txt (saved to .kandev/attachments/session/report.txt)."
	blocks := []acp.ContentBlock{
		acp.ImageBlock(base64.StdEncoding.EncodeToString(image), "image/png"),
		acp.TextBlock(pathText),
	}

	got := summarizeACPPromptBlocks(blocks)
	imageDigest := sha256.Sum256(image)
	textDigest := sha256.Sum256([]byte(pathText))
	if len(got) != 2 || got[0].Type != "image" || got[0].MimeType != "image/png" ||
		got[0].ByteLength != len(image) || got[0].SHA256 != fmt.Sprintf("%x", imageDigest) {
		t.Fatalf("image prompt trace = %#v", got)
	}
	if got[1].Type != "text" || got[1].ByteLength != len(pathText) ||
		got[1].SHA256 != fmt.Sprintf("%x", textDigest) {
		t.Fatalf("resource path prompt trace = %#v", got[1])
	}
	encoded, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("encode prompt trace: %v", err)
	}
	if string(encoded) == "" || string(encoded) == base64.StdEncoding.EncodeToString(image) {
		t.Fatalf("prompt trace must not contain raw attachment bytes: %s", encoded)
	}
}
