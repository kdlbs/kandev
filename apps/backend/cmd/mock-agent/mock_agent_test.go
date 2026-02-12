package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseModelFromArgs(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "no flag returns default",
			args: []string{"mock-agent"},
			want: "mock-default",
		},
		{
			name: "separate flag and value",
			args: []string{"mock-agent", "--model", "mock-slow"},
			want: "mock-slow",
		},
		{
			name: "equals syntax",
			args: []string{"mock-agent", "--model=mock-fast"},
			want: "mock-fast",
		},
		{
			name: "flag with other args before",
			args: []string{"mock-agent", "--verbose", "--model", "mock-slow"},
			want: "mock-slow",
		},
		{
			name: "flag with other args after",
			args: []string{"mock-agent", "--model", "mock-fast", "--verbose"},
			want: "mock-fast",
		},
		{
			name: "dangling flag without value",
			args: []string{"mock-agent", "--model"},
			want: "mock-default",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseModelFromArgs(tt.args)
			if got != tt.want {
				t.Errorf("parseModelFromArgs(%v) = %q, want %q", tt.args, got, tt.want)
			}
		})
	}
}

func TestDelayRange(t *testing.T) {
	tests := []struct {
		model      string
		wantMinLo  int
		wantMinHi  int
		wantMaxLo  int
		wantMaxHi  int
	}{
		{"mock-fast", 10, 10, 50, 50},
		{"mock-slow", 500, 500, 3000, 3000},
		{"mock-default", 100, 100, 500, 500},
		{"unknown-model", 100, 100, 500, 500},
	}
	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			lo, hi := delayRange(tt.model)
			if lo != tt.wantMinLo || hi != tt.wantMaxHi {
				t.Errorf("delayRange(%q) = (%d, %d), want (%d, %d)", tt.model, lo, hi, tt.wantMinLo, tt.wantMaxHi)
			}
		})
	}
}

func TestReadFileSnippet(t *testing.T) {
	// Create a temp file with known content
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	content := "line1\nline2\nline3\nline4\nline5\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	t.Run("reads up to maxLines", func(t *testing.T) {
		result := readFileSnippet(path, 3)
		expected := "line1\nline2\nline3\n"
		if result != expected {
			t.Errorf("readFileSnippet(%q, 3) = %q, want %q", path, result, expected)
		}
	})

	t.Run("reads all lines when maxLines exceeds file", func(t *testing.T) {
		result := readFileSnippet(path, 100)
		expected := "line1\nline2\nline3\nline4\nline5\n"
		if result != expected {
			t.Errorf("readFileSnippet(%q, 100) = %q, want %q", path, result, expected)
		}
	})

	t.Run("returns fallback for missing file", func(t *testing.T) {
		result := readFileSnippet("/nonexistent/file.txt", 10)
		if result != "// (file not readable)\n" {
			t.Errorf("readFileSnippet(missing) = %q, want fallback", result)
		}
	})

	t.Run("handles empty file", func(t *testing.T) {
		emptyPath := filepath.Join(dir, "empty.txt")
		if err := os.WriteFile(emptyPath, []byte{}, 0644); err != nil {
			t.Fatal(err)
		}
		result := readFileSnippet(emptyPath, 10)
		if result != "\n" {
			t.Errorf("readFileSnippet(empty) = %q, want %q", result, "\n")
		}
	})
}

func TestPickEditableFragment(t *testing.T) {
	dir := t.TempDir()

	t.Run("returns fallback for missing file", func(t *testing.T) {
		old, new_ := pickEditableFragment("/nonexistent/file.go")
		if old != "hello" || new_ != "hello_mock" {
			t.Errorf("pickEditableFragment(missing) = (%q, %q), want (\"hello\", \"hello_mock\")", old, new_)
		}
	})

	t.Run("returns fallback for file with only short lines", func(t *testing.T) {
		path := filepath.Join(dir, "short.txt")
		if err := os.WriteFile(path, []byte("a\nb\nc\n"), 0644); err != nil {
			t.Fatal(err)
		}
		old, new_ := pickEditableFragment(path)
		if old != "original" || new_ != "modified" {
			t.Errorf("pickEditableFragment(short) = (%q, %q), want (\"original\", \"modified\")", old, new_)
		}
	})

	t.Run("produces different old and new strings", func(t *testing.T) {
		path := filepath.Join(dir, "code.go")
		content := "package main\n\nfunc main() {\n\tfmt.Println(\"hello world\")\n}\n"
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
		old, new_ := pickEditableFragment(path)
		if old == new_ {
			t.Errorf("pickEditableFragment should produce different old and new, got %q", old)
		}
		if old == "" {
			t.Error("old string should not be empty")
		}
	})
}

func TestDiscoverFiles(t *testing.T) {
	// Reset global state
	workspaceFiles = nil

	// Save and restore working directory
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(origWd) }()

	dir := t.TempDir()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}

	// Create test files
	for _, f := range []struct{ name, content string }{
		{"main.go", "package main"},
		{"util.ts", "export {}"},
		{"image.png", "fake png"}, // should be skipped (non-text extension)
	} {
		if err := os.WriteFile(filepath.Join(dir, f.name), []byte(f.content), 0644); err != nil {
			t.Fatal(err)
		}
	}

	// Create a skipped directory
	if err := os.MkdirAll(filepath.Join(dir, "node_modules"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "node_modules", "lib.js"), []byte("//"), 0644); err != nil {
		t.Fatal(err)
	}

	// Reset cache before test
	workspaceFiles = nil
	files := discoverFiles()

	// Should find .go and .ts but not .png or node_modules
	foundGo, foundTs, foundPng, foundNodeModules := false, false, false, false
	for _, f := range files {
		switch filepath.Base(f.absPath) {
		case "main.go":
			foundGo = true
		case "util.ts":
			foundTs = true
		case "image.png":
			foundPng = true
		case "lib.js":
			foundNodeModules = true
		}
	}

	if !foundGo {
		t.Error("expected to find main.go")
	}
	if !foundTs {
		t.Error("expected to find util.ts")
	}
	if foundPng {
		t.Error("should not find image.png (not a text extension)")
	}
	if foundNodeModules {
		t.Error("should not find files in node_modules")
	}

	// Reset global state for other tests
	workspaceFiles = nil
}
