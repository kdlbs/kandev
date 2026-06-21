package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestBuildPromptArgs(t *testing.T) {
	tests := []struct {
		name           string
		prompt, model  string
		continueSesson bool
		want           []string
	}{
		{
			name:   "first turn with model",
			prompt: "find the shortest file",
			model:  "Gemini 3.5 Flash (Medium)",
			want:   []string{"--print", "find the shortest file", "--model", "Gemini 3.5 Flash (Medium)"},
		},
		{
			name:           "continued turn",
			prompt:         "now sort them",
			model:          "Gemini 3.5 Flash (Medium)",
			continueSesson: true,
			want:           []string{"--print", "now sort them", "--model", "Gemini 3.5 Flash (Medium)", "--continue"},
		},
		{
			name:   "no model",
			prompt: "hi",
			want:   []string{"--print", "hi"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := buildPromptArgs(tc.prompt, tc.model, tc.continueSesson); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("buildPromptArgs() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestParseModels(t *testing.T) {
	out := "Gemini 3.5 Flash (Medium)\n\n  Claude Opus 4.6 (Thinking)  \n"
	want := []modelEntry{
		{ID: "Gemini 3.5 Flash (Medium)", Name: "Gemini 3.5 Flash (Medium)"},
		{ID: "Claude Opus 4.6 (Thinking)", Name: "Claude Opus 4.6 (Thinking)"},
	}
	if got := parseModels(out); !reflect.DeepEqual(got, want) {
		t.Fatalf("parseModels() = %v, want %v", got, want)
	}
	if got := parseModels(""); got != nil {
		t.Fatalf("parseModels(empty) = %v, want nil", got)
	}
}

func TestParseModelFlag(t *testing.T) {
	if got := parseModelFlag([]string{"antigravity-acp", "--model", "M1"}); got != "M1" {
		t.Fatalf("parseModelFlag(space) = %q", got)
	}
	if got := parseModelFlag([]string{"antigravity-acp", "--model=M2"}); got != "M2" {
		t.Fatalf("parseModelFlag(eq) = %q", got)
	}
	if got := parseModelFlag([]string{"antigravity-acp"}); got != "" {
		t.Fatalf("parseModelFlag(none) = %q", got)
	}
}

func TestSeedTrustedFolder(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".gemini", "trustedFolders.json")
	ws := "/work/space"

	if err := seedTrustedFolder(path, ws); err != nil {
		t.Fatalf("seedTrustedFolder() error = %v", err)
	}
	folders := map[string]string{}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if err := json.Unmarshal(data, &folders); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if folders[ws] != trustValue {
		t.Fatalf("trust[%q] = %q, want %q", ws, folders[ws], trustValue)
	}

	// Idempotent + preserves existing entries.
	if err := seedTrustedFolder(path, "/other"); err != nil {
		t.Fatalf("second seed error = %v", err)
	}
	data, _ = os.ReadFile(path)
	_ = json.Unmarshal(data, &folders)
	if folders[ws] != trustValue || folders["/other"] != trustValue {
		t.Fatalf("expected both entries, got %v", folders)
	}
}
