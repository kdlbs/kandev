package sqlite

import (
	"context"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

// shellOutputMetadata builds the metadata shape ProjectMessageMetadata/
// ExtractShellExecOutput/RehydrateShellOutput all understand: a
// normalized.shell_exec.output nested map holding stdout/stderr.
func shellOutputMetadata(stdout, stderr string) map[string]interface{} {
	return map[string]interface{}{
		"tool_call_id": "call-1",
		"normalized": map[string]interface{}{
			"shell_exec": map[string]interface{}{
				"command": "echo",
				"output": map[string]interface{}{
					"stdout":    stdout,
					"stderr":    stderr,
					"truncated": false,
				},
			},
		},
	}
}

func newShellMessage(id, sessionID string, stdout, stderr string) *models.Message {
	return &models.Message{
		ID:            id,
		TaskSessionID: sessionID,
		AuthorType:    models.MessageAuthorAgent,
		Type:          models.MessageTypeToolCall,
		Metadata:      shellOutputMetadata(stdout, stderr),
	}
}

// TestCreateMessageLeavesSmallMetadataInline confirms the common case (small
// tool output) is untouched: no externalization, no digest, other metadata
// keys preserved verbatim (message_clarification_response.go and friends
// read several of them via direct SQL json_extract).
func TestCreateMessageLeavesSmallMetadataInline(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedForMsgTest(t, repo, "task-payload-small", "sess-payload-small", "turn-1")

	msg := newShellMessage("msg-small", "sess-payload-small", "ok", "")
	msg.TurnID = "turn-1"
	if err := repo.CreateMessage(ctx, msg); err != nil {
		t.Fatalf("CreateMessage: %v", err)
	}
	if msg.PayloadDigest != "" {
		t.Fatalf("PayloadDigest = %q, want empty for small output", msg.PayloadDigest)
	}

	got, err := repo.GetMessage(ctx, "msg-small")
	if err != nil {
		t.Fatalf("GetMessage: %v", err)
	}
	if got.PayloadDigest != "" {
		t.Fatalf("persisted PayloadDigest = %q, want empty", got.PayloadDigest)
	}
	output, ok := models.ExtractShellExecOutput(got.Metadata)
	if !ok || output.Stdout != "ok" {
		t.Fatalf("ExtractShellExecOutput = (%+v, %v), want stdout=ok", output, ok)
	}
	if got.Metadata["tool_call_id"] != "call-1" {
		t.Fatalf("tool_call_id metadata key was lost: %+v", got.Metadata)
	}
}

// TestCreateMessageExternalizesLargeShellOutputAndRehydrates covers the
// write -> project -> lazy-load -> verify round trip end to end.
func TestCreateMessageExternalizesLargeShellOutputAndRehydrates(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedForMsgTest(t, repo, "task-payload-large", "sess-payload-large", "turn-1")

	largeStdout := strings.Repeat("x", largeMessagePayloadThresholdBytes+1)
	msg := newShellMessage("msg-large", "sess-payload-large", largeStdout, "")
	msg.TurnID = "turn-1"
	if err := repo.CreateMessage(ctx, msg); err != nil {
		t.Fatalf("CreateMessage: %v", err)
	}
	if msg.PayloadDigest == "" {
		t.Fatal("PayloadDigest was not set for output over the externalization threshold")
	}

	// GetMessage (the lazy-detail read path) must return the small projected
	// summary, not the full stdout, until explicitly rehydrated.
	got, err := repo.GetMessage(ctx, "msg-large")
	if err != nil {
		t.Fatalf("GetMessage: %v", err)
	}
	if got.PayloadDigest != msg.PayloadDigest {
		t.Fatalf("persisted PayloadDigest = %q, want %q", got.PayloadDigest, msg.PayloadDigest)
	}
	if _, ok := models.ExtractShellExecOutput(got.Metadata); !ok {
		t.Fatal("ExtractShellExecOutput reported ok=false for projected metadata; want ok=true with an empty body")
	}
	if output, _ := models.ExtractShellExecOutput(got.Metadata); output.Stdout != "" {
		t.Fatalf("GetMessage returned a non-empty stdout body (%d bytes) before rehydration; expected only the projected summary", len(output.Stdout))
	}
	normalized, _ := got.Metadata["normalized"].(map[string]interface{})
	shellExec, _ := normalized["shell_exec"].(map[string]interface{})
	if shellExec == nil || shellExec["output"] == nil {
		t.Fatalf("projected metadata missing shell_exec output summary: %+v", got.Metadata)
	}
	if summary, ok := shellExec["output"].(map[string]interface{}); ok {
		if _, hasStdout := summary["stdout"]; hasStdout {
			t.Fatal("projected summary must not include the raw stdout body")
		}
	}

	// Explicit authorized rehydration restores the full output.
	if err := repo.RehydrateMessagePayload(ctx, got); err != nil {
		t.Fatalf("RehydrateMessagePayload: %v", err)
	}
	output, ok := models.ExtractShellExecOutput(got.Metadata)
	if !ok || output.Stdout != largeStdout {
		t.Fatalf("rehydrated stdout length = %d, want %d (ok=%v)", len(output.Stdout), len(largeStdout), ok)
	}
}

// TestExternalizeMessagePayloadDedupesIdenticalContentAcrossMessages proves
// the content-addressed store never writes the same payload bytes twice:
// two different messages with byte-identical large output share one
// task_message_payloads row.
func TestExternalizeMessagePayloadDedupesIdenticalContentAcrossMessages(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedForMsgTest(t, repo, "task-payload-dedup", "sess-payload-dedup", "turn-1")

	largeStdout := strings.Repeat("y", largeMessagePayloadThresholdBytes+1)
	first := newShellMessage("msg-dedup-1", "sess-payload-dedup", largeStdout, "")
	first.TurnID = "turn-1"
	second := newShellMessage("msg-dedup-2", "sess-payload-dedup", largeStdout, "")
	second.TurnID = "turn-1"

	if err := repo.CreateMessage(ctx, first); err != nil {
		t.Fatalf("CreateMessage(first): %v", err)
	}
	if err := repo.CreateMessage(ctx, second); err != nil {
		t.Fatalf("CreateMessage(second): %v", err)
	}

	if first.PayloadDigest == "" || first.PayloadDigest != second.PayloadDigest {
		t.Fatalf("expected identical payload digests, got %q and %q", first.PayloadDigest, second.PayloadDigest)
	}

	count := countRows(t, repo, `SELECT COUNT(*) FROM task_message_payloads WHERE digest = ?`, first.PayloadDigest)
	if count != 1 {
		t.Fatalf("task_message_payloads rows for shared digest = %d, want 1 (content-addressed dedup)", count)
	}
}

// TestRehydrateMessagePayloadRejectsTamperedContent proves the integrity
// check is enforced: if the stored bytes no longer match the message's
// recorded digest (corruption, or a hostile write to the payload table), the
// rehydration must fail rather than silently return altered content.
func TestRehydrateMessagePayloadRejectsTamperedContent(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedForMsgTest(t, repo, "task-payload-tamper", "sess-payload-tamper", "turn-1")

	largeStdout := strings.Repeat("z", largeMessagePayloadThresholdBytes+1)
	msg := newShellMessage("msg-tamper", "sess-payload-tamper", largeStdout, "")
	msg.TurnID = "turn-1"
	if err := repo.CreateMessage(ctx, msg); err != nil {
		t.Fatalf("CreateMessage: %v", err)
	}

	tampered, err := gzipCompress([]byte(`{"stdout":"not the original content"}`))
	if err != nil {
		t.Fatalf("gzipCompress: %v", err)
	}
	if _, err := repo.db.ExecContext(ctx, repo.db.Rebind(
		`UPDATE task_message_payloads SET compressed_content = ? WHERE digest = ?`),
		tampered, msg.PayloadDigest); err != nil {
		t.Fatalf("tamper with stored payload: %v", err)
	}

	got, err := repo.GetMessage(ctx, "msg-tamper")
	if err != nil {
		t.Fatalf("GetMessage: %v", err)
	}
	if err := repo.RehydrateMessagePayload(ctx, got); err == nil {
		t.Fatal("RehydrateMessagePayload accepted tampered content; want an integrity-check error")
	}
}

func TestRehydrateMessagePayloadRejectsOversizedStoredPayload(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedForMsgTest(t, repo, "task-payload-oversized", "sess-payload-oversized", "turn-1")

	largeStdout := strings.Repeat("z", largeMessagePayloadThresholdBytes+1)
	msg := newShellMessage("msg-oversized", "sess-payload-oversized", largeStdout, "")
	msg.TurnID = "turn-1"
	if err := repo.CreateMessage(ctx, msg); err != nil {
		t.Fatalf("CreateMessage: %v", err)
	}

	replacementPayload := []byte(`{"stdout":"small but metadata claims it is too large"}`)
	oversizedCompressed, err := gzipCompress(replacementPayload)
	if err != nil {
		t.Fatalf("gzipCompress: %v", err)
	}
	oversizedDigest := sha256Hex(replacementPayload)
	if _, err := repo.db.ExecContext(ctx, repo.db.Rebind(
		`UPDATE task_message_payloads
		 SET digest = ?, compressed_content = ?, uncompressed_size = ?, compressed_size = ?
		 WHERE digest = ?`),
		oversizedDigest, oversizedCompressed, maxMessagePayloadRehydrateBytes+1, len(oversizedCompressed), msg.PayloadDigest); err != nil {
		t.Fatalf("replace stored payload: %v", err)
	}
	if _, err := repo.db.ExecContext(ctx, repo.db.Rebind(
		`UPDATE task_session_messages SET payload_digest = ?, payload_size = ? WHERE id = ?`),
		oversizedDigest, maxMessagePayloadRehydrateBytes+1, msg.ID); err != nil {
		t.Fatalf("point message at oversized payload: %v", err)
	}

	got, err := repo.GetMessage(ctx, "msg-oversized")
	if err != nil {
		t.Fatalf("GetMessage: %v", err)
	}
	if err := repo.RehydrateMessagePayload(ctx, got); err == nil {
		t.Fatal("RehydrateMessagePayload accepted an oversized payload; want an error")
	}
}

func TestRehydrateMessagePayloadRejectsMismatchedStoredPayloadSize(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedForMsgTest(t, repo, "task-payload-size-mismatch", "sess-payload-size-mismatch", "turn-1")

	largeStdout := strings.Repeat("s", largeMessagePayloadThresholdBytes+1)
	msg := newShellMessage("msg-size-mismatch", "sess-payload-size-mismatch", largeStdout, "")
	msg.TurnID = "turn-1"
	if err := repo.CreateMessage(ctx, msg); err != nil {
		t.Fatalf("CreateMessage: %v", err)
	}

	if _, err := repo.db.ExecContext(ctx, repo.db.Rebind(
		`UPDATE task_message_payloads SET uncompressed_size = ? WHERE digest = ?`),
		len(largeStdout)-1, msg.PayloadDigest); err != nil {
		t.Fatalf("corrupt stored payload size: %v", err)
	}

	got, err := repo.GetMessage(ctx, "msg-size-mismatch")
	if err != nil {
		t.Fatalf("GetMessage: %v", err)
	}
	if err := repo.RehydrateMessagePayload(ctx, got); err == nil {
		t.Fatal("RehydrateMessagePayload accepted mismatched payload size metadata")
	}
}

func TestRehydrateMessagePayloadRejectsMismatchedCompressedPayloadSize(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedForMsgTest(t, repo, "task-payload-compressed-size-mismatch", "sess-payload-compressed-size-mismatch", "turn-1")

	largeStdout := strings.Repeat("c", largeMessagePayloadThresholdBytes+1)
	msg := newShellMessage("msg-compressed-size-mismatch", "sess-payload-compressed-size-mismatch", largeStdout, "")
	msg.TurnID = "turn-1"
	if err := repo.CreateMessage(ctx, msg); err != nil {
		t.Fatalf("CreateMessage: %v", err)
	}

	if _, err := repo.db.ExecContext(ctx, repo.db.Rebind(
		`UPDATE task_message_payloads SET compressed_size = ? WHERE digest = ?`),
		1, msg.PayloadDigest); err != nil {
		t.Fatalf("corrupt stored compressed payload size: %v", err)
	}

	got, err := repo.GetMessage(ctx, "msg-compressed-size-mismatch")
	if err != nil {
		t.Fatalf("GetMessage: %v", err)
	}
	if err := repo.RehydrateMessagePayload(ctx, got); err == nil {
		t.Fatal("RehydrateMessagePayload accepted mismatched compressed payload size metadata")
	}
}

func TestGzipDecompressRejectsActualOversize(t *testing.T) {
	compressed, err := gzipCompress([]byte("123456789"))
	if err != nil {
		t.Fatalf("gzipCompress: %v", err)
	}
	if _, err := gzipDecompress(compressed, 8); err == nil {
		t.Fatal("gzipDecompress accepted output beyond the configured cap")
	}
}

func TestGzipDecompressRejectsExpectedOversize(t *testing.T) {
	compressed, err := gzipCompress([]byte("123456789"))
	if err != nil {
		t.Fatalf("gzipCompress: %v", err)
	}
	if _, err := gzipDecompressExpectedSize(compressed, 8, 9); err == nil {
		t.Fatal("gzipDecompressExpectedSize accepted an expected size above the configured cap")
	}
}
