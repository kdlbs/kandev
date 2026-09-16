package models

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// @covers AC-SYSTEM-PAGE-TOOL-PAYLOAD-RETENTION-002.3
func TestReduceToolPayloadFieldMatrix(t *testing.T) {
	for _, tc := range []struct{ kind, typ, body string }{
		{"shell_exec", "tool_execute", `"command":"ls","output":{"stdout":"PAYLOAD","stderr":"PAYLOAD","exit_code":0}`},
		{"read_file", "tool_read", `"file_path":"a","output":{"content":"PAYLOAD","line_count":1}`},
		{"modify_file", "tool_edit", `"file_path":"a","mutations":[{"type":"edit","content":"PAYLOAD","old_content":"PAYLOAD","new_content":"PAYLOAD","diff":"PAYLOAD"}]`},
		{"code_search", "tool_search", `"query":"a","output":{"files":["PAYLOAD"],"file_count":1}`},
		{"http_request", "tool_call", `"url":"https://example.com","response":"PAYLOAD"`},
		{"generic", "tool_call", `"name":"ordinary_tool","input":{"arg":"PAYLOAD"},"output":"PAYLOAD"`},
	} {
		t.Run(tc.kind, func(t *testing.T) {
			raw := []byte(`{"tool_call_id":"id","title":"title","future":9007199254740993,"normalized":{"kind":"` + tc.kind + `","` + tc.kind + `":{` + strings.ReplaceAll(tc.body, "PAYLOAD", strings.Repeat("secret", 200)) + `,"future":9007199254740993}},"result":"` + strings.Repeat("secret", 200) + `"}`)
			got, err := ReduceToolPayload(tc.typ, raw, time.Date(2026, 9, 14, 0, 0, 0, 0, time.UTC))
			require.NoError(t, err)
			require.Empty(t, got.Reason)
			require.NotContains(t, string(got.Metadata), "secret")
			require.Contains(t, string(got.Metadata), "9007199254740993")
			require.Equal(t, int64(len(raw)-len(got.Metadata)), got.RemovedBytes)
			var m map[string]any
			require.NoError(t, json.Unmarshal(got.Metadata, &m))
			require.True(t, ToolPayloadRemoved(m))
			again, err := ReduceToolPayload(tc.typ, got.Metadata, time.Now())
			require.NoError(t, err)
			require.Equal(t, "already_removed", again.Reason)
			require.Equal(t, got.Metadata, again.Metadata)
		})
	}
}

func TestReduceToolPayloadSkipsUnsafeRecords(t *testing.T) {
	for _, tc := range []struct{ name, typ, raw, reason string }{
		{"invalid", "tool_call", "{", "malformed"},
		{"permission", "permission_request", `{"normalized":{"kind":"generic","generic":{"name":"ordinary","input":"large"}}}`, "unsupported"},
		{"mismatched kind", "tool_read", `{"normalized":{"kind":"shell_exec","shell_exec":{}}}`, "unsupported"},
		{"background", "tool_execute", `{"normalized":{"kind":"shell_exec","shell_exec":{},"background_work":{}}}`, "unsupported"},
		{"control", "tool_call", `{"normalized":{"kind":"generic","generic":{"name":"create_task_kandev","input":"large"}}}`, "unsupported"},
		{"structured envelope", "tool_call", `{"normalized":{"kind":"generic","generic":{"name":"ordinary","output":{"content":[{"type":"resource","resource":{}}]}}}}`, "unsupported"},
		{"string envelope", "tool_call", `{"normalized":{"kind":"generic","generic":{"name":"ordinary","output":"{\"task\":{}}"}}}`, "unsupported"},
		{"unexpected output envelope", "tool_execute", `{"normalized":{"kind":"shell_exec","shell_exec":{"output":{"stdout":{"resource":"must survive"}}}}}`, "malformed"},
		{"invalid output", "tool_execute", `{"normalized":{"kind":"shell_exec","shell_exec":{"output":[]}}}`, "malformed"},
		{"no net savings", "tool_execute", `{"normalized":{"kind":"shell_exec","shell_exec":{"output":{"stdout":"x"}}}}`, "no_payload"},
		{"empty", "tool_execute", `{"normalized":{"kind":"shell_exec","shell_exec":{}}}`, "no_payload"},
		{"oversize", "tool_execute", strings.Repeat("x", ToolPayloadMaxBytes+1), "oversize"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ReduceToolPayload(tc.typ, []byte(tc.raw), time.Now())
			require.NoError(t, err)
			require.Equal(t, tc.reason, got.Reason)
			require.Equal(t, []byte(tc.raw), got.Metadata)
			require.Zero(t, got.RemovedBytes)
		})
	}
}

func TestRemovalMarkerReportsExactNetBytesAtDecimalBoundaries(t *testing.T) {
	for size := 1100; size < 1500; size++ {
		raw := []byte(`{"normalized":{"kind":"shell_exec","shell_exec":{"output":{"stdout":"` + strings.Repeat("x", size) + `"}}}}`)
		got, err := ReduceToolPayload("tool_execute", raw, time.Date(2026, 9, 14, 0, 0, 0, 0, time.UTC))
		require.NoError(t, err)
		require.Empty(t, got.Reason)
		var decoded struct {
			Marker struct {
				Bytes float64 `json:"removed_bytes"`
			} `json:"payload_retention"`
		}
		require.NoError(t, json.Unmarshal(got.Metadata, &decoded))
		require.Equal(t, float64(got.RemovedBytes), decoded.Marker.Bytes, "input size %d", size)
	}
}

func TestToolPayloadRemovedFailsClosedForUnknownMarker(t *testing.T) {
	require.False(t, ToolPayloadRemoved(nil))
	require.True(t, ToolPayloadRemoved(map[string]any{"payload_retention": nil}))
}
