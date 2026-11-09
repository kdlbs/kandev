package acpdbg

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestPrompt_SelectsModelFromConfigOptionAndFallsBackToLegacy(t *testing.T) {
	tests := []struct {
		name        string
		mode        string
		wantMethods []string
	}{
		{
			name:        "model category",
			mode:        "config-option",
			wantMethods: []string{"initialize", "session/new", "session/set_config_option", "session/prompt"},
		},
		{
			name:        "legacy model list",
			mode:        "legacy",
			wantMethods: []string{"initialize", "session/new", "session/set_model", "session/prompt"},
		},
		{
			name:        "config method not found",
			mode:        "config-method-not-found",
			wantMethods: []string{"initialize", "session/new", "session/set_config_option", "session/set_model", "session/prompt"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("ACPDBG_MODEL_PEER", test.mode)
			self, err := os.Executable()
			if err != nil {
				t.Fatalf("locate test binary: %v", err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			runner, err := NewRunner(ctx, filepath.Join(t.TempDir(), "model-selection.jsonl"), RunConfig{
				AgentID: "model-selection-peer",
				Command: []string{self, "-test.run=^TestACPDBGModelSelectionPeer$"},
			})
			if err != nil {
				t.Fatalf("NewRunner: %v", err)
			}
			defer runner.Close("test complete")

			if _, err := Prompt(ctx, runner, PromptOptions{Model: "opus", Prompt: "hello"}); err != nil {
				t.Fatalf("Prompt: %v", err)
			}
			frames := recordedSentFrames(t, runner.Path())
			methods := make([]string, 0, len(frames))
			for _, frame := range frames {
				methods = append(methods, frame.Method())
			}
			if !reflect.DeepEqual(methods, test.wantMethods) {
				t.Fatalf("sent methods = %v, want %v", methods, test.wantMethods)
			}
			if test.mode != "legacy" {
				frame := findSentFrame(t, frames, "session/set_config_option")
				params, ok := frame["params"].(map[string]any)
				if !ok || params["configId"] != "model-choice" || params["value"] != "opus" {
					t.Fatalf("set_config_option params = %v", frame["params"])
				}
			}
		})
	}
}

func TestACPDBGModelSelectionPeer(t *testing.T) {
	mode := os.Getenv("ACPDBG_MODEL_PEER")
	if mode == "" {
		return
	}
	scanner := bufio.NewScanner(os.Stdin)
	encoder := json.NewEncoder(os.Stdout)
	for scanner.Scan() {
		var request map[string]any
		if err := json.Unmarshal(scanner.Bytes(), &request); err != nil {
			os.Exit(1)
		}
		result := map[string]any{}
		var rpcError map[string]any
		switch request["method"] {
		case "initialize":
			result["protocolVersion"] = 1
		case "session/new":
			result["sessionId"] = "session-1"
			if mode != "legacy" {
				result["configOptions"] = []any{map[string]any{
					"id": "model-choice", "category": "model", "type": "select",
					"currentValue": "default", "options": []any{map[string]any{"value": "opus"}},
				}}
			}
		case "session/set_config_option":
			if mode == "config-method-not-found" {
				rpcError = map[string]any{"code": -32601, "message": "method not found"}
			}
		case "session/prompt":
			result["stopReason"] = "end_turn"
		}
		response := map[string]any{"jsonrpc": "2.0", "id": request["id"]}
		if rpcError != nil {
			response["error"] = rpcError
		} else {
			response["result"] = result
		}
		if err := encoder.Encode(response); err != nil {
			os.Exit(1)
		}
	}
}

func recordedSentFrames(t *testing.T, path string) []Frame {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("open JSONL: %v", err)
	}
	defer func() {
		if err := file.Close(); err != nil {
			t.Errorf("close JSONL: %v", err)
		}
	}()
	var frames []Frame
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var entry Entry
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			t.Fatalf("decode JSONL entry: %v", err)
		}
		if entry.Direction == DirSent {
			frames = append(frames, entry.Frame)
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan JSONL: %v", err)
	}
	return frames
}

func findSentFrame(t *testing.T, frames []Frame, method string) Frame {
	t.Helper()
	for _, frame := range frames {
		if frame.Method() == method {
			return frame
		}
	}
	t.Fatalf("sent frame %q not found in %v", method, frames)
	return nil
}
