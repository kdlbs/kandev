package api

import (
	"archive/tar"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	pathpkg "path"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
	agentctltypes "github.com/kandev/kandev/internal/agentctl/types"
	"go.uber.org/zap"
)

const (
	// CanvasSourceTransferPath is exported from this package too so agentctl
	// clients do not need to duplicate the route string.
	CanvasSourceTransferPath = agentctltypes.CanvasSourceTransferPath
	CanvasSourceContentType  = agentctltypes.CanvasSourceContentType
)

type canvasSourceEntry struct {
	absolutePath string
	relativePath string
	info         os.FileInfo
}

type canvasSourceHTTPError struct {
	status  int
	code    string
	message string
	detail  error
}

func (e *canvasSourceHTTPError) Error() string {
	if e.detail == nil {
		return e.message
	}
	return fmt.Sprintf("%s: %v", e.message, e.detail)
}

func newCanvasSourceError(code, message string, detail error) *canvasSourceHTTPError {
	return &canvasSourceHTTPError{
		status:  http.StatusBadRequest,
		code:    code,
		message: message,
		detail:  detail,
	}
}

func newCanvasSourceLimitError(detail error) *canvasSourceHTTPError {
	return &canvasSourceHTTPError{
		status:  http.StatusRequestEntityTooLarge,
		code:    "source_limit_exceeded",
		message: "canvas source exceeds the transfer limit",
		detail:  detail,
	}
}

// handleCanvasSourceTransfer streams a bounded tar archive for one assigned
// workspace-relative source root. The route is behind agentctl's bearer-token
// middleware and instance guard in NewServer.
func (s *Server) handleCanvasSourceTransfer(c *gin.Context) {
	request, err := decodeCanvasSourceRequest(c)
	if err != nil {
		writeCanvasSourceError(c, newCanvasSourceError("invalid_request", "invalid canvas source request", err))
		return
	}

	entries, err := collectCanvasSourceEntries(s.procMgr.WorkDir(), request.Root)
	if err != nil {
		var sourceErr *canvasSourceHTTPError
		if errors.As(err, &sourceErr) {
			writeCanvasSourceError(c, sourceErr)
			return
		}
		unexpected := newCanvasSourceError("source_unavailable", "canvas source is unavailable", err)
		unexpected.status = http.StatusInternalServerError
		writeCanvasSourceError(c, unexpected)
		return
	}

	c.Header("Content-Type", CanvasSourceContentType)
	c.Header("Content-Disposition", `attachment; filename="canvas-source.tar"`)
	c.Header("Cache-Control", "no-store")
	c.Header("X-Content-Type-Options", "nosniff")
	c.Status(http.StatusOK)

	if err := streamCanvasSource(c.Request.Context(), c.Writer, entries); err != nil {
		s.logger.Warn("canvas source transfer stopped",
			zap.String("root", request.Root),
			zap.Error(err))
	}
}

func decodeCanvasSourceRequest(c *gin.Context) (agentctltypes.CanvasSourceTransferRequest, error) {
	var request agentctltypes.CanvasSourceTransferRequest
	limited := http.MaxBytesReader(c.Writer, c.Request.Body, agentctltypes.MaxCanvasSourceRequestBytes)
	decoder := json.NewDecoder(limited)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		return request, err
	}
	var extra interface{}
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return request, errors.New("request must contain one JSON object")
		}
		return request, err
	}
	if strings.TrimSpace(request.Root) == "" {
		return request, errors.New("root is required")
	}
	return request, nil
}

func writeCanvasSourceError(c *gin.Context, sourceErr *canvasSourceHTTPError) {
	if sourceErr == nil {
		sourceErr = newCanvasSourceError("source_unavailable", "canvas source is unavailable", nil)
	}
	c.JSON(sourceErr.status, agentctltypes.CanvasSourceTransferError{
		Code:    sourceErr.code,
		Message: sourceErr.message,
	})
}

func collectCanvasSourceEntries(workDir, requestedRoot string) ([]canvasSourceEntry, error) {
	root, err := resolveCanvasSourceRoot(workDir, requestedRoot)
	if err != nil {
		return nil, err
	}
	collector := &canvasSourceCollector{root: root, entries: make([]canvasSourceEntry, 0)}
	err = filepath.WalkDir(root, collector.visit)
	if err != nil {
		return nil, err
	}

	wireBytes, err := estimateCanvasSourceWireBytes(collector.entries)
	if err != nil {
		return nil, err
	}
	if wireBytes > int64(agentctltypes.MaxCanvasSourceWireBytes) {
		return nil, newCanvasSourceLimitError(errors.New("tar stream exceeds the wire limit"))
	}
	return collector.entries, nil
}

type canvasSourceCollector struct {
	root         string
	entries      []canvasSourceEntry
	totalData    int64
	regularFiles int
}

func (c *canvasSourceCollector) visit(currentPath string, entry fs.DirEntry, walkErr error) error {
	if walkErr != nil {
		return newCanvasSourceError("source_unavailable", "canvas source is unavailable", walkErr)
	}
	if currentPath == c.root {
		return nil
	}
	info, err := canvasSourceEntryInfo(currentPath, entry)
	if err != nil {
		return err
	}
	relativePath, err := filepath.Rel(c.root, currentPath)
	if err != nil || relativePath == "." {
		return newCanvasSourceError("invalid_source_path", "canvas source contains an invalid path", err)
	}
	relativePath = filepath.ToSlash(relativePath)
	if err := validateCanvasSourceRelativePath(relativePath); err != nil {
		return newCanvasSourceError("invalid_source_path", "canvas source contains an invalid path", err)
	}
	// The marker is created by create_canvas_kandev to establish the assigned
	// directory on every executor. It is host bookkeeping, not application
	// source, and must never make an otherwise valid package fail the static
	// package extension allowlist.
	if relativePath == ".canvas-root" {
		return nil
	}
	if len([]byte(relativePath)) > 240 {
		return newCanvasSourceLimitError(errors.New("source path is too long"))
	}
	return c.add(canvasSourceEntry{absolutePath: currentPath, relativePath: relativePath, info: info})
}

func canvasSourceEntryInfo(currentPath string, entry fs.DirEntry) (os.FileInfo, error) {
	if entry.Type()&os.ModeSymlink != 0 {
		return nil, newCanvasSourceError("source_contains_link", "canvas source cannot contain symlinks", errors.New(currentPath))
	}
	info, err := os.Lstat(currentPath)
	if err != nil {
		return nil, newCanvasSourceError("source_unavailable", "canvas source is unavailable", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, newCanvasSourceError("source_contains_link", "canvas source cannot contain symlinks", errors.New(currentPath))
	}
	if !info.IsDir() && !info.Mode().IsRegular() {
		return nil, newCanvasSourceError("source_contains_unsupported_entry", "canvas source contains an unsupported filesystem entry", errors.New(currentPath))
	}
	return info, nil
}

func (c *canvasSourceCollector) add(entry canvasSourceEntry) error {
	if entry.info.Mode().IsRegular() {
		if entry.info.Size() < 0 || entry.info.Size() > int64(agentctltypes.MaxCanvasSourceFileData) ||
			c.totalData > int64(agentctltypes.MaxCanvasSourceFileData)-entry.info.Size() {
			return newCanvasSourceLimitError(errors.New("source file data exceeds the limit"))
		}
		c.totalData += entry.info.Size()
		c.regularFiles++
	}
	c.entries = append(c.entries, entry)
	if len(c.entries) > agentctltypes.MaxCanvasSourceFiles*4 {
		return newCanvasSourceLimitError(errors.New("source contains too many entries"))
	}
	if c.regularFiles > agentctltypes.MaxCanvasSourceFiles {
		return newCanvasSourceLimitError(errors.New("source contains too many files"))
	}
	return nil
}

func resolveCanvasSourceRoot(workDir, requestedRoot string) (string, error) {
	cleanRoot, err := cleanCanvasSourceRoot(requestedRoot)
	if err != nil {
		return "", err
	}
	workspace, err := canonicalCanvasWorkspace(workDir)
	if err != nil {
		return "", err
	}
	relativeRoot := filepath.FromSlash(cleanRoot)
	if err := ensureNoCanvasSourceSymlinkComponents(workspace, relativeRoot); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", newCanvasSourceError("source_root_not_found", "source root does not exist", err)
		}
		return "", newCanvasSourceError("source_contains_link", "source roots and entries cannot contain symlinks", err)
	}
	root := filepath.Join(workspace, relativeRoot)
	if err := ensureCanvasSourceContainment(workspace, root); err != nil {
		return "", err
	}
	if err := validateCanvasSourceRootDirectory(root); err != nil {
		return "", err
	}
	return root, nil
}

func cleanCanvasSourceRoot(requestedRoot string) (string, error) {
	if strings.IndexByte(requestedRoot, 0) >= 0 || filepath.IsAbs(requestedRoot) || pathpkg.IsAbs(filepath.ToSlash(requestedRoot)) || filepath.VolumeName(requestedRoot) != "" || strings.Contains(requestedRoot, `\`) {
		return "", newCanvasSourceError("invalid_source_root", "root must be a workspace-relative directory", nil)
	}
	cleanRoot := pathpkg.Clean(requestedRoot)
	if cleanRoot == "." || cleanRoot == ".." || strings.HasPrefix(cleanRoot, "../") {
		return "", newCanvasSourceError("invalid_source_root", "root must remain inside the agent workspace", nil)
	}
	return cleanRoot, nil
}

func canonicalCanvasWorkspace(workDir string) (string, error) {
	workspace, err := filepath.Abs(workDir)
	if err != nil {
		return "", newCanvasSourceError("invalid_workspace", "agent workspace is unavailable", err)
	}
	workspace, err = filepath.EvalSymlinks(workspace)
	if err != nil {
		return "", newCanvasSourceError("invalid_workspace", "agent workspace is unavailable", err)
	}
	return workspace, nil
}

func ensureCanvasSourceContainment(workspace, root string) error {
	relativeCheck, err := filepath.Rel(workspace, root)
	if err != nil || relativeCheck == ".." || strings.HasPrefix(relativeCheck, ".."+string(filepath.Separator)) {
		return newCanvasSourceError("invalid_source_root", "root must remain inside the agent workspace", err)
	}
	return nil
}

func validateCanvasSourceRootDirectory(root string) error {
	info, err := os.Lstat(root)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return newCanvasSourceError("source_root_not_found", "source root does not exist", err)
		}
		return newCanvasSourceError("source_unavailable", "source root is unavailable", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return newCanvasSourceError("source_contains_link", "source roots and entries cannot contain symlinks", nil)
	}
	if !info.IsDir() {
		return newCanvasSourceError("invalid_source_root", "root must name a directory", nil)
	}
	return nil
}

func ensureNoCanvasSourceSymlinkComponents(base, relative string) error {
	current := base
	for _, component := range strings.Split(filepath.Clean(relative), string(filepath.Separator)) {
		if component == "" || component == "." {
			continue
		}
		current = filepath.Join(current, component)
		info, err := os.Lstat(current)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return errors.New(current)
		}
	}
	return nil
}

func validateCanvasSourceRelativePath(relativePath string) error {
	if relativePath == "" || relativePath == "." || pathpkg.IsAbs(relativePath) {
		return errors.New("path is not relative")
	}
	clean := pathpkg.Clean(relativePath)
	if clean != relativePath || clean == ".." || strings.HasPrefix(clean, "../") {
		return errors.New("path contains traversal")
	}
	return nil
}

func estimateCanvasSourceWireBytes(entries []canvasSourceEntry) (int64, error) {
	var total int64
	for _, entry := range entries {
		header, err := tar.FileInfoHeader(entry.info, "")
		if err != nil {
			return 0, newCanvasSourceError("source_unavailable", "canvas source is unavailable", err)
		}
		header.Name = canvasSourceTarName(entry.info, entry.relativePath)
		if entry.info.Mode().IsRegular() {
			header.Size = 0
		}
		count := &canvasSourceCountingWriter{}
		writer := tar.NewWriter(count)
		if err := writer.WriteHeader(header); err != nil {
			return 0, newCanvasSourceError("invalid_source", "canvas source cannot be archived", err)
		}
		if err := writer.Close(); err != nil {
			return 0, newCanvasSourceError("invalid_source", "canvas source cannot be archived", err)
		}
		if count.bytes < 1024 {
			return 0, newCanvasSourceError("invalid_source", "canvas source cannot be archived", errors.New("invalid tar header"))
		}
		headerBytes := count.bytes - 1024
		dataBytes := int64(0)
		if entry.info.Mode().IsRegular() {
			dataBytes = ((entry.info.Size() + 511) / 512) * 512
		}
		if total > int64(agentctltypes.MaxCanvasSourceWireBytes)-headerBytes-dataBytes {
			return 0, newCanvasSourceLimitError(errors.New("tar stream exceeds the wire limit"))
		}
		total += headerBytes + dataBytes
	}
	if total > int64(agentctltypes.MaxCanvasSourceWireBytes)-1024 {
		return 0, newCanvasSourceLimitError(errors.New("tar stream exceeds the wire limit"))
	}
	return total + 1024, nil
}

func streamCanvasSource(ctx context.Context, output io.Writer, entries []canvasSourceEntry) error {
	limited := &canvasSourceLimitedWriter{writer: output, limit: int64(agentctltypes.MaxCanvasSourceWireBytes)}
	tarWriter := tar.NewWriter(limited)
	var totalData int64
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return err
		}
		if entry.info.IsDir() {
			if err := writeCanvasSourceHeader(tarWriter, entry.info, entry.relativePath); err != nil {
				return err
			}
			continue
		}

		file, info, err := openCanvasSourceFile(entry)
		if err != nil {
			return err
		}
		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			_ = file.Close()
			return err
		}
		header.Name = canvasSourceTarName(info, entry.relativePath)
		header.Size = info.Size()
		if err := tarWriter.WriteHeader(header); err != nil {
			_ = file.Close()
			return err
		}
		if err := copyCanvasSourceFile(ctx, tarWriter, file, info.Size(), &totalData); err != nil {
			_ = file.Close()
			return err
		}
		if current, statErr := file.Stat(); statErr != nil {
			_ = file.Close()
			return statErr
		} else if current.Size() != info.Size() {
			_ = file.Close()
			return errors.New("canvas source changed during transfer")
		}
		if err := file.Close(); err != nil {
			return err
		}
	}
	if err := tarWriter.Close(); err != nil {
		return err
	}
	if flusher, ok := output.(http.Flusher); ok {
		flusher.Flush()
	}
	return nil
}

func writeCanvasSourceHeader(writer *tar.Writer, info os.FileInfo, relativePath string) error {
	header, err := tar.FileInfoHeader(info, "")
	if err != nil {
		return err
	}
	header.Name = canvasSourceTarName(info, relativePath)
	return writer.WriteHeader(header)
}

func canvasSourceTarName(info os.FileInfo, relativePath string) string {
	if info.IsDir() && !strings.HasSuffix(relativePath, "/") {
		return relativePath + "/"
	}
	return relativePath
}

func openCanvasSourceFile(entry canvasSourceEntry) (*os.File, os.FileInfo, error) {
	info, err := os.Lstat(entry.absolutePath)
	if err != nil {
		return nil, nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || !os.SameFile(info, entry.info) || info.Size() != entry.info.Size() {
		return nil, nil, errors.New("canvas source changed or contains an unsupported entry")
	}
	file, err := os.Open(entry.absolutePath)
	if err != nil {
		return nil, nil, err
	}
	openedInfo, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, nil, err
	}
	if !openedInfo.Mode().IsRegular() || !os.SameFile(openedInfo, info) {
		_ = file.Close()
		return nil, nil, errors.New("canvas source changed during transfer")
	}
	return file, openedInfo, nil
}

func copyCanvasSourceFile(ctx context.Context, writer io.Writer, file *os.File, size int64, totalData *int64) error {
	buffer := make([]byte, 32*1024)
	var copied int64
	for copied < size {
		if err := ctx.Err(); err != nil {
			return err
		}
		want := int64(len(buffer))
		if remaining := size - copied; remaining < want {
			want = remaining
		}
		read, readErr := file.Read(buffer[:want])
		if read > 0 {
			if *totalData > int64(agentctltypes.MaxCanvasSourceFileData)-int64(read) {
				return errors.New("canvas source data limit exceeded")
			}
			written, writeErr := writer.Write(buffer[:read])
			if writeErr != nil {
				return writeErr
			}
			if written != read {
				return io.ErrShortWrite
			}
			copied += int64(read)
			*totalData += int64(read)
		}
		if readErr != nil {
			if errors.Is(readErr, io.EOF) && copied == size {
				break
			}
			if errors.Is(readErr, io.EOF) {
				return io.ErrUnexpectedEOF
			}
			return readErr
		}
		if read == 0 {
			return io.ErrUnexpectedEOF
		}
	}
	return nil
}

type canvasSourceCountingWriter struct {
	bytes int64
}

func (w *canvasSourceCountingWriter) Write(data []byte) (int, error) {
	w.bytes += int64(len(data))
	return len(data), nil
}

type canvasSourceLimitedWriter struct {
	writer io.Writer
	limit  int64
	used   int64
}

func (w *canvasSourceLimitedWriter) Write(data []byte) (int, error) {
	if int64(len(data)) > w.limit-w.used {
		return 0, errors.New("canvas source wire limit exceeded")
	}
	written, err := w.writer.Write(data)
	w.used += int64(written)
	return written, err
}
