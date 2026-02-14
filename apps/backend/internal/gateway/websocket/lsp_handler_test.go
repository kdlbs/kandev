package websocket

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/lsp/installer"
)

func TestReadLSPMessage_Valid(t *testing.T) {
	body := `{"jsonrpc":"2.0","id":1,"method":"initialize"}`
	raw := fmt.Sprintf("Content-Length: %d\r\n\r\n%s", len(body), body)
	reader := bufio.NewReader(strings.NewReader(raw))

	msg, err := readLSPMessage(reader)
	if err != nil {
		t.Fatalf("readLSPMessage() error = %v", err)
	}
	if string(msg) != body {
		t.Errorf("readLSPMessage() = %q, want %q", string(msg), body)
	}
}

func TestReadLSPMessage_WithExtraHeaders(t *testing.T) {
	body := `{"jsonrpc":"2.0","id":2}`
	raw := fmt.Sprintf("Content-Length: %d\r\nContent-Type: application/vscode-jsonrpc; charset=utf-8\r\n\r\n%s", len(body), body)
	reader := bufio.NewReader(strings.NewReader(raw))

	msg, err := readLSPMessage(reader)
	if err != nil {
		t.Fatalf("readLSPMessage() error = %v", err)
	}
	if string(msg) != body {
		t.Errorf("readLSPMessage() = %q, want %q", string(msg), body)
	}
}

func TestReadLSPMessage_MissingContentLength(t *testing.T) {
	raw := "Content-Type: application/json\r\n\r\n{}"
	reader := bufio.NewReader(strings.NewReader(raw))

	_, err := readLSPMessage(reader)
	if err == nil {
		t.Fatal("readLSPMessage() should return error for missing Content-Length")
	}
	if !strings.Contains(err.Error(), "missing Content-Length") {
		t.Errorf("readLSPMessage() error = %q, want 'missing Content-Length'", err.Error())
	}
}

func TestReadLSPMessage_InvalidContentLength(t *testing.T) {
	raw := "Content-Length: abc\r\n\r\n{}"
	reader := bufio.NewReader(strings.NewReader(raw))

	_, err := readLSPMessage(reader)
	if err == nil {
		t.Fatal("readLSPMessage() should return error for invalid Content-Length")
	}
	if !strings.Contains(err.Error(), "invalid Content-Length") {
		t.Errorf("readLSPMessage() error = %q, want 'invalid Content-Length'", err.Error())
	}
}

func TestReadLSPMessage_EOF(t *testing.T) {
	reader := bufio.NewReader(bytes.NewReader(nil))

	_, err := readLSPMessage(reader)
	if err == nil {
		t.Fatal("readLSPMessage() should return error on EOF")
	}
	if err != io.EOF {
		t.Errorf("readLSPMessage() error = %v, want io.EOF", err)
	}
}

func TestReadLSPMessage_MultipleMessages(t *testing.T) {
	body1 := `{"jsonrpc":"2.0","id":1}`
	body2 := `{"jsonrpc":"2.0","id":2}`
	raw := fmt.Sprintf("Content-Length: %d\r\n\r\n%sContent-Length: %d\r\n\r\n%s",
		len(body1), body1, len(body2), body2)
	reader := bufio.NewReader(strings.NewReader(raw))

	msg1, err := readLSPMessage(reader)
	if err != nil {
		t.Fatalf("first readLSPMessage() error = %v", err)
	}
	if string(msg1) != body1 {
		t.Errorf("first readLSPMessage() = %q, want %q", string(msg1), body1)
	}

	msg2, err := readLSPMessage(reader)
	if err != nil {
		t.Fatalf("second readLSPMessage() error = %v", err)
	}
	if string(msg2) != body2 {
		t.Errorf("second readLSPMessage() = %q, want %q", string(msg2), body2)
	}
}

func TestLspCommand_ViaRegistry(t *testing.T) {
	tests := []struct {
		language   string
		wantBinary string
		wantArgs   []string
	}{
		{"typescript", "typescript-language-server", []string{"--stdio"}},
		{"go", "gopls", []string{"serve"}},
		{"rust", "rust-analyzer", nil},
		{"python", "pyright-langserver", []string{"--stdio"}},
		{"unknown", "", nil},
	}
	for _, tc := range tests {
		binary, args := installer.LspCommand(tc.language)
		if binary != tc.wantBinary {
			t.Errorf("LspCommand(%q) binary = %q, want %q", tc.language, binary, tc.wantBinary)
		}
		if len(args) != len(tc.wantArgs) {
			t.Errorf("LspCommand(%q) args = %v, want %v", tc.language, args, tc.wantArgs)
			continue
		}
		for i := range args {
			if args[i] != tc.wantArgs[i] {
				t.Errorf("LspCommand(%q) args[%d] = %q, want %q", tc.language, i, args[i], tc.wantArgs[i])
			}
		}
	}
}

func TestIsValidLSPLanguage_ViaRegistry(t *testing.T) {
	tests := []struct {
		language string
		want     bool
	}{
		{"typescript", true},
		{"go", true},
		{"rust", true},
		{"python", true},
		{"java", false},
		{"", false},
		{"ruby", false},
	}
	for _, tc := range tests {
		if got := installer.IsSupported(tc.language); got != tc.want {
			t.Errorf("IsSupported(%q) = %v, want %v", tc.language, got, tc.want)
		}
	}
}

func TestCloseCodeConstants(t *testing.T) {
	// Verify close codes are in the expected range (4000-4999 for application-specific)
	codes := []struct {
		name string
		code int
	}{
		{"lspCloseBinaryNotFound", lspCloseBinaryNotFound},
		{"lspCloseSessionNotFound", lspCloseSessionNotFound},
		{"lspCloseInstallFailed", lspCloseInstallFailed},
	}
	for _, tc := range codes {
		if tc.code < 4000 || tc.code > 4999 {
			t.Errorf("%s = %d, want value in range 4000-4999", tc.name, tc.code)
		}
	}
	// Verify they're distinct
	seen := make(map[int]string)
	for _, tc := range codes {
		if prev, ok := seen[tc.code]; ok {
			t.Errorf("%s and %s have the same code %d", prev, tc.name, tc.code)
		}
		seen[tc.code] = tc.name
	}
}
