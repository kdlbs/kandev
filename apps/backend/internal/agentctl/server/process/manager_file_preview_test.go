package process

import (
	"bytes"
	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/common/logger"
	"os"
	"path/filepath"
	"testing"
)

func TestFilePreviewManagerReadAndProtectedFiles(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"model.stl", "private.stl"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(name), 0600); err != nil {
			t.Fatal(err)
		}
	}
	m := NewManager(&config.InstanceConfig{WorkDir: dir}, logger.Default())
	t.Cleanup(func() { _ = m.StopForTeardown(t.Context()) })
	info, err := m.PrepareFilePreview(t.Context(), "model.stl", "")
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := m.CopyFilePreview(t.Context(), info.Token, &out); err != nil {
		t.Fatal(err)
	}
	if out.String() != "model.stl" {
		t.Fatal("wrong selected bytes")
	}
	tracker := m.GetWorkspaceTracker()
	tracker.SetRecoveryArtifactExclusions([]string{filepath.Join(dir, "private.stl")})
	if _, err := m.PrepareFilePreview(t.Context(), "private.stl", ""); err == nil {
		t.Fatal("protected file admitted")
	}
	info, err = m.PrepareFilePreview(t.Context(), "model.stl", "")
	if err != nil {
		t.Fatal(err)
	}
	tracker.SetRecoveryArtifactExclusions([]string{filepath.Join(dir, "model.stl"), filepath.Join(dir, "private.stl")})
	out.Reset()
	if err := m.CopyFilePreview(t.Context(), info.Token, &out); err == nil || out.Len() != 0 {
		t.Fatal("exclusions bypassed after preparation")
	}
}

func TestFilePreviewIgnoresMissingUnrelatedExclusions(t *testing.T) {
	dir := t.TempDir()
	model := filepath.Join(dir, "model.stl")
	if err := os.WriteFile(model, []byte("model"), 0600); err != nil {
		t.Fatal(err)
	}
	m := NewManager(&config.InstanceConfig{WorkDir: dir}, logger.Default())
	t.Cleanup(func() { _ = m.StopForTeardown(t.Context()) })
	m.GetWorkspaceTracker().SetRecoveryArtifactExclusions([]string{filepath.Join(dir, "deleted", "recovery.stl")})
	info, err := m.PrepareFilePreview(t.Context(), "model.stl", "")
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := m.CopyFilePreview(t.Context(), info.Token, &out); err != nil {
		t.Fatal(err)
	}
	if out.String() != "model" {
		t.Fatal("wrong selected bytes")
	}
}

func TestFilePreviewStillRejectsExcludedSymlinkTarget(t *testing.T) {
	dir := t.TempDir()
	private := filepath.Join(t.TempDir(), "private.stl")
	if err := os.WriteFile(private, []byte("private"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(private, filepath.Join(dir, "recovery.stl")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	m := NewManager(&config.InstanceConfig{WorkDir: dir}, logger.Default())
	t.Cleanup(func() { _ = m.StopForTeardown(t.Context()) })
	info, err := m.PrepareFilePreview(t.Context(), private, "")
	if err != nil {
		t.Fatal(err)
	}
	m.CancelFilePreview(info.Token)
	m.GetWorkspaceTracker().SetRecoveryArtifactExclusions([]string{
		filepath.Join(dir, "deleted.stl"),
		filepath.Join(dir, "recovery.stl"),
	})
	if _, err := m.PrepareFilePreview(t.Context(), private, ""); err == nil {
		t.Fatal("excluded symlink target admitted")
	}
}
