package installer

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/kandev/kandev/internal/common/logger"
	"go.uber.org/zap"
)

// GoInstallStrategy installs tools via `go install`.
type GoInstallStrategy struct {
	binary     string // "gopls"
	importPath string // "golang.org/x/tools/gopls@latest"
	logger     *logger.Logger
	runner     CommandRunner
}

// NewGoInstallStrategy creates a new go install strategy.
func NewGoInstallStrategy(binary, importPath string, log *logger.Logger, runners ...CommandRunner) *GoInstallStrategy {
	strategy := &GoInstallStrategy{
		binary:     binary,
		importPath: importPath,
		logger:     log,
	}
	if len(runners) > 0 {
		strategy.runner = runners[0]
	}
	return strategy
}

func (s *GoInstallStrategy) Name() string {
	return fmt.Sprintf("go install %s", s.importPath)
}

func (s *GoInstallStrategy) Install(ctx context.Context) (*InstallResult, error) {
	goPath, commandEnv, err := resolveCommand("go", s.runner)
	if err != nil {
		return nil, fmt.Errorf("go not found: %w", err)
	}

	s.logger.Info("installing via go install", zap.String("import_path", s.importPath))

	output, err := combinedOutput(ctx, s.runner, CommandSpec{
		Path: goPath,
		Args: []string{installSubcommand, s.importPath},
		Env:  commandEnv,
	})
	if err != nil {
		return nil, fmt.Errorf("go install failed: %w\nOutput: %s", err, string(output))
	}

	// Find the installed binary using the shared Go binary lookup
	binaryPath, err := FindGoBinaryWithRunner(s.binary, s.runner)
	if err != nil {
		return nil, err
	}

	s.logger.Info("go install completed", zap.String("binary", binaryPath))
	return &InstallResult{
		BinaryPath: binaryPath,
	}, nil
}

// FindGoBinary looks for a Go binary in GOBIN, GOPATH/bin, and the platform's
// default user Go workspace.
func FindGoBinary(binary string) (string, error) {
	return FindGoBinaryWithRunner(binary, nil)
}

// FindGoBinaryWithRunner looks for a Go binary using the runner's task
// environment, including GOBIN, GOPATH, HOME, and USERPROFILE.
func FindGoBinaryWithRunner(binary string, runner CommandRunner) (string, error) {
	env, _, err := commandEnvironment(runner)
	if err != nil {
		return "", err
	}
	for _, directory := range goBinaryDirectories(env) {
		for _, name := range executableNames(binary, env) {
			path := filepath.Join(directory, name)
			if isExecutableFile(path) {
				return path, nil
			}
		}
	}
	return "", fmt.Errorf("%s not found in GOBIN/GOPATH/user Go bin", binary)
}

func goBinaryDirectories(env map[string]string) []string {
	directories := make([]string, 0, 4)
	if gobin := environmentValue(env, "GOBIN"); filepath.IsAbs(gobin) {
		directories = append(directories, gobin)
	}
	for _, gopath := range filepath.SplitList(environmentValue(env, "GOPATH")) {
		if filepath.IsAbs(gopath) {
			directories = append(directories, filepath.Join(gopath, "bin"))
		}
	}
	if home := environmentValue(env, "HOME"); filepath.IsAbs(home) {
		directories = append(directories, filepath.Join(home, "go", "bin"))
	}
	if userProfile := environmentValue(env, "USERPROFILE"); filepath.IsAbs(userProfile) {
		directories = append(directories, filepath.Join(userProfile, "go", "bin"))
	}
	return directories
}
