package installer

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
	tools "github.com/kandev/kandev/internal/tools/installer"
)

type registryCommandRunner struct {
	environment map[string]string
}

func (r *registryCommandRunner) CombinedOutput(
	_ context.Context,
	_ tools.CommandSpec,
) ([]byte, error) {
	return nil, nil
}

func (r *registryCommandRunner) CommandEnvironment() (map[string]string, error) {
	return r.environment, nil
}

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
	expected := []string{"typescript", "go", "rust", "python", "kotlin"}
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
		{"kotlin", true},
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
		{"kotlin", "kotlin-lsp", []string{"--stdio"}},
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

func TestCanAutoInstall(t *testing.T) {
	for _, language := range []string{"typescript", "go", "python"} {
		if !CanAutoInstall(language) {
			t.Errorf("CanAutoInstall(%q) = false, want true", language)
		}
	}
	for _, goos := range []string{darwinOS, linuxOS} {
		if !canAutoInstallOnPlatform("rust", goos) {
			t.Errorf("canAutoInstallOnPlatform(rust, %q) = false, want true", goos)
		}
	}
	if canAutoInstallOnPlatform("rust", windowsOS) {
		t.Error("canAutoInstallOnPlatform(rust, windows) = true, want false")
	}
	wantRust := runtime.GOOS == darwinOS || runtime.GOOS == linuxOS
	if got := CanAutoInstall("rust"); got != wantRust {
		t.Errorf("CanAutoInstall(rust) = %v, want %v on %s", got, wantRust, runtime.GOOS)
	}
	for _, language := range []string{"kotlin", "java", ""} {
		if CanAutoInstall(language) {
			t.Errorf("CanAutoInstall(%q) = true, want false", language)
		}
	}
}

func TestAutoInstallLanguagesForPlatform(t *testing.T) {
	tests := []struct {
		goos string
		want []string
	}{
		{goos: linuxOS, want: []string{"go", "python", "rust", "typescript"}},
		{goos: darwinOS, want: []string{"go", "python", "rust", "typescript"}},
		{goos: windowsOS, want: []string{"go", "python", "typescript"}},
	}
	for _, tc := range tests {
		if got := autoInstallLanguagesForPlatform(tc.goos); !slices.Equal(got, tc.want) {
			t.Errorf("autoInstallLanguagesForPlatform(%q) = %v, want %v", tc.goos, got, tc.want)
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
	r := NewRegistry("", testLogger())

	installable := []string{"typescript", "go", "python"}
	if CanAutoInstall("rust") {
		installable = append(installable, "rust")
	}
	for _, lang := range installable {
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
	if !CanAutoInstall("rust") {
		if _, err := r.StrategyFor("rust"); err == nil {
			t.Error("StrategyFor(rust) should reject auto-install on this platform")
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
	r := NewRegistry("", testLogger())

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
	t.Setenv("PATH", t.TempDir())
	tmpDir := t.TempDir()
	binaryName := "typescript-language-server"
	if runtime.GOOS == windowsOS {
		binaryName += ".cmd"
	}
	fakeBinary := filepath.Join(tmpDir, "node_modules", ".bin", binaryName)
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
		t.Fatalf("BinaryPath(typescript) error = %v", err)
	}
	if p != fakeBinary {
		t.Errorf("BinaryPath(typescript) = %q, want %q", p, fakeBinary)
	}
}

func TestBinaryPath_DirectBinary(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	tmpDir := t.TempDir()
	binaryName := "rust-analyzer"
	if runtime.GOOS == windowsOS {
		binaryName += ".exe"
	}
	fakeBinary := filepath.Join(tmpDir, binaryName)
	if err := os.WriteFile(fakeBinary, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	r := &Registry{
		binDir: tmpDir,
		logger: testLogger(),
	}

	p, err := r.BinaryPath("rust")
	if err != nil {
		t.Fatalf("BinaryPath(rust) error = %v", err)
	}
	if p != fakeBinary {
		t.Errorf("BinaryPath(rust) = %q, want %q", p, fakeBinary)
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
	r := NewRegistry("", testLogger())
	_, err := r.BinaryPath("java")
	if err == nil {
		t.Error("BinaryPath(\"java\") should return error for unsupported language")
	}
}

func TestNewRegistryFailsClosedWithoutTrustedHome(t *testing.T) {
	workDir := t.TempDir()
	t.Chdir(workDir)
	t.Setenv("HOME", "")
	t.Setenv("PATH", "")
	projectBinary := filepath.Join(workDir, DefaultBinDir, "kotlin-lsp")
	if err := os.MkdirAll(filepath.Dir(projectBinary), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(projectBinary, []byte("project-controlled"), 0o755); err != nil {
		t.Fatal(err)
	}

	r := NewRegistry("", testLogger())
	if r.binDir != "" {
		t.Fatalf("binDir = %q, want disabled cache", r.binDir)
	}
	if path, err := r.BinaryPath("kotlin"); err == nil {
		t.Fatalf("BinaryPath(kotlin) = %q from relative project cache, want not found", path)
	}
}

func TestFindGoBinary(t *testing.T) {
	// Test with GOBIN set to a temp directory containing a fake binary
	tmpDir := t.TempDir()
	binaryName := "gopls"
	if runtime.GOOS == "windows" {
		binaryName += ".exe"
	}
	fakeBinary := filepath.Join(tmpDir, binaryName)
	if err := os.WriteFile(fakeBinary, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	t.Setenv("GOBIN", tmpDir)
	t.Setenv("GOPATH", "")

	p, err := tools.FindGoBinary("gopls")
	if err != nil {
		t.Errorf("tools.FindGoBinary(\"gopls\") error = %v", err)
	}
	if p != fakeBinary {
		t.Errorf("tools.FindGoBinary(\"gopls\") = %q, want %q", p, fakeBinary)
	}
}

func TestBinaryPathUsesTaskEnvironment(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	taskBin := t.TempDir()
	binaryName := "gopls"
	if runtime.GOOS == "windows" {
		binaryName += ".exe"
	}
	binaryPath := filepath.Join(taskBin, binaryName)
	if err := os.WriteFile(binaryPath, []byte("fixture"), 0o755); err != nil {
		t.Fatal(err)
	}
	registry := NewRegistry("", testLogger(), WithCommandRunner(&registryCommandRunner{
		environment: map[string]string{"PATH": taskBin},
	}))

	path, err := registry.BinaryPath("go")
	if err != nil {
		t.Fatalf("BinaryPath(go) error = %v", err)
	}
	if path != binaryPath {
		t.Fatalf("BinaryPath(go) = %q, want task-environment path %q", path, binaryPath)
	}
}

func TestFindGoBinary_NotFound(t *testing.T) {
	t.Setenv("GOBIN", t.TempDir())
	t.Setenv("GOPATH", t.TempDir())
	t.Setenv("HOME", t.TempDir())

	_, err := tools.FindGoBinary("nonexistent-binary")
	if err == nil {
		t.Error("tools.FindGoBinary(\"nonexistent-binary\") should return error")
	}
}
