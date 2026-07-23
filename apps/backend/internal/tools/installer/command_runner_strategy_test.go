package installer

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
)

type recordingCommandRunner struct {
	spec   CommandSpec
	output []byte
	err    error
}

func (r *recordingCommandRunner) CombinedOutput(_ context.Context, spec CommandSpec) ([]byte, error) {
	r.spec = spec
	return r.output, r.err
}

func installStrategyTestLogger(t *testing.T) *logger.Logger {
	t.Helper()
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json", OutputPath: os.DevNull})
	if err != nil {
		t.Fatal(err)
	}
	return log
}

func putFakeExecutableOnPath(t *testing.T, name string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	dir := t.TempDir()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("fixture"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	return path
}

func TestNpmStrategyUsesInjectedCommandRunner(t *testing.T) {
	npmPath := putFakeExecutableOnPath(t, "npm")
	binDir := t.TempDir()
	binaryPath := filepath.Join(binDir, "node_modules", ".bin", "pyright-langserver")
	if err := os.MkdirAll(filepath.Dir(binaryPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(binaryPath, []byte("fixture"), 0o755); err != nil {
		t.Fatal(err)
	}
	runner := &recordingCommandRunner{}
	strategy := NewNpmStrategy(binDir, "pyright-langserver", []string{"pyright"}, installStrategyTestLogger(t), runner)

	result, err := strategy.Install(context.Background())
	if err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	if result.BinaryPath != binaryPath {
		t.Fatalf("BinaryPath = %q, want %q", result.BinaryPath, binaryPath)
	}
	if runner.spec.Path != npmPath || runner.spec.Dir != binDir {
		t.Fatalf("command spec = %+v, want path %q and dir %q", runner.spec, npmPath, binDir)
	}
	wantArgs := []string{installSubcommand, "--prefix", binDir, "pyright"}
	if len(runner.spec.Args) != len(wantArgs) {
		t.Fatalf("args = %v, want %v", runner.spec.Args, wantArgs)
	}
	for i := range wantArgs {
		if runner.spec.Args[i] != wantArgs[i] {
			t.Fatalf("args[%d] = %q, want %q", i, runner.spec.Args[i], wantArgs[i])
		}
	}
}

func TestGoInstallStrategyUsesInjectedCommandRunner(t *testing.T) {
	goPath := putFakeExecutableOnPath(t, "go")
	runnerErr := errors.New("managed runner stopped")
	runner := &recordingCommandRunner{output: []byte("compiler output"), err: runnerErr}
	strategy := NewGoInstallStrategy("gopls", "golang.org/x/tools/gopls@latest", installStrategyTestLogger(t), runner)

	_, err := strategy.Install(context.Background())
	if !errors.Is(err, runnerErr) {
		t.Fatalf("Install() error = %v, want %v", err, runnerErr)
	}
	if runner.spec.Path != goPath {
		t.Fatalf("command path = %q, want %q", runner.spec.Path, goPath)
	}
	wantArgs := []string{installSubcommand, "golang.org/x/tools/gopls@latest"}
	if len(runner.spec.Args) != len(wantArgs) {
		t.Fatalf("args = %v, want %v", runner.spec.Args, wantArgs)
	}
	for i := range wantArgs {
		if runner.spec.Args[i] != wantArgs[i] {
			t.Fatalf("args[%d] = %q, want %q", i, runner.spec.Args[i], wantArgs[i])
		}
	}
}
