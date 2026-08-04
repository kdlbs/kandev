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
	spec           CommandSpec
	output         []byte
	err            error
	environment    map[string]string
	environmentErr error
}

func (r *recordingCommandRunner) CombinedOutput(_ context.Context, spec CommandSpec) ([]byte, error) {
	r.spec = spec
	return r.output, r.err
}

func (r *recordingCommandRunner) CommandEnvironment() (map[string]string, error) {
	return r.environment, r.environmentErr
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
	if runtime.GOOS == windowsOS {
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

func TestNpmStrategyUsesRunnerEnvironmentForLookup(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	taskBin := t.TempDir()
	npmName := "npm"
	if runtime.GOOS == windowsOS {
		npmName += ".exe"
	}
	npmPath := filepath.Join(taskBin, npmName)
	if err := os.WriteFile(npmPath, []byte("fixture"), 0o755); err != nil {
		t.Fatal(err)
	}
	binDir := t.TempDir()
	binaryPath := filepath.Join(binDir, "node_modules", ".bin", "pyright-langserver")
	if err := os.MkdirAll(filepath.Dir(binaryPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(binaryPath, []byte("fixture"), 0o755); err != nil {
		t.Fatal(err)
	}
	runner := &recordingCommandRunner{environment: map[string]string{"PATH": taskBin}}
	strategy := NewNpmStrategy(
		binDir,
		"pyright-langserver",
		[]string{"pyright"},
		installStrategyTestLogger(t),
		runner,
	)

	if _, err := strategy.Install(context.Background()); err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	if runner.spec.Path != npmPath {
		t.Fatalf("command path = %q, want task-environment path %q", runner.spec.Path, npmPath)
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

func TestGoInstallStrategyUsesRunnerEnvironmentForLookupAndResult(t *testing.T) {
	parentPath := t.TempDir()
	t.Setenv("PATH", parentPath)
	t.Setenv("GOBIN", "")
	t.Setenv("GOPATH", "")

	taskBin := t.TempDir()
	goName := "go"
	goplsName := "gopls"
	if runtime.GOOS == windowsOS {
		goName += ".exe"
		goplsName += ".exe"
	}
	goPath := filepath.Join(taskBin, goName)
	if err := os.WriteFile(goPath, []byte("fixture"), 0o755); err != nil {
		t.Fatal(err)
	}
	gobin := t.TempDir()
	goplsPath := filepath.Join(gobin, goplsName)
	if err := os.WriteFile(goplsPath, []byte("fixture"), 0o755); err != nil {
		t.Fatal(err)
	}

	runner := &recordingCommandRunner{environment: map[string]string{
		"PATH":  taskBin,
		"GOBIN": gobin,
		"HOME":  t.TempDir(),
	}}
	strategy := NewGoInstallStrategy(
		"gopls",
		"golang.org/x/tools/gopls@latest",
		installStrategyTestLogger(t),
		runner,
	)

	result, err := strategy.Install(context.Background())
	if err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	if runner.spec.Path != goPath {
		t.Fatalf("command path = %q, want task-environment path %q", runner.spec.Path, goPath)
	}
	if runner.spec.Env["GOBIN"] != gobin {
		t.Fatalf("command GOBIN = %q, want %q", runner.spec.Env["GOBIN"], gobin)
	}
	if result.BinaryPath != goplsPath {
		t.Fatalf("BinaryPath = %q, want task-environment path %q", result.BinaryPath, goplsPath)
	}
}
