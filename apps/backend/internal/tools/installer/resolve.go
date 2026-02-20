package installer

import (
	"context"
	"fmt"
	"os"
	"os/exec"

	"github.com/kandev/kandev/internal/common/logger"
	"go.uber.org/zap"
)

// ResolveBinary checks if a binary exists in PATH or searchPaths.
// If not found and a strategy is provided, installs it automatically.
// Returns the resolved absolute path to the binary.
func ResolveBinary(ctx context.Context, binary string, searchPaths []string, strategy Strategy, log *logger.Logger) (string, error) {
	// Check system PATH first
	if p, err := exec.LookPath(binary); err == nil {
		log.Debug("binary found in PATH", zap.String("binary", binary), zap.String("path", p))
		return p, nil
	}

	// Check each search path
	for _, dir := range searchPaths {
		p := dir
		if info, err := os.Stat(p); err == nil && !info.IsDir() {
			log.Debug("binary found in search path", zap.String("binary", binary), zap.String("path", p))
			return p, nil
		}
	}

	// Not found — try auto-install if strategy is provided
	if strategy == nil {
		return "", fmt.Errorf("%s not found in PATH or search paths", binary)
	}

	log.Info("binary not found, installing via strategy",
		zap.String("binary", binary),
		zap.String("strategy", strategy.Name()))

	result, err := strategy.Install(ctx)
	if err != nil {
		return "", fmt.Errorf("auto-install of %s failed: %w", binary, err)
	}

	log.Info("binary installed successfully",
		zap.String("binary", binary),
		zap.String("path", result.BinaryPath))

	return result.BinaryPath, nil
}
