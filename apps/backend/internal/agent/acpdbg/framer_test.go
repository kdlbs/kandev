package acpdbg

import (
	"bytes"
	"io"
	"strings"
	"testing"
)

func TestFramer_WriteAppendsNewline(t *testing.T) {
	var out bytes.Buffer
	f := NewFramer(&out, strings.NewReader(""))
	if err := f.Write(Frame{"jsonrpc": "2.0", "id": 1, "method": "initialize"}); err != nil {
		t.Fatalf("write: %v", err)
	}
	got := out.String()
	if !strings.HasSuffix(got, "\n") {
		t.Errorf("expected trailing newline, got %q", got)
	}
	if !strings.Contains(got, `"method":"initialize"`) {
		t.Errorf("expected method field in %q", got)
	}
}

func TestFramer_ReadParsesMultipleFrames(t *testing.T) {
	input := `{"jsonrpc":"2.0","id":1,"result":{"protocolVersion":1}}
{"jsonrpc":"2.0","method":"session/update","params":{"sessionId":"abc"}}
{"jsonrpc":"2.0","id":2,"error":{"code":-32601,"message":"nope"}}
`
	f := NewFramer(io.Discard, strings.NewReader(input))

	first, err := f.Read()
	if err != nil {
		t.Fatalf("read 1: %v", err)
	}
	if !first.IsResponse() || first.ID().(float64) != 1 {
		t.Errorf("first frame should be response id=1, got %+v", first)
	}

	second, err := f.Read()
	if err != nil {
		t.Fatalf("read 2: %v", err)
	}
	if second.Method() != "session/update" || second.ID() != nil {
		t.Errorf("second frame should be notification, got %+v", second)
	}

	third, err := f.Read()
	if err != nil {
		t.Fatalf("read 3: %v", err)
	}
	if !third.IsResponse() || third.ID().(float64) != 2 {
		t.Errorf("third frame should be error response id=2, got %+v", third)
	}

	if _, err := f.Read(); err != io.EOF {
		t.Errorf("expected io.EOF after last frame, got %v", err)
	}
}

func TestFramer_ReadSkipsBlankLines(t *testing.T) {
	input := "\n\n{\"id\":1,\"result\":{}}\n\n{\"id\":2,\"result\":{}}\n"
	f := NewFramer(io.Discard, strings.NewReader(input))

	first, err := f.Read()
	if err != nil {
		t.Fatalf("read 1: %v", err)
	}
	if first.ID().(float64) != 1 {
		t.Errorf("first id = %v, want 1", first.ID())
	}
	second, err := f.Read()
	if err != nil {
		t.Fatalf("read 2: %v", err)
	}
	if second.ID().(float64) != 2 {
		t.Errorf("second id = %v, want 2", second.ID())
	}
}

func TestFramer_NextIDMonotonic(t *testing.T) {
	f := NewFramer(io.Discard, strings.NewReader(""))
	if id := f.NextID(); id != 1 {
		t.Errorf("first id = %d, want 1", id)
	}
	if id := f.NextID(); id != 2 {
		t.Errorf("second id = %d, want 2", id)
	}
	req, id := f.NewRequest("initialize", map[string]any{"protocolVersion": 1})
	if id != 3 {
		t.Errorf("NewRequest id = %d, want 3", id)
	}
	if req["method"] != "initialize" || req["jsonrpc"] != "2.0" {
		t.Errorf("bad request frame: %+v", req)
	}
}

func TestFramer_ReadLargeFrame(t *testing.T) {
	// A realistic session/new response can be ~50 KB when models + modes are
	// populated. Ensure the scanner buffer handles it.
	big := strings.Repeat("x", 200_000)
	input := `{"id":1,"result":{"sessionId":"` + big + `"}}` + "\n"
	f := NewFramer(io.Discard, strings.NewReader(input))
	got, err := f.Read()
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !got.IsResponse() {
		t.Errorf("expected response, got %+v", got)
	}
}

func TestFramer_ReadBadJSONReturnsError(t *testing.T) {
	f := NewFramer(io.Discard, strings.NewReader("this is not json\n"))
	if _, err := f.Read(); err == nil {
		t.Error("expected parse error on malformed input")
	}
}

func TestNewMethodNotFound(t *testing.T) {
	fr := NewMethodNotFound(42, "fs/read_text_file")
	if fr["id"] != 42 {
		t.Errorf("id = %v, want 42", fr["id"])
	}
	errMap, ok := fr["error"].(map[string]any)
	if !ok {
		t.Fatalf("error field missing: %+v", fr)
	}
	if errMap["code"] != -32601 {
		t.Errorf("code = %v, want -32601", errMap["code"])
	}
}
