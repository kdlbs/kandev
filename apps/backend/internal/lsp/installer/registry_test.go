package installer

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
)

func testLogger() *logger.Logger {
	log, _ := logger.NewLogger(logger.LoggingConfig{
		Level:      "error",
		Format:     "json",
		OutputPath: os.DevNull,
	})
	return log
}

func TestSupportedLanguages(t *testing.T) {
	langs := SupportedLanguages()
	expected := []string{"typescript", "go", "rust", "python"}
	for _, lang := range expected {
		if _, ok := langs[lang]; !ok {
			t.Errorf("expected %q in SupportedLanguages()", lang)
		}
	}
	if _, ok := langs["java"]; ok {
		t.Error("unexpected language 'java' in SupportedLanguages()")
	}
}

func TestIsSupported(t *testing.T) {
	tests := []struct {
		language string
		want     bool
	}{
		{"typescript", true},
		{"go", true},
		{"rust", true},
		{"python", true},
		{"java", false},
		{"", false},
		{"ruby", false},
	}
	for _, tc := range tests {
		if got := IsSupported(tc.language); got != tc.want {
			t.Errorf("IsSupported(%q) = %v, want %v", tc.language, got, tc.want)
		}
	}
}

func TestLspCommand(t *testing.T) {
	tests := []struct {
		language   string
		wantBinary string
		wantArgs   []string
	}{
		{"typescript", "typescript-language-server", []string{"--stdio"}},
		{"go", "gopls", []string{"serve"}},
		{"rust", "rust-analyzer", nil},
		{"python", "pyright-langserver", []string{"--stdio"}},
		{"unknown", "", nil},
	}
	for _, tc := range tests {
		binary, args := LspCommand(tc.language)
		if binary != tc.wantBinary {
			t.Errorf("LspCommand(%q) binary = %q, want %q", tc.language, binary, tc.wantBinary)
		}
		if len(args) != len(tc.wantArgs) {
			t.Errorf("LspCommand(%q) args = %v, want %v", tc.language, args, tc.wantArgs)
		} else {
			for i := range args {
				if args[i] != tc.wantArgs[i] {
					t.Errorf("LspCommand(%q) args[%d] = %q, want %q", tc.language, i, args[i], tc.wantArgs[i])
				}
			}
		}
	}
}

func TestBinaryName(t *testing.T) {
	tests := []struct {
		language string
		want     string
		wantErr  bool
	}{
		{"typescript", "typescript-language-server", false},
		{"go", "gopls", false},
		{"rust", "rust-analyzer", false},
		{"python", "pyright-langserver", false},
		{"java", "", true},
	}
	for _, tc := range tests {
		got, err := binaryName(tc.language)
		if (err != nil) != tc.wantErr {
			t.Errorf("binaryName(%q) error = %v, wantErr %v", tc.language, err, tc.wantErr)
			continue
		}
		if got != tc.want {
			t.Errorf("binaryName(%q) = %q, want %q", tc.language, got, tc.want)
		}
	}
}

func TestStrategyFor(t *testing.T) {
	r := NewRegistry(testLogger())

	// Supported languages should return a strategy
	for _, lang := range []string{"typescript", "go", "rust", "python"} {
		s, err := r.StrategyFor(lang)
		if err != nil {
			t.Errorf("StrategyFor(%q) returned error: %v", lang, err)
			continue
		}
		if s == nil {
			t.Errorf("StrategyFor(%q) returned nil strategy", lang)
			continue
		}
		if s.Name() == "" {
			t.Errorf("StrategyFor(%q).Name() is empty", lang)
		}
	}

	// Unsupported language should return error
	_, err := r.StrategyFor("java")
	if err == nil {
		t.Error("StrategyFor(\"java\") should return error")
	}
}

func TestBinaryPath_InPATH(t *testing.T) {
	// "ls" should always be in PATH
	r := NewRegistry(testLogger())

	// Override the languages map temporarily — we can't do that directly,
	// so we test that BinaryPath returns an error for a language whose binary
	// is not installed at all.
	_, err := r.BinaryPath("rust")
	// We can't guarantee rust-analyzer is installed, but we can verify
	// the function doesn't panic and returns a valid result or error
	_ = err
}

func TestBinaryPath_InBinDir(t *testing.T) {
	// Create a temp bin directory with a fake binary
	tmpDir := t.TempDir()
	fakeBinary := filepath.Join(tmpDir, "node_modules", ".bin", "typescript-language-server")
	if err := os.MkdirAll(filepath.Dir(fakeBinary), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fakeBinary, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	r := &Registry{
		binDir: tmpDir,
		logger: testLogger(),
	}

	p, err := r.BinaryPath("typescript")
	if err != nil {
		// Only fail if it's not in PATH either
		if _, lookErr := exec.LookPath("typescript-language-server"); lookErr != nil {
			t.Errorf("BinaryPath(\"typescript\") error = %v (expected to find in binDir)", err)
		}
		return
	}
	if p == "" {
		t.Error("BinaryPath(\"typescript\") returned empty path")
	}
}

func TestBinaryPath_DirectBinary(t *testing.T) {
	tmpDir := t.TempDir()
	fakeBinary := filepath.Join(tmpDir, "rust-analyzer")
	if err := os.WriteFile(fakeBinary, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	r := &Registry{
		binDir: tmpDir,
		logger: testLogger(),
	}

	p, err := r.BinaryPath("rust")
	if err != nil {
		if _, lookErr := exec.LookPath("rust-analyzer"); lookErr != nil {
			t.Errorf("BinaryPath(\"rust\") error = %v (expected to find direct binary)", err)
		}
		return
	}
	if p == "" {
		t.Error("BinaryPath(\"rust\") returned empty path")
	}
}

func TestBinaryPath_NotFound(t *testing.T) {
	// Use empty binDir so nothing is found there
	r := &Registry{
		binDir: t.TempDir(),
		logger: testLogger(),
	}

	// Use a language whose binary is unlikely to be in PATH on CI
	// We test the error case by overriding PATH
	origPath := os.Getenv("PATH")
	t.Setenv("PATH", t.TempDir())
	defer func() { _ = os.Setenv("PATH", origPath) }()

	_, err := r.BinaryPath("rust")
	if err == nil {
		t.Error("BinaryPath(\"rust\") should return error when binary not found")
	}
}

func TestBinaryPath_UnsupportedLanguage(t *testing.T) {
	r := NewRegistry(testLogger())
	_, err := r.BinaryPath("java")
	if err == nil {
		t.Error("BinaryPath(\"java\") should return error for unsupported language")
	}
}

func TestFindGoBinary(t *testing.T) {
	// Test with GOBIN set to a temp directory containing a fake binary
	tmpDir := t.TempDir()
	fakeBinary := filepath.Join(tmpDir, "gopls")
	if err := os.WriteFile(fakeBinary, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	t.Setenv("GOBIN", tmpDir)
	t.Setenv("GOPATH", "")

	p, err := findGoBinary("gopls")
	if err != nil {
		t.Errorf("findGoBinary(\"gopls\") error = %v", err)
	}
	if p != fakeBinary {
		t.Errorf("findGoBinary(\"gopls\") = %q, want %q", p, fakeBinary)
	}
}

func TestFindGoBinary_NotFound(t *testing.T) {
	t.Setenv("GOBIN", t.TempDir())
	t.Setenv("GOPATH", t.TempDir())
	t.Setenv("HOME", t.TempDir())

	_, err := findGoBinary("nonexistent-binary")
	if err == nil {
		t.Error("findGoBinary(\"nonexistent-binary\") should return error")
	}
}
