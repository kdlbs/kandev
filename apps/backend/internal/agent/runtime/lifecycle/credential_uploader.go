package lifecycle

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/remoteauth"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/common/subproc"
)

// FileUploader abstracts writing files to a remote environment. Used by
// UploadCredentialFiles to seed agent auth files into the kandev-managed
// per-container session dir (local) or sprite (remote).
type FileUploader interface {
	WriteFile(ctx context.Context, path string, data []byte, mode os.FileMode) error
}

// UploadCredentialFiles reads local credential files and uploads them to the remote environment.
func UploadCredentialFiles(
	ctx context.Context,
	uploader FileUploader,
	methods []remoteauth.Method,
	targetHomeDir string,
	log *logger.Logger,
) error {
	if len(methods) == 0 {
		return nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}

	for _, method := range methods {
		if method.Type != "files" {
			continue
		}

		for _, relPath := range method.SourceFiles {
			srcPath := filepath.Join(home, relPath)
			data, readErr := os.ReadFile(srcPath)
			if readErr != nil {
				log.Warn("credential source file not found, skipping",
					zap.String("method_id", method.MethodID),
					zap.String("path", srcPath))
				continue
			}

			targetPath := filepath.Join(targetHomeDir, method.TargetRelDir, filepath.Base(relPath))
			if err := uploader.WriteFile(ctx, targetPath, data, 0o644); err != nil {
				return fmt.Errorf("failed to upload %s: %w", targetPath, err)
			}
			log.Debug("uploaded credential file",
				zap.String("method_id", method.MethodID),
				zap.String("target", targetPath))
		}
	}

	return nil
}

// DetectGHToken runs `gh auth token` locally and returns the GitHub OAuth token.
//
// Uses two contexts so a saturated gh throttle can't silently disable token
// injection: acquireCtx (30s) bounds waiting for a slot; execCtx (5s) bounds
// the gh subprocess itself. Without this split, a busy gh pool would
// consume the 5s budget queueing and `gh auth token` would never start.
func DetectGHToken() (string, error) {
	acquireCtx, cancelAcquire := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancelAcquire()
	execCtx, cancelExec := context.WithTimeout(acquireCtx, 5*time.Second)
	defer cancelExec()
	cmd := exec.CommandContext(execCtx, "gh", "auth", "token")
	out, err := subproc.RunGHOutput(acquireCtx, cmd)
	if err != nil {
		return "", fmt.Errorf("gh auth token failed: %w", err)
	}
	token := strings.TrimSpace(string(out))
	if token == "" {
		return "", fmt.Errorf("gh auth token returned empty")
	}
	return token, nil
}
