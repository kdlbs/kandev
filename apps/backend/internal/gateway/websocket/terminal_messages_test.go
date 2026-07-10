package websocket

import (
	"bytes"
	"encoding/json"
	"testing"
)

// TestResizePayloadWireFormat locks in the exact JSON encoding the frontend
// expects. Any change to ResizePayload field tags would break every connected
// terminal client. If this fails after a struct edit, the wire format changed
// and the frontend would need to be updated in lockstep.
func TestResizePayloadWireFormat(t *testing.T) {
	tests := []struct {
		name    string
		payload ResizePayload
		want    string
	}{
		{name: "typical 80x24", payload: ResizePayload{Cols: 80, Rows: 24}, want: `{"cols":80,"rows":24}`},
		{name: "zero", payload: ResizePayload{Cols: 0, Rows: 0}, want: `{"cols":0,"rows":0}`},
		{name: "max uint16", payload: ResizePayload{Cols: 65535, Rows: 65535}, want: `{"cols":65535,"rows":65535}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := json.Marshal(tt.payload)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			if string(got) != tt.want {
				t.Fatalf("encoded = %q, want %q", got, tt.want)
			}

			// Round-trip: server must be able to decode what it (and the
			// frontend) emit. Field names are lowercase JSON tags.
			var decoded ResizePayload
			if err := json.Unmarshal(got, &decoded); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if decoded != tt.payload {
				t.Fatalf("round-trip = %+v, want %+v", decoded, tt.payload)
			}
		})
	}
}

// TestIsResizeCommandWireFormat documents the binary protocol prefix used to
// distinguish resize commands from raw PTY input. Changing either resizeCommandByte
// or the leading-'{' heuristic would silently route resize frames to the PTY (and
// vice versa) for any connected client.
func TestIsResizeCommandWireFormat(t *testing.T) {
	tests := []struct {
		name string
		data []byte
		want bool
	}{
		{name: "resize prefix + json", data: []byte{0x01, '{', '}'}, want: true},
		{name: "resize prefix with realistic payload", data: append([]byte{0x01}, []byte(`{"cols":80,"rows":24}`)...), want: true},
		{name: "bare 0x01 (Ctrl+A)", data: []byte{0x01}, want: false},
		{name: "0x01 followed by non-brace", data: []byte{0x01, 'X'}, want: false},
		{name: "regular input", data: []byte("ls\n"), want: false},
		{name: "empty", data: []byte{}, want: false},
		{name: "json without prefix", data: []byte(`{"cols":80,"rows":24}`), want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isResizeCommand(tt.data); got != tt.want {
				t.Fatalf("isResizeCommand(%q) = %v, want %v", tt.data, got, tt.want)
			}
		})
	}
}

// TestResizeFramePayloadParseable verifies the full wire frame (prefix byte +
// JSON body) round-trips through the handler's parsing path: strip the prefix,
// json.Unmarshal the remainder into ResizePayload.
func TestResizeFramePayloadParseable(t *testing.T) {
	frame := []byte{0x01}
	frame = append(frame, []byte(`{"cols":120,"rows":40}`)...)

	if !isResizeCommand(frame) {
		t.Fatalf("frame not detected as resize: %q", frame)
	}
	if !bytes.HasPrefix(frame, []byte{resizeCommandByte}) {
		t.Fatalf("frame missing resize prefix byte")
	}

	var resize ResizePayload
	if err := json.Unmarshal(frame[1:], &resize); err != nil {
		t.Fatalf("unmarshal frame payload: %v", err)
	}
	if resize.Cols != 120 || resize.Rows != 40 {
		t.Fatalf("parsed = %+v, want cols=120 rows=40", resize)
	}
}
