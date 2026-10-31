package streams

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestNormalizedPayloadMarshalRoundTrip(t *testing.T) {
	tests := []struct {
		name    string
		payload *NormalizedPayload
	}{
		{
			name:    "shell_exec",
			payload: NewShellExec("ls -la", "/home/user", "list files", 30000, false),
		},
		{
			name:    "read_file",
			payload: NewReadFile("/path/to/file.go", 0, 100),
		},
		{
			name:    "modify_file",
			payload: NewModifyFile("/path/to/file.go", []FileMutation{{Type: MutationPatch, Diff: "- old\n+ new"}}),
		},
		{
			name:    "code_search",
			payload: NewCodeSearch("query", "pattern", "/path", "*.go"),
		},
		{
			name:    "generic",
			payload: NewGeneric("custom_tool", map[string]any{"key": "value"}),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Marshal
			data, err := json.Marshal(tt.payload)
			if err != nil {
				t.Fatalf("Marshal failed: %v", err)
			}

			// Unmarshal
			var result NormalizedPayload
			if err := json.Unmarshal(data, &result); err != nil {
				t.Fatalf("Unmarshal failed: %v", err)
			}

			// Check kind matches
			if result.Kind() != tt.payload.Kind() {
				t.Errorf("Kind mismatch: got %q, want %q", result.Kind(), tt.payload.Kind())
			}

			// Check specific fields based on kind
			switch tt.payload.Kind() {
			case ToolKindShellExec:
				if result.ShellExec() == nil {
					t.Error("ShellExec() is nil after unmarshal")
				} else if result.ShellExec().Command != tt.payload.ShellExec().Command {
					t.Errorf("ShellExec.Command mismatch: got %q, want %q",
						result.ShellExec().Command, tt.payload.ShellExec().Command)
				}
			case ToolKindReadFile:
				if result.ReadFile() == nil {
					t.Error("ReadFile() is nil after unmarshal")
				} else if result.ReadFile().FilePath != tt.payload.ReadFile().FilePath {
					t.Errorf("ReadFile.FilePath mismatch: got %q, want %q",
						result.ReadFile().FilePath, tt.payload.ReadFile().FilePath)
				}
			case ToolKindModifyFile:
				if result.ModifyFile() == nil {
					t.Error("ModifyFile() is nil after unmarshal")
				} else if result.ModifyFile().FilePath != tt.payload.ModifyFile().FilePath {
					t.Errorf("ModifyFile.FilePath mismatch: got %q, want %q",
						result.ModifyFile().FilePath, tt.payload.ModifyFile().FilePath)
				}
			case ToolKindGeneric:
				if result.Generic() == nil {
					t.Error("Generic() is nil after unmarshal")
				} else if result.Generic().Name != tt.payload.Generic().Name {
					t.Errorf("Generic.Name mismatch: got %q, want %q",
						result.Generic().Name, tt.payload.Generic().Name)
				}
			}
		})
	}
}

func TestNormalizedPayloadSnapshotDetachesMutablePayloads(t *testing.T) {
	tests := []struct {
		name    string
		payload *NormalizedPayload
		check   func(*NormalizedPayload)
	}{
		{
			name: "read file output",
			payload: func() *NormalizedPayload {
				payload := NewReadFile("before.go", 1, 2)
				payload.ReadFile().Output = &ReadFileOutput{Content: "before"}
				return payload
			}(),
		},
		{
			name:    "modify file mutations",
			payload: NewModifyFile("before.go", []FileMutation{{Diff: "before"}}),
		},
		{
			name: "shell output",
			payload: func() *NormalizedPayload {
				payload := NewShellExec("echo before", "/tmp", "before", 1, false)
				payload.ShellExec().Output = &ShellExecOutput{Stdout: "before", RawTerminalOutput: "before"}
				return payload
			}(),
		},
		{
			name: "code search output",
			payload: func() *NormalizedPayload {
				payload := NewCodeSearch("before", "before", "/tmp", "*.go")
				payload.CodeSearch().Output = &CodeSearchOutput{Files: []string{"before.go"}}
				return payload
			}(),
		},
		{
			name: "http request",
			payload: &NormalizedPayload{
				kind:        ToolKindHttpRequest,
				httpRequest: &HttpRequestPayload{URL: "https://before", Response: "before"},
			},
		},
		{
			name: "generic nested JSON",
			payload: func() *NormalizedPayload {
				rawMessage := json.RawMessage(`{"nested":["before"]}`)
				return &NormalizedPayload{
					kind: ToolKindGeneric,
					generic: &GenericPayload{
						Name: "before",
						Input: map[string]any{
							"nested": map[string]any{"values": []any{rawMessage}},
						},
						Output: []any{map[string]any{"value": "before"}},
					},
				}
			}(),
		},
		{
			name:    "create task",
			payload: &NormalizedPayload{kind: ToolKindCreateTask, createTask: &CreateTaskPayload{Title: "before"}},
		},
		{
			name: "subagent result",
			payload: func() *NormalizedPayload {
				count := 1
				payload := NewSubagentTask("before", "before", "before")
				payload.SubagentTask().ToolUseCount = &count
				payload.SubagentTask().SetIsAuggie(true)
				return payload
			}(),
			check: func(snapshot *NormalizedPayload) {
				if !snapshot.SubagentTask().IsAuggie() {
					t.Error("subagent adapter marker was not copied")
				}
			},
		},
		{
			name: "plan steps",
			payload: &NormalizedPayload{
				kind:     ToolKindShowPlan,
				showPlan: &ShowPlanPayload{Summary: "before", Steps: []string{"before"}},
			},
		},
		{
			name: "todo items",
			payload: &NormalizedPayload{
				kind:        ToolKindManageTodos,
				manageTodos: &ManageTodosPayload{Operation: "before", Items: []TodoItem{{Description: "before"}}},
			},
		},
		{
			name: "misc nested JSON",
			payload: func() *NormalizedPayload {
				rawMessage := json.RawMessage(`{"nested":["before"]}`)
				return &NormalizedPayload{
					kind: ToolKindMisc,
					misc: &MiscPayload{Label: "before", Details: map[string]any{
						"nested": []any{rawMessage},
					}},
				}
			}(),
		},
		{
			name: "adapter provenance",
			payload: func() *NormalizedPayload {
				payload := &NormalizedPayload{
					kind: ToolKindMisc,
					misc: &MiscPayload{Label: "before", Details: map[string]any{"value": "before"}},
				}
				payload.SetMonitorIdentity("before-task", false)
				return payload
			}(),
			check: func(snapshot *NormalizedPayload) {
				if snapshot.Monitor().TaskID != "before-task" || snapshot.BackgroundWork().WorkID != "before-task" {
					t.Error("adapter provenance was not copied")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			before, err := json.Marshal(tt.payload.Snapshot())
			if err != nil {
				t.Fatalf("marshal snapshot before mutation: %v", err)
			}
			snapshot := tt.payload.Snapshot()
			mutateSnapshotSource(tt.payload)
			after, err := json.Marshal(snapshot)
			if err != nil {
				t.Fatalf("marshal snapshot after mutation: %v", err)
			}
			if string(after) != string(before) {
				t.Fatalf("snapshot changed after source mutation: got %s, want %s", after, before)
			}
			if tt.check != nil {
				tt.check(snapshot)
			}
		})
	}
}

func mutateSnapshotSource(payload *NormalizedPayload) {
	switch payload.Kind() {
	case ToolKindReadFile:
		payload.ReadFile().Output.Content = "after"
	case ToolKindModifyFile:
		payload.ModifyFile().Mutations[0].Diff = "after"
	case ToolKindShellExec:
		payload.ShellExec().Output.Stdout = "after"
		payload.ShellExec().Output.RawTerminalOutput = "after"
	case ToolKindCodeSearch:
		payload.CodeSearch().Output.Files[0] = "after.go"
	case ToolKindHttpRequest:
		payload.HttpRequest().Response = "after"
	case ToolKindGeneric:
		payload.Generic().Input.(map[string]any)["nested"].(map[string]any)["values"].([]any)[0].(json.RawMessage)[0] = 'x'
		payload.Generic().Output.([]any)[0].(map[string]any)["value"] = "after"
		if payload.Monitor() != nil {
			payload.Monitor().TaskID = "after-task"
			payload.BackgroundWork().WorkID = "after-task"
		}
	case ToolKindCreateTask:
		payload.CreateTask().Title = "after"
	case ToolKindSubagentTask:
		payload.SubagentTask().Description = "after"
		*payload.SubagentTask().ToolUseCount = 2
	case ToolKindShowPlan:
		payload.ShowPlan().Steps[0] = "after"
	case ToolKindManageTodos:
		payload.ManageTodos().Items[0].Description = "after"
	case ToolKindMisc:
		details := payload.Misc().Details.(map[string]any)
		if nested, ok := details["nested"]; ok {
			nested.([]any)[0].(json.RawMessage)[0] = 'x'
		}
		if payload.Monitor() != nil {
			payload.Monitor().TaskID = "after-task"
			payload.BackgroundWork().WorkID = "after-task"
		}
	}
}

func TestAgentEventToolOutputBytesLegacyJSON(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantJSON  string
		wantValue *uint64
		wantError bool
	}{
		{
			name: "absent", input: `{"type":"tool_update"}`,
			wantJSON: `{"type":"tool_update","supports_image":false,"supports_audio":false,"supports_embedded_context":false,"supports_prompt_queueing":false}`,
		},
		{
			name: "null", input: `{"type":"tool_update","tool_output_bytes":null}`,
			wantJSON: `{"type":"tool_update","supports_image":false,"supports_audio":false,"supports_embedded_context":false,"supports_prompt_queueing":false}`,
		},
		{
			name: "zero", input: `{"type":"tool_update","tool_output_bytes":0}`,
			wantJSON: `{"type":"tool_update","tool_output_bytes":0,"supports_image":false,"supports_audio":false,"supports_embedded_context":false,"supports_prompt_queueing":false}`, wantValue: uint64Pointer(0),
		},
		{
			name: "nonzero", input: `{"type":"tool_update","tool_output_bytes":5}`,
			wantJSON: `{"type":"tool_update","tool_output_bytes":5,"supports_image":false,"supports_audio":false,"supports_embedded_context":false,"supports_prompt_queueing":false}`, wantValue: uint64Pointer(5),
		},
		{
			name: "maximum", input: `{"type":"tool_update","tool_output_bytes":18446744073709551615}`,
			wantJSON:  `{"type":"tool_update","tool_output_bytes":18446744073709551615,"supports_image":false,"supports_audio":false,"supports_embedded_context":false,"supports_prompt_queueing":false}`,
			wantValue: uint64Pointer(^uint64(0)),
		},
		{name: "string", input: `{"type":"tool_update","tool_output_bytes":"5"}`, wantError: true},
		{name: "negative", input: `{"type":"tool_update","tool_output_bytes":-1}`, wantError: true},
		{name: "fraction", input: `{"type":"tool_update","tool_output_bytes":1.5}`, wantError: true},
		{name: "overflow", input: `{"type":"tool_update","tool_output_bytes":18446744073709551616}`, wantError: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var event AgentEvent
			err := json.Unmarshal([]byte(test.input), &event)
			if (err != nil) != test.wantError {
				t.Fatalf("unmarshal error = %v, want error %t", err, test.wantError)
			}
			if test.wantError {
				return
			}
			if test.wantValue == nil {
				if event.ToolOutputBytes != nil {
					t.Fatalf("decoded field = %d, want nil", *event.ToolOutputBytes)
				}
			} else if event.ToolOutputBytes == nil || *event.ToolOutputBytes != *test.wantValue {
				t.Fatalf("decoded field = %v, want %d", event.ToolOutputBytes, *test.wantValue)
			}
			got, err := json.Marshal(event)
			if err != nil {
				t.Fatalf("marshal event: %v", err)
			}
			if string(got) != test.wantJSON {
				t.Fatalf("serialized event = %s, want %s", got, test.wantJSON)
			}
		})
	}
}

func TestAgentEventToolOutputBytesSerializationOrder(t *testing.T) {
	zero := uint64(0)
	encoded, err := json.Marshal(AgentEvent{
		Type:              EventTypeToolUpdate,
		ToolStatus:        "completed",
		ToolOutputBytes:   &zero,
		NormalizedPayload: NewGeneric("tool", nil),
	})
	if err != nil {
		t.Fatalf("marshal event: %v", err)
	}
	wire := string(encoded)
	status := strings.Index(wire, `"tool_status"`)
	outputBytes := strings.Index(wire, `"tool_output_bytes"`)
	normalized := strings.Index(wire, `"normalized"`)
	if status < 0 || outputBytes <= status || normalized <= outputBytes {
		t.Fatalf("serialized field order must be tool_status, tool_output_bytes, normalized: %s", wire)
	}
}

func uint64Pointer(value uint64) *uint64 {
	return &value
}
