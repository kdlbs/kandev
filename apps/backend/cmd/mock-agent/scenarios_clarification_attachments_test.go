package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestClarificationAttachmentProofReadsBytes(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "evidence.png")
	body := []byte{0x89, 'P', 'N', 'G', 1, 2, 3, 4}
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	result := `{"db":{"selected_option":"db_opt1","attachments":[{"name":"evidence.png","path":` +
		quoteJSON(path) + `}]}}`
	sum := sha256.Sum256(body)
	want := "evidence.png 8 " + hex.EncodeToString(sum[:])
	if got := clarificationAttachmentProof(result); got != want {
		t.Fatalf("proof = %q, want %q", got, want)
	}
}

func TestClarificationAttachmentProofReportsUnreadableFiles(t *testing.T) {
	result := `{"db":{"attachments":[{"name":"missing.png","path":` +
		quoteJSON(filepath.Join(t.TempDir(), "missing.png")) + `}]}}`
	if got := clarificationAttachmentProof(result); got != "missing.png unreadable" {
		t.Fatalf("proof = %q", got)
	}
}

func TestClarificationAttachmentProofWithoutAttachments(t *testing.T) {
	if got := clarificationAttachmentProof(`{"db":{"selected_option":"db_opt1"}}`); got != "none" {
		t.Fatalf("proof = %q", got)
	}
}

func quoteJSON(value string) string {
	encoded, _ := json.Marshal(value)
	return string(encoded)
}
