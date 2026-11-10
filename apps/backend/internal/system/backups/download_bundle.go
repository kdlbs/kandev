package backups

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// OpenForDownloadBundle streams one complete backup artifact. Retention
// backups with a checkpoint sidecar are delivered as a zip containing the
// database image and history sources; legacy snapshots stay plain .db files.
func (s *Service) OpenForDownloadBundle(name string) (io.ReadCloser, int64, string, string, error) {
	path, err := s.resolveSnapshotPath(name)
	if err != nil {
		return nil, 0, "", "", err
	}
	if s.RetentionSources == nil || !s.RetentionSources.HasRetentionBackup(path) {
		file, size, openErr := s.OpenForDownload(name)
		if openErr != nil {
			return nil, 0, "", "", openErr
		}
		return file, size, name, "application/octet-stream", nil
	}
	return zipBackupArtifact(path, name)
}

type cleanupReadCloser struct {
	file    *os.File
	cleanup func()
}

func (r *cleanupReadCloser) Read(data []byte) (int, error) { return r.file.Read(data) }

func (r *cleanupReadCloser) Close() error {
	err := r.file.Close()
	if r.cleanup != nil {
		r.cleanup()
	}
	return err
}

func zipBackupArtifact(path, name string) (io.ReadCloser, int64, string, string, error) {
	file, err := os.CreateTemp("", "kandev-backup-*.zip")
	if err != nil {
		return nil, 0, "", "", err
	}
	cleanup := func() { _ = os.Remove(file.Name()) }
	writer := zip.NewWriter(file)
	if err := addZipFile(writer, filepath.Base(path), path); err != nil {
		_ = writer.Close()
		_ = file.Close()
		cleanup()
		return nil, 0, "", "", err
	}
	historyDir := path + ".history"
	if err := addZipTree(writer, historyDir); err != nil {
		_ = writer.Close()
		_ = file.Close()
		cleanup()
		return nil, 0, "", "", err
	}
	if err := writer.Close(); err != nil {
		_ = file.Close()
		cleanup()
		return nil, 0, "", "", err
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		cleanup()
		return nil, 0, "", "", err
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		_ = file.Close()
		cleanup()
		return nil, 0, "", "", err
	}
	return &cleanupReadCloser{file: file, cleanup: cleanup}, info.Size(), name + ".zip", "application/zip", nil
}

func addZipTree(writer *zip.Writer, root string) error {
	return filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		return addZipFile(writer, filepath.Join("history", relative), path)
	})
}

func addZipFile(writer *zip.Writer, name, path string) error {
	entry, err := writer.Create(name)
	if err != nil {
		return fmt.Errorf("create backup bundle entry: %w", err)
	}
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()
	_, err = io.Copy(entry, file)
	return err
}
