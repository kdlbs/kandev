package backups

import (
	"archive/zip"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestRetentionDownloadBundleIncludesHistorySidecar(t *testing.T) {
	s, dir := newTestService(t)
	snapshotPath := filepath.Join(dir, "backups", "manual-test.db")
	if err := os.MkdirAll(filepath.Dir(snapshotPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(snapshotPath, []byte("database image"), 0o600); err != nil {
		t.Fatal(err)
	}
	historyDir := snapshotPath + ".history"
	if err := os.MkdirAll(historyDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(historyDir, "manifest.json"), []byte(`{"version":2}`), 0o600); err != nil {
		t.Fatal(err)
	}
	s.RetentionSources = &fakeRetentionSources{has: true}
	reader, size, filename, contentType, err := s.OpenForDownloadBundle("manual-test.db")
	if err != nil {
		t.Fatalf("open backup bundle: %v", err)
	}
	if size <= 0 || filename != "manual-test.db.zip" || contentType != "application/zip" {
		t.Fatalf("bundle metadata = size %d name %q type %q", size, filename, contentType)
	}
	path := filepath.Join(t.TempDir(), "bundle.zip")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(file, reader); err != nil {
		t.Fatal(err)
	}
	_ = file.Close()
	_ = reader.Close()
	archive, err := zip.OpenReader(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = archive.Close() }()
	names := map[string]bool{}
	for _, entry := range archive.File {
		names[entry.Name] = true
	}
	if !names["manual-test.db"] || !names["history/manifest.json"] {
		t.Fatalf("bundle entries = %v", names)
	}
}
