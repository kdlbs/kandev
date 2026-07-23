package installer

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/kandev/kandev/internal/common/logger"
	"go.uber.org/zap"
)

// NpmStrategy installs tools via npm.
type NpmStrategy struct {
	binDir   string   // install prefix directory
	binary   string   // e.g. "typescript-language-server"
	packages []string // e.g. ["typescript-language-server", "typescript"]
	logger   *logger.Logger
	runner   CommandRunner
}

// NewNpmStrategy creates a new npm install strategy.
func NewNpmStrategy(binDir, binary string, packages []string, log *logger.Logger, runners ...CommandRunner) *NpmStrategy {
	strategy := &NpmStrategy{
		binDir:   binDir,
		binary:   binary,
		packages: packages,
		logger:   log,
	}
	if len(runners) > 0 {
		strategy.runner = runners[0]
	}
	return strategy
}

func (s *NpmStrategy) Name() string {
	return fmt.Sprintf("npm install %s", strings.Join(s.packages, " "))
}

func (s *NpmStrategy) Install(ctx context.Context) (*InstallResult, error) {
	npmPath, err := exec.LookPath("npm")
	if err != nil {
		return nil, fmt.Errorf("npm not found: %w", err)
	}

	// Ensure bin directory exists
	if err := os.MkdirAll(s.binDir, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create directory %s: %w", s.binDir, err)
	}

	args := append([]string{installSubcommand, "--prefix", s.binDir}, s.packages...)
	s.logger.Info("installing via npm", zap.Strings("packages", s.packages), zap.String("prefix", s.binDir))

	output, err := combinedOutput(ctx, s.runner, CommandSpec{Path: npmPath, Args: args, Dir: s.binDir})
	if err != nil {
		return nil, fmt.Errorf("npm install failed: %w\nOutput: %s", err, string(output))
	}

	binaryPath := filepath.Join(s.binDir, "node_modules", ".bin", s.binary)
	if _, err := os.Stat(binaryPath); err != nil {
		return nil, fmt.Errorf("binary not found after install: %s", binaryPath)
	}

	s.logger.Info("npm install completed", zap.String("binary", binaryPath))
	return &InstallResult{
		BinaryPath: binaryPath,
	}, nil
}
