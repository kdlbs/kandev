package api

import (
	"archive/zip"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/agentctl/server/adapter/transport/shared"
)

const (
	defaultACPExportBytes = int64(96 * 1024 * 1024)
	maxACPExportBytes     = int64(96 * 1024 * 1024)
	maxACPExportFiles     = 128
)

type acpExportFile struct {
	name string
	path string
	kind string
	size int64
}

func acpDebugExportEnabled() bool {
	return os.Getenv("KANDEV_DEBUG_AGENT_MESSAGES") == "true" &&
		os.Getenv("KANDEV_DEBUG_DEV_MODE") == "true"
}

func (s *Server) handleACPDebugExport(c *gin.Context) {
	sessionID := c.Param("session")
	if !validACPExportSession(sessionID) {
		c.JSON(http.StatusNotFound, gin.H{"error": "ACP debug export unavailable"})
		return
	}
	limit := defaultACPExportBytes
	if raw := c.Query("max_bytes"); raw != "" {
		parsed, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || parsed <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid ACP export byte limit"})
			return
		}
		limit = min(parsed, maxACPExportBytes)
	}

	shared.FlushACPLogs()
	files, err := findACPExportFiles(shared.ACPLogDir(), sessionID)
	if err != nil || len(files) == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "ACP debug export unavailable"})
		return
	}

	c.Header("Content-Type", "application/zip")
	c.Header("Content-Disposition", `attachment; filename="kandev-acp-debug.zip"`)
	writer := zip.NewWriter(c.Writer)
	remaining := limit
	for _, file := range files {
		if remaining <= 0 {
			break
		}
		length := min(file.size, remaining)
		offset := file.size - length
		source, openErr := os.Open(file.path)
		if openErr != nil {
			_ = writer.Close()
			return
		}
		header := &zip.FileHeader{Name: "acp/" + file.kind + "/" + file.name, Method: zip.Store}
		header.SetMode(0o600)
		destination, createErr := writer.CreateHeader(header)
		if createErr == nil {
			createErr = copyACPSection(destination, source, offset, length)
		}
		closeErr := source.Close()
		if createErr != nil || closeErr != nil {
			_ = writer.Close()
			return
		}
		remaining -= length
	}
	if err := writer.Close(); err != nil {
		return
	}
}

func findACPExportFiles(dir, sessionID string) ([]acpExportFile, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	safeSession := sanitizeACPSessionPart(sessionID)
	files := make([]acpExportFile, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !isExactACPSessionFilename(name, safeSession) {
			continue
		}
		path := filepath.Join(dir, name)
		info, statErr := os.Lstat(path)
		if statErr != nil || !info.Mode().IsRegular() {
			continue
		}
		kind := "normalized"
		if strings.HasPrefix(name, "raw-") {
			kind = "raw"
		}
		files = append(files, acpExportFile{name: name, path: path, kind: kind, size: info.Size()})
	}
	sort.Slice(files, func(i, j int) bool {
		if files[i].kind != files[j].kind {
			return files[i].kind < files[j].kind
		}
		return files[i].name < files[j].name
	})
	if len(files) > maxACPExportFiles {
		files = files[:maxACPExportFiles]
	}
	return files, nil
}

func isExactACPSessionFilename(name, safeSession string) bool {
	if !strings.HasSuffix(name, ".jsonl") ||
		(!strings.HasPrefix(name, "raw-") && !strings.HasPrefix(name, "normalized-")) {
		return false
	}
	base := strings.TrimSuffix(name, ".jsonl")
	if dot := strings.LastIndexByte(base, '.'); dot > 0 && allASCIIDigits(base[dot+1:]) {
		base = base[:dot]
	}
	return strings.HasSuffix(base, "-"+safeSession)
}

func validACPExportSession(sessionID string) bool {
	return sessionID != "" && len(sessionID) <= 256 &&
		!strings.ContainsAny(sessionID, "/\\\x00\r\n")
}

func sanitizeACPSessionPart(value string) string {
	if value == "" {
		return "unknown"
	}
	var builder strings.Builder
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_', r == '.':
			builder.WriteRune(r)
		default:
			builder.WriteByte('_')
		}
	}
	return builder.String()
}

func allASCIIDigits(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func copyACPSection(destination io.Writer, source *os.File, offset, length int64) error {
	if _, err := source.Seek(offset, io.SeekStart); err != nil {
		return err
	}
	_, err := io.CopyN(destination, source, length)
	return err
}
