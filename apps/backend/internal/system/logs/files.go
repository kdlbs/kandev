// Package logs serves the System -> Logs page: lists log files on disk
// (current + lumberjack-rotated backups), tails the most recent N lines of
// the active log file, and streams individual files for download.
package logs

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
)

// FileInfo describes one log file on disk.
type FileInfo struct {
	Name    string    `json:"name"`
	Size    int64     `json:"size"`
	Mtime   time.Time `json:"mtime"`
	Current bool      `json:"current"`
}

// Service enumerates and serves files from the configured log directory.
// The active (un-rotated) file is identified by exact name match against
// currentName (set to the lumberjack Filename's basename, e.g. "kandev.log").
type Service struct {
	logDir      string
	currentName string
	log         *logger.Logger
}

// NewService constructs a Service. logDir may be empty (logger is writing to
// stdout/stderr); List then returns an empty slice and Tail returns no lines.
// currentName is the basename of the active log file (e.g. "kandev.log").
func NewService(logDir, currentName string, log *logger.Logger) *Service {
	return &Service{
		logDir:      logDir,
		currentName: currentName,
		log:         log,
	}
}

// List returns the log files in the configured directory, sorted by mtime
// descending (newest first). The active file is flagged with Current=true.
//
// Subdirectories are skipped. A missing log directory yields an empty slice
// without error (the logger may be configured for stdout-only output).
func (s *Service) List() ([]FileInfo, error) {
	out := []FileInfo{}
	if s.logDir == "" {
		return out, nil
	}

	entries, err := os.ReadDir(s.logDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return out, nil
		}
		s.warn("read log directory failed", zap.String("dir", s.logDir), zap.Error(err))
		return nil, err
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		fi, statErr := entry.Info()
		if statErr != nil {
			s.warn("stat log entry failed", zap.String("name", entry.Name()), zap.Error(statErr))
			continue
		}
		if !fi.Mode().IsRegular() {
			continue
		}
		name := entry.Name()
		out = append(out, FileInfo{
			Name:    name,
			Size:    fi.Size(),
			Mtime:   fi.ModTime(),
			Current: s.isCurrent(name),
		})
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].Mtime.After(out[j].Mtime)
	})
	return out, nil
}

// isCurrent reports whether name is the active (un-rotated) log file.
// Lumberjack rotates "kandev.log" to "kandev-<timestamp>.log[.gz]", so an
// exact basename match is enough to distinguish current from rotated.
func (s *Service) isCurrent(name string) bool {
	return s.currentName != "" && name == s.currentName
}

// Open returns an *os.File for download, plus its size. name must be a bare
// filename inside the log directory — any path separator, traversal segment,
// or empty value is rejected. Caller closes the returned file.
func (s *Service) Open(name string) (*os.File, int64, error) {
	if s.logDir == "" {
		return nil, 0, errors.New("logs: log directory not configured")
	}
	clean, err := s.safeName(name)
	if err != nil {
		return nil, 0, err
	}
	full, err := s.containedPath(clean)
	if err != nil {
		return nil, 0, err
	}
	// Reject symlinks and other non-regular files before opening so an
	// adversarial link planted in the log directory cannot escape it.
	lstat, err := os.Lstat(full)
	if err != nil {
		return nil, 0, err
	}
	if !lstat.Mode().IsRegular() {
		return nil, 0, errors.New("logs: not a regular file")
	}
	f, err := os.Open(full)
	if err != nil {
		return nil, 0, err
	}
	fi, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, 0, err
	}
	return f, fi.Size(), nil
}

// containedPath joins the (already-safeName-validated) clean filename to the
// configured log directory and verifies that the resolved absolute path is
// still a child of the log directory. This is belt-and-braces alongside
// safeName and gives CodeQL's taint-tracking a syntactic sanitizer it
// recognises ("go/path-injection").
func (s *Service) containedPath(clean string) (string, error) {
	root, err := filepath.Abs(s.logDir)
	if err != nil {
		return "", fmt.Errorf("logs: resolve log dir: %w", err)
	}
	joined, err := filepath.Abs(filepath.Join(root, clean))
	if err != nil {
		return "", fmt.Errorf("logs: resolve filename: %w", err)
	}
	rel, err := filepath.Rel(root, joined)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", errors.New("logs: resolved path escapes the log directory")
	}
	return joined, nil
}

// safeName rejects any name that resolves outside the log directory.
func (s *Service) safeName(name string) (string, error) {
	if name == "" {
		return "", errors.New("logs: empty filename")
	}
	if strings.ContainsAny(name, `/\`) {
		return "", errors.New("logs: path separators not allowed")
	}
	if name == "." || name == ".." {
		return "", errors.New("logs: invalid filename")
	}
	// filepath.Clean of a single segment without separators is the identity
	// for legitimate filenames; anything that changes under Clean is unsafe.
	if filepath.Clean(name) != name {
		return "", errors.New("logs: invalid filename")
	}
	return name, nil
}

func (s *Service) warn(msg string, fields ...zap.Field) {
	if s.log == nil {
		return
	}
	s.log.Warn(msg, fields...)
}
