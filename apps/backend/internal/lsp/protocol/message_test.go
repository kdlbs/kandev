package protocol

import (
	"bufio"
	"fmt"
	"strings"
	"testing"
)

const (
	testMaxHeaderLineBytes = 8 << 10
	testMaxHeaderBytes     = 32 << 10
	testMaxBodyBytes       = MaxMessageBytes
)

func TestReadMessageValid(t *testing.T) {
	body := `{"jsonrpc":"2.0","id":1,"method":"initialize"}`
	raw := fmt.Sprintf("Content-Length: %d\r\n\r\n%s", len(body), body)

	message, err := ReadMessage(bufio.NewReader(strings.NewReader(raw)))
	if err != nil {
		t.Fatalf("ReadMessage() error = %v", err)
	}
	if string(message) != body {
		t.Fatalf("ReadMessage() = %q, want %q", message, body)
	}
}

func TestReadMessageRejectsOversizedBodyBeforeReadingIt(t *testing.T) {
	raw := fmt.Sprintf("Content-Length: %d\r\n\r\n", testMaxBodyBytes+1)

	_, err := ReadMessage(bufio.NewReader(strings.NewReader(raw)))
	if err == nil {
		t.Fatal("ReadMessage() error = nil, want oversized body error")
	}
	if !strings.Contains(err.Error(), "Content-Length exceeds") {
		t.Fatalf("ReadMessage() error = %q, want Content-Length limit error", err)
	}
}

func TestReadMessageRejectsOversizedHeaderLine(t *testing.T) {
	raw := "X-Test: " + strings.Repeat("a", testMaxHeaderLineBytes) +
		"\r\nContent-Length: 2\r\n\r\n{}"

	_, err := ReadMessage(bufio.NewReader(strings.NewReader(raw)))
	if err == nil {
		t.Fatal("ReadMessage() error = nil, want oversized header line error")
	}
	if !strings.Contains(err.Error(), "header line exceeds") {
		t.Fatalf("ReadMessage() error = %q, want header line limit error", err)
	}
}

func TestReadMessageRejectsOversizedAggregateHeaders(t *testing.T) {
	var raw strings.Builder
	raw.WriteString("Content-Length: 2\r\n")
	for raw.Len() <= testMaxHeaderBytes {
		raw.WriteString("X-Test: a\r\n")
	}
	raw.WriteString("\r\n{}")

	_, err := ReadMessage(bufio.NewReader(strings.NewReader(raw.String())))
	if err == nil {
		t.Fatal("ReadMessage() error = nil, want oversized aggregate headers error")
	}
	if !strings.Contains(err.Error(), "headers exceed") {
		t.Fatalf("ReadMessage() error = %q, want aggregate header limit error", err)
	}
}

func TestReadMessageRejectsInvalidContentLength(t *testing.T) {
	tests := []struct {
		name  string
		value string
	}{
		{name: "malformed", value: "abc"},
		{name: "negative", value: "-1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw := "Content-Length: " + tt.value + "\r\n\r\n"

			_, err := ReadMessage(bufio.NewReader(strings.NewReader(raw)))
			if err == nil {
				t.Fatal("ReadMessage() error = nil, want invalid Content-Length error")
			}
			if !strings.Contains(err.Error(), "invalid Content-Length") {
				t.Fatalf("ReadMessage() error = %q, want invalid Content-Length error", err)
			}
		})
	}
}
