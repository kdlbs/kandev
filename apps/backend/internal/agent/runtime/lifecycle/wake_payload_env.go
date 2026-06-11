package lifecycle

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"unicode"

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
	if infoDir, err := gitDirInfo(gitPath); err != nil || infoDir != "" {
		return infoDir, err
	}
	return gitWorktreeInfo(workspacePath, gitPath)
}

func gitDirInfo(gitPath string) (string, error) {
	infoPath := filepath.Join(gitPath, "info")
	st, err := os.Stat(infoPath)
	if err == nil {
		if st.IsDir() {
			return infoPath, nil
		}
		return "", nil
	}
	if os.IsNotExist(err) || errors.Is(err, syscall.ENOTDIR) {
		if fi, statErr := os.Stat(gitPath); statErr == nil && fi.IsDir() {
			return infoPath, nil
		}
		return "", nil
	}
	return "", err
}

func gitWorktreeInfo(workspacePath string, gitPath string) (string, error) {
	data, err := os.ReadFile(gitPath)
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	const prefix = "gitdir:"
	line := strings.TrimSpace(string(data))
	if !strings.HasPrefix(line, prefix) {
		return "", nil
	}
	gitDir := strings.TrimSpace(strings.TrimPrefix(line, prefix))
	if gitDir == "" {
		return "", nil
	}
	if !filepath.IsAbs(gitDir) {
		gitDir = filepath.Join(workspacePath, gitDir)
	}
	infoDir := filepath.Join(filepath.Clean(gitDir), "info")
	st, err := os.Stat(infoDir)
	switch {
	case os.IsNotExist(err):
		return infoDir, nil
	case err != nil:
		return "", err
	case !st.IsDir():
		return "", fmt.Errorf("git info path is not a directory: %s", infoDir)
	}
	return infoDir, nil
}
