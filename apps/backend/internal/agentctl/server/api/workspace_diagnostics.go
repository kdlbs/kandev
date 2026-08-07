package api

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

const maxDiagnosticBundleBytes = int64(256 * 1024 * 1024)

var diagnosticBundleID = regexp.MustCompile(`^[a-f0-9]{16,64}$`)

func (s *Server) handleWorkspaceDiagnostics(c *gin.Context) {
	id := c.Param("id")
	if !diagnosticBundleID.MatchString(id) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid diagnostic bundle id"})
		return
	}
	root := filepath.Join(s.procMgr.WorkDir(), ".kandev", "diagnostics")
	if err := os.MkdirAll(root, 0o700); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to prepare diagnostics directory"})
		return
	}
	_ = os.Chmod(root, 0o700)
	cleanupExpiredDiagnostics(root, time.Now().Add(-24*time.Hour))
	file, err := os.CreateTemp(root, ".upload-*.tmp")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create diagnostic file"})
		return
	}
	tempPath := file.Name()
	defer func() { _ = os.Remove(tempPath) }()
	_ = file.Chmod(0o600)
	written, copyErr := io.CopyBuffer(
		file,
		io.LimitReader(c.Request.Body, maxDiagnosticBundleBytes+1),
		make([]byte, 1024*1024),
	)
	closeErr := file.Close()
	if copyErr != nil || closeErr != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to write diagnostic file"})
		return
	}
	if written > maxDiagnosticBundleBytes {
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "diagnostic bundle is too large"})
		return
	}
	destination := filepath.Join(root, id+".zip")
	if err := os.Rename(tempPath, destination); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to commit diagnostic file"})
		return
	}
	_ = os.Chmod(destination, 0o600)
	ensureDiagnosticsIgnored(s.procMgr.WorkDir())
	time.AfterFunc(24*time.Hour, func() {
		info, err := os.Lstat(destination)
		if err == nil && info.Mode().IsRegular() && time.Since(info.ModTime()) >= 24*time.Hour {
			_ = os.Remove(destination)
		}
	})
	c.JSON(http.StatusOK, gin.H{
		"path":  filepath.ToSlash(filepath.Join(".kandev", "diagnostics", id+".zip")),
		"bytes": written,
	})
}

func cleanupExpiredDiagnostics(root string, cutoff time.Time) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".zip") {
			continue
		}
		info, err := entry.Info()
		if err == nil && info.Mode().IsRegular() && info.ModTime().Before(cutoff) {
			_ = os.Remove(filepath.Join(root, entry.Name()))
		}
	}
}

func ensureDiagnosticsIgnored(workDir string) {
	excludePath, err := diagnosticsExcludePath(workDir)
	if err != nil || excludePath == "" {
		return
	}
	if err := os.MkdirAll(filepath.Dir(excludePath), 0o700); err != nil {
		return
	}
	content, err := os.ReadFile(excludePath)
	if err != nil && !os.IsNotExist(err) {
		return
	}
	if strings.Contains(string(content), "/.kandev/") {
		return
	}
	file, err := os.OpenFile(excludePath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer func() { _ = file.Close() }()
	_, _ = file.WriteString("\n/.kandev/\n")
}

func diagnosticsExcludePath(workDir string) (string, error) {
	gitPath := filepath.Join(workDir, ".git")
	info, err := os.Stat(gitPath)
	if err == nil && info.IsDir() {
		return filepath.Join(gitPath, "info", "exclude"), nil
	}
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	data, err := os.ReadFile(gitPath)
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
		gitDir = filepath.Join(workDir, gitDir)
	}
	gitDir = filepath.Clean(gitDir)
	commonData, commonErr := os.ReadFile(filepath.Join(gitDir, "commondir"))
	if commonErr == nil {
		commonDir := strings.TrimSpace(string(commonData))
		if commonDir != "" {
			if !filepath.IsAbs(commonDir) {
				commonDir = filepath.Join(gitDir, commonDir)
			}
			gitDir = filepath.Clean(commonDir)
		}
	} else if !os.IsNotExist(commonErr) {
		return "", commonErr
	}
	return filepath.Join(gitDir, "info", "exclude"), nil
}
