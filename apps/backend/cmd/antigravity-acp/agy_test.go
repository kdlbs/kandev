package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
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

func TestSeedTrustedFolderSymlinkRefused(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "real.json")
	if err := os.WriteFile(target, []byte("{}"), 0o600); err != nil {
		t.Fatalf("write target: %v", err)
	}
	link := filepath.Join(dir, "trustedFolders.json")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlink unsupported on this platform: %v", err)
	}

	err := seedTrustedFolder(link, "/work/space")
	if err == nil {
		t.Fatal("expected error for symlink target, got nil")
	}
	// The symlinked target must be left untouched (no write through the link).
	if data, _ := os.ReadFile(target); string(data) != "{}" {
		t.Fatalf("symlink target was modified: %q", data)
	}
}

func TestTrustWorkspaceHomeUnresolvable(t *testing.T) {
	// With no resolvable home directory, trustedFoldersPath returns ok=false and
	// trustWorkspace must return early without panicking or logging a failure.
	t.Setenv("HOME", "")
	t.Setenv("USERPROFILE", "") // Windows fallback used by os.UserHomeDir.
	if _, ok := trustedFoldersPath(); ok {
		t.Skip("home directory still resolvable; cannot exercise early-return path")
	}

	var logs strings.Builder
	orig := logOutput
	logOutput = &logs
	t.Cleanup(func() { logOutput = orig })

	trustWorkspace("/work/space") // must not panic
	if logs.Len() != 0 {
		t.Fatalf("expected no log output on early return, got %q", logs.String())
	}
}
