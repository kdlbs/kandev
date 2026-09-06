package lifecycle

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/kandev/kandev/internal/worktree"
	"go.uber.org/zap"
)

const (
	envWakePayloadJSON       = "KANDEV_WAKE_PAYLOAD_JSON"
	envWakePayloadPath       = "KANDEV_WAKE_PAYLOAD_PATH"
	envWakePayloadInlineMax  = 64 * 1024
	wakePayloadDirRel        = ".kandev/wake-payloads"
	wakePayloadExcludeLine   = ".kandev/wake-payloads/"
	defaultWakePayloadFileID = "payload"
)

func spillLargeWakePayloadEnv(env map[string]string, workspacePath string, log *zap.Logger) error {
	payload := env[envWakePayloadJSON]
	if payload == "" || len(payload) <= envWakePayloadInlineMax {
		return nil
	}
	fileID := sanitizeWakePayloadFileID(env["KANDEV_RUN_ID"])
	if fileID == defaultWakePayloadFileID && log != nil {
		log.Warn("KANDEV_RUN_ID is missing or empty; spill file may collide across runs",
			zap.String("payload_path", filepath.Join(wakePayloadDirRel, fileID+".json")))
	}
	if workspacePath == "" {
		return fmt.Errorf("%s is %d bytes, above %d byte inline limit, but workspace path is empty",
			envWakePayloadJSON, len(payload), envWakePayloadInlineMax)
	}

	relPath := filepath.ToSlash(filepath.Join(wakePayloadDirRel, fileID+".json"))
	absPath := filepath.Join(workspacePath, filepath.FromSlash(relPath))
	if err := os.MkdirAll(filepath.Dir(absPath), 0o700); err != nil {
		return fmt.Errorf("create wake payload directory: %w", err)
	}
	if err := os.WriteFile(absPath, []byte(payload), 0o600); err != nil {
		_ = os.Remove(absPath)
		return fmt.Errorf("write wake payload file: %w", err)
	}

	delete(env, envWakePayloadJSON)
	env[envWakePayloadPath] = relPath

	if err := ensureWakePayloadGitExclude(workspacePath); err != nil && log != nil {
		log.Warn("failed to update git exclude for wake payload spill file",
			zap.String("workspace_path", workspacePath),
			zap.Error(err))
	}
	if log != nil {
		log.Info("spilled oversized wake payload env to workspace file",
			zap.Int("payload_bytes", len(payload)),
			zap.String("payload_path", relPath))
	}
	return nil
}

func sanitizeWakePayloadFileID(id string) string {
	id = strings.TrimSpace(id)
	if id == "" {
		return defaultWakePayloadFileID
	}
	var b strings.Builder
	for _, r := range id {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_' {
			b.WriteRune(r)
		}
	}
	if b.Len() == 0 {
		return defaultWakePayloadFileID
	}
	return b.String()
}

func ensureWakePayloadGitExclude(workspacePath string) error {
	infoDir, err := gitInfoDir(workspacePath)
	if err != nil {
		return err
	}
	if infoDir == "" {
		return nil
	}
	if err := os.MkdirAll(infoDir, 0o700); err != nil {
		return err
	}
	excludePath := filepath.Join(infoDir, "exclude")
	data, err := os.ReadFile(excludePath)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if strings.Contains(string(data), wakePayloadExcludeLine) {
		return nil
	}
	prefix := ""
	if len(data) > 0 && !strings.HasSuffix(string(data), "\n") {
		prefix = "\n"
	}
	return os.WriteFile(excludePath, append(data, []byte(prefix+wakePayloadExcludeLine+"\n")...), 0o600)
}

func gitInfoDir(workspacePath string) (string, error) {
	gitPath := filepath.Join(workspacePath, ".git")
	if _, err := os.Lstat(gitPath); os.IsNotExist(err) {
		return "", nil
	} else if err != nil {
		return "", err
	}
	projection, err := worktree.ResolveGitMetadata(workspacePath)
	if err != nil {
		return "", err
	}
	return filepath.Join(projection.GitDir, "info"), nil
}
