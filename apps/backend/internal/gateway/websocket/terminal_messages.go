package websocket

import "regexp"

// Terminal WebSocket wire protocol
// ---------------------------------
// Frames are binary WebSocket messages. The first byte distinguishes the
// frame kind:
//
//   - 0x01 + '{' ... '}'  → JSON resize command (ResizePayload)
//   - anything else       → raw PTY input/output bytes
//
// Output to the client is always raw PTY bytes — never wrapped. Wrapping
// PTY data in a JSON envelope would require base64 and break the xterm.js
// AttachAddon, so the protocol stays binary on purpose.

// resizeCommandByte is the binary protocol marker for resize messages.
// First byte 0x01 indicates resize, followed by JSON {cols, rows}.
const resizeCommandByte = 0x01

// isResizeCommand checks whether a binary frame is a resize command.
// Resize messages use 0x01 as a prefix followed by a JSON object (starts with '{').
// A bare 0x01 or 0x01 followed by non-JSON data is regular PTY input (e.g. Ctrl+A).
func isResizeCommand(data []byte) bool {
	return len(data) >= 2 && data[0] == resizeCommandByte && data[1] == '{'
}

// ResizePayload is the JSON payload for resize commands.
type ResizePayload struct {
	Cols uint16 `json:"cols"`
	Rows uint16 `json:"rows"`
}

// terminalResponsePattern matches terminal query responses that should not be replayed.
// These are responses from the terminal emulator to queries sent by the shell, which
// appear as PTY output and get captured in the ring buffer. On replay they render as
// visible garbage text because they are response sequences, not display sequences.
//
// Matched sequences:
//   - OSC 11 response: \x1b]11;rgb:XXXX/XXXX/XXXX ST (background color, ST = ESC\ or BEL)
//   - DA1 response: \x1b[?<params>c (device attributes)
//   - CPR response: \x1b[<row>;<col>R or \x1b[<row>R (cursor position report)
var terminalResponsePattern = regexp.MustCompile(
	`\x1b]11;rgb:[0-9a-fA-F/]+(?:\x1b\\|\x07)` + // OSC 11 (ESC\ or BEL terminator)
		`|\x1b\[\?[0-9;]*c` + // DA1 response
		`|\x1b\[\d+(?:;\d+)?R`, // CPR response
)

// stripTerminalResponses removes terminal query response sequences from data.
// Used to clean buffered PTY output before replaying it on reconnect.
func stripTerminalResponses(data []byte) []byte {
	return terminalResponsePattern.ReplaceAll(data, nil)
}
