package improvekandev

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/logger/buffer"
)

func TestCreateBundleDirAndWriteFiles(t *testing.T) {
	dir, err := createBundleDir()
	if err != nil {
		t.Fatalf("createBundleDir: %v", err)
	}
	defer func() { _ = os.RemoveAll(dir) }()

	if !strings.HasPrefix(filepath.Base(dir), bundlePrefix) {
		t.Errorf("bundle dir base %q must start with %q", filepath.Base(dir), bundlePrefix)
	}

	if err := writeMetadata(dir, "test", map[string]any{"github_auth": "ok"}); err != nil {
		t.Fatalf("writeMetadata: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "metadata.json"))
	if err != nil {
		t.Fatalf("read metadata.json: %v", err)
	}
	if !strings.Contains(string(data), "\"version\": \"test\"") {
		t.Errorf("metadata.json missing version: %s", data)
	}
	if !strings.Contains(string(data), "\"github_auth\": \"ok\"") {
		t.Errorf("metadata.json missing health: %s", data)
	}

	entries := []buffer.Entry{{
		Timestamp: time.Date(2026, 4, 29, 10, 0, 0, 0, time.UTC),
		Level:     "info",
		Message:   "hello world",
		Fields:    map[string]any{"foo": "bar", "n": 42},
	}}
	if err := writeBackendLog(dir, entries); err != nil {
		t.Fatalf("writeBackendLog: %v", err)
	}
	beLog, _ := os.ReadFile(filepath.Join(dir, "backend.log"))
	got := string(beLog)
	if !strings.Contains(got, "INFO") || !strings.Contains(got, "hello world") {
		t.Errorf("backend.log missing expected content: %q", got)
	}
	if !strings.Contains(got, "foo=bar") || !strings.Contains(got, "n=42") {
		t.Errorf("backend.log missing fields: %q", got)
	}

	feEntries := []FrontendLogEntry{{
		Timestamp: "2026-04-29T10:00:00.000Z",
		Level:     "error",
		Message:   "boom",
		Stack:     "Error\n    at foo",
	}}
	if err := writeFrontendLog(dir, feEntries); err != nil {
		t.Fatalf("writeFrontendLog: %v", err)
	}
	feLog, _ := os.ReadFile(filepath.Join(dir, "frontend.log"))
	if !strings.Contains(string(feLog), "ERROR") || !strings.Contains(string(feLog), "boom") {
		t.Errorf("frontend.log missing expected content: %q", feLog)
	}
}

func TestValidateBundleDir_AcceptsValid(t *testing.T) {
	dir, err := createBundleDir()
	if err != nil {
		t.Fatalf("createBundleDir: %v", err)
	}
	defer func() { _ = os.RemoveAll(dir) }()

	resolved, err := validateBundleDir(dir)
	if err != nil {
		t.Errorf("validateBundleDir(%q): %v", dir, err)
	}
	if !strings.HasPrefix(filepath.Base(resolved), bundlePrefix) {
		t.Errorf("resolved base %q must start with %q", filepath.Base(resolved), bundlePrefix)
	}
}

func TestValidateBundleDir_RejectsBad(t *testing.T) {
	cases := []struct {
		name string
		dir  string
	}{
		{"empty", ""},
		{"home", "/etc"},
		{"wrong_prefix", filepath.Join(os.TempDir(), "not-kandev")},
		{"missing", filepath.Join(os.TempDir(), "kandev-improve-doesnotexist")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := validateBundleDir(tc.dir); err == nil {
				t.Errorf("expected error for %q", tc.dir)
			}
		})
	}
}

func TestFormatBackendEntry_DeterministicFieldOrder(t *testing.T) {
	e := buffer.Entry{
		Timestamp: time.Date(2026, 4, 29, 10, 0, 0, 0, time.UTC),
		Level:     "warn",
		Message:   "msg",
		Fields:    map[string]any{"b": 2, "a": 1, "c": 3},
	}
	got := formatBackendEntry(e)
	idx := strings.Index(got, "a=1")
	if idx < 0 {
		t.Fatalf("missing a=1 in %q", got)
	}
	if !strings.Contains(got[idx:], "b=2") || !strings.Contains(got[idx:], "c=3") {
		t.Errorf("fields not alphabetically ordered: %q", got)
	}
}
