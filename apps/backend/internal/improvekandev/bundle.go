// Package improvekandev exposes the HTTP endpoints that bootstrap a hidden
// improve-kandev workflow and capture recent backend/frontend logs into a
// temporary bundle directory referenced by the resulting task.
package improvekandev

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/common/logger/buffer"
)

// bundlePrefix is the temp-dir prefix used by os.MkdirTemp.
// It is also the marker validateBundleDir uses to refuse arbitrary writes.
const bundlePrefix = "kandev-improve-"

// FrontendLogEntry mirrors apps/web/lib/logger/buffer.ts shape.
type FrontendLogEntry struct {
	Timestamp string         `json:"timestamp"`
	Level     string         `json:"level"`
	Message   string         `json:"message"`
	Args      []any          `json:"args,omitempty"`
	Stack     string         `json:"stack,omitempty"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}

// metadata is the JSON payload written to <bundle>/metadata.json.
type metadata struct {
	Version    string         `json:"version"`
	OS         string         `json:"os"`
	Arch       string         `json:"arch"`
	GoVersion  string         `json:"go_version"`
	CapturedAt time.Time      `json:"captured_at"`
	Health     map[string]any `json:"health,omitempty"`
}

// createBundleDir creates a fresh temp directory matching bundlePrefix.
func createBundleDir() (string, error) {
	return os.MkdirTemp("", bundlePrefix+"*")
}

// writeMetadata writes metadata.json into the bundle directory.
// health may be nil if no health snapshot is available.
func writeMetadata(dir, version string, health map[string]any) error {
	m := metadata{
		Version:    version,
		OS:         runtime.GOOS,
		Arch:       runtime.GOARCH,
		GoVersion:  runtime.Version(),
		CapturedAt: time.Now().UTC(),
		Health:     health,
	}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal metadata: %w", err)
	}
	return os.WriteFile(filepath.Join(dir, "metadata.json"), data, 0o644)
}

// writeBackendLog renders the buffer snapshot as plain text and writes it.
func writeBackendLog(dir string, entries []buffer.Entry) error {
	var b strings.Builder
	for _, e := range entries {
		b.WriteString(formatBackendEntry(e))
		b.WriteByte('\n')
	}
	return os.WriteFile(filepath.Join(dir, "backend.log"), []byte(b.String()), 0o644)
}

// writeFrontendLog renders frontend entries as plain text and writes them.
func writeFrontendLog(dir string, entries []FrontendLogEntry) error {
	var b strings.Builder
	for _, e := range entries {
		b.WriteString(formatFrontendEntry(e))
		b.WriteByte('\n')
	}
	return os.WriteFile(filepath.Join(dir, "frontend.log"), []byte(b.String()), 0o644)
}

// formatBackendEntry renders a single buffer.Entry as a single line of text.
func formatBackendEntry(e buffer.Entry) string {
	ts := e.Timestamp.UTC().Format(time.RFC3339Nano)
	level := strings.ToUpper(e.Level)
	var b strings.Builder
	fmt.Fprintf(&b, "%s\t%s\t%s", ts, level, e.Message)
	if e.Caller != "" {
		fmt.Fprintf(&b, "\t%s", e.Caller)
	}
	if len(e.Fields) > 0 {
		fmt.Fprintf(&b, "\t%s", flattenFields(e.Fields))
	}
	if e.Stack != "" {
		fmt.Fprintf(&b, "\n%s", e.Stack)
	}
	return b.String()
}

// formatFrontendEntry mirrors formatBackendEntry for frontend entries.
func formatFrontendEntry(e FrontendLogEntry) string {
	level := strings.ToUpper(e.Level)
	var b strings.Builder
	fmt.Fprintf(&b, "%s\t%s\t%s", e.Timestamp, level, e.Message)
	if len(e.Args) > 0 {
		fmt.Fprintf(&b, "\targs=%v", e.Args)
	}
	if e.Stack != "" {
		fmt.Fprintf(&b, "\n%s", e.Stack)
	}
	return b.String()
}

// flattenFields renders structured fields as space-separated key=value pairs in
// alphabetical order so log lines are deterministic across runs.
func flattenFields(fields map[string]any) string {
	keys := make([]string, 0, len(fields))
	for k := range fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for i, k := range keys {
		if i > 0 {
			b.WriteByte(' ')
		}
		fmt.Fprintf(&b, "%s=%v", k, fields[k])
	}
	return b.String()
}

// staleBundleAge is the minimum age before a leftover bundle dir is removed
// by CleanupStaleBundles. Recent dirs are preserved in case a task is still
// being created and references them.
const staleBundleAge = 24 * time.Hour

// CleanupStaleBundles removes kandev-improve-* directories under the OS temp
// dir that are older than staleBundleAge. Errors are logged via onError but
// do not abort the sweep. Safe to call concurrently with active bundles —
// fresh dirs are skipped by the mtime check.
func CleanupStaleBundles(onError func(path string, err error)) {
	cleanupStaleBundlesIn(os.TempDir(), staleBundleAge, onError)
}

func cleanupStaleBundlesIn(tempDir string, maxAge time.Duration, onError func(path string, err error)) {
	entries, err := os.ReadDir(tempDir)
	if err != nil {
		if onError != nil {
			onError(tempDir, err)
		}
		return
	}
	cutoff := time.Now().Add(-maxAge)
	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), bundlePrefix) {
			continue
		}
		path := filepath.Join(tempDir, entry.Name())
		info, err := entry.Info()
		if err != nil {
			if onError != nil {
				onError(path, err)
			}
			continue
		}
		if info.ModTime().After(cutoff) {
			continue
		}
		if err := os.RemoveAll(path); err != nil && onError != nil {
			onError(path, err)
		}
	}
}

// validateBundleDir refuses paths that do not match the kandev-improve-* temp
// pattern under the OS temp dir. Returns the cleaned absolute path on success.
func validateBundleDir(dir string) (string, error) {
	if dir == "" {
		return "", errors.New("bundle_dir is required")
	}
	clean := filepath.Clean(dir)
	abs, err := filepath.Abs(clean)
	if err != nil {
		return "", fmt.Errorf("invalid bundle_dir: %w", err)
	}
	tempDir, err := filepath.EvalSymlinks(os.TempDir())
	if err != nil {
		tempDir = os.TempDir()
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		resolved = abs
	}
	if !strings.HasPrefix(resolved, tempDir+string(os.PathSeparator)) {
		return "", errors.New("bundle_dir must be inside the OS temp directory")
	}
	if !strings.HasPrefix(filepath.Base(resolved), bundlePrefix) {
		return "", errors.New("bundle_dir name must start with " + bundlePrefix)
	}
	info, err := os.Stat(resolved)
	if err != nil || !info.IsDir() {
		return "", errors.New("bundle_dir does not exist or is not a directory")
	}
	return resolved, nil
}
