package service

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/task/models"
)

func uploadDocumentAttachment(t *testing.T, svc *DocumentService, base, key, filename, data string) *models.TaskDocument {
	t.Helper()
	doc, err := svc.UploadAttachment(context.Background(), "task-doc", key, filename, "application/octet-stream", []byte(data), base)
	require.NoError(t, err)
	return doc
}

func assertDocumentAttachmentBytes(t *testing.T, svc *DocumentService, key, expected string) string {
	t.Helper()
	path, _, err := svc.DownloadAttachment(context.Background(), "task-doc", key)
	require.NoError(t, err)
	assertAttachmentFileBytes(t, path, expected)
	return path
}

func assertAttachmentFileBytes(t *testing.T, path, expected string) {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, expected, string(data))
}

// @covers AC-TASKS-DOCUMENTS-002.1
func TestDocumentAttachmentFailedReplacementPreservesPublished(t *testing.T) {
	for _, failure := range []string{"lookup", "update"} {
		for _, filename := range []string{"replacement.bin", "replacement.pdf"} {
			t.Run(failure+"/"+filename, func(t *testing.T) {
				svc, repo := newDocumentTestService(t)
				base := t.TempDir()
				uploadDocumentAttachment(t, svc, base, "artifact", "original.bin", "original published bytes")
				ctx := context.Background()
				before, err := repo.GetDocument(ctx, "task-doc", "artifact")
				require.NoError(t, err)
				boom := errors.New("reject replacement")
				wrapped := &stubDocRepo{docRepo: repo}
				if failure == "lookup" {
					wrapped.getDocErr = boom
				} else {
					wrapped.updateErr = boom
				}
				failing := NewDocumentService(wrapped, accessTestLogger(t))
				_, err = failing.UploadAttachment(ctx, "task-doc", "artifact", filename, "application/pdf", []byte("replacement bytes"), base)
				require.ErrorIs(t, err, boom)
				after, err := repo.GetDocument(ctx, "task-doc", "artifact")
				require.NoError(t, err)
				require.Equal(t, before, after)
				assertDocumentAttachmentBytes(t, svc, "artifact", "original published bytes")
				entries, err := os.ReadDir(filepath.Join(base, "attachments", "task-doc"))
				require.NoError(t, err)
				if failure == "lookup" {
					require.Len(t, entries, 1)
				} else {
					require.Len(t, entries, 2)
					for _, entry := range entries {
						path := filepath.Join(base, "attachments", "task-doc", entry.Name())
						if path != before.DiskPath {
							assertAttachmentFileBytes(t, path, "replacement bytes")
						}
					}
				}
			})
		}
	}
}

type faultDocumentAttachmentFile struct {
	*os.File
	fault  string
	err    error
	closed bool
}

func (f *faultDocumentAttachmentFile) Write(data []byte) (int, error) {
	if f.fault == "write" || f.fault == "short-write" {
		n, err := f.File.Write(data[:1])
		if err != nil {
			return n, err
		}
		if f.fault == "write" {
			return n, f.err
		}
		return n, nil
	}
	return f.File.Write(data)
}

func (f *faultDocumentAttachmentFile) Close() error {
	f.closed = true
	if err := f.File.Close(); err != nil {
		return err
	}
	if f.fault == "close" {
		return f.err
	}
	return nil
}

// @covers AC-TASKS-DOCUMENTS-002.1
func TestDocumentAttachmentPreparationFailures(t *testing.T) {
	for _, fault := range []string{"directory", "create", "write", "short-write", "close"} {
		t.Run(fault, func(t *testing.T) {
			svc, repo := newDocumentTestService(t)
			base := t.TempDir()
			uploadDocumentAttachment(t, svc, base, "artifact", "first.bin", "published bytes")
			before, err := repo.GetDocument(context.Background(), "task-doc", "artifact")
			require.NoError(t, err)
			boom := errors.New("preparation failure")
			var candidate *faultDocumentAttachmentFile
			if fault == "directory" {
				base = t.TempDir()
				require.NoError(t, os.WriteFile(filepath.Join(base, "attachments"), []byte("directory blocker"), 0o600))
			} else {
				svc.createAttachmentFile = func(dir string) (documentAttachmentFile, error) {
					if fault == "create" {
						return nil, boom
					}
					file, err := os.CreateTemp(dir, ".document-attachment-")
					if err != nil {
						return nil, err
					}
					t.Cleanup(func() { _ = file.Close() })
					candidate = &faultDocumentAttachmentFile{File: file, fault: fault, err: boom}
					return candidate, nil
				}
			}
			_, err = svc.UploadAttachment(context.Background(), "task-doc", "artifact", "failed.bin", "application/octet-stream", []byte("replacement bytes"), base)
			require.Error(t, err)
			if fault == "short-write" {
				require.ErrorIs(t, err, io.ErrShortWrite)
			} else if fault != "directory" {
				require.ErrorIs(t, err, boom)
			}
			if candidate != nil {
				require.True(t, candidate.closed)
				_, err := os.Stat(candidate.Name())
				require.ErrorIs(t, err, os.ErrNotExist)
			}
			after, err := repo.GetDocument(context.Background(), "task-doc", "artifact")
			require.NoError(t, err)
			require.Equal(t, before, after)
			assertDocumentAttachmentBytes(t, svc, "artifact", "published bytes")
		})
	}
}

// @covers AC-TASKS-DOCUMENTS-002.4
func TestDocumentAttachmentCleanupIsolation(t *testing.T) {
	svc, repo := newDocumentTestService(t)
	base := t.TempDir()
	first := uploadDocumentAttachment(t, svc, base, "artifact", "first.bin", "first bytes")
	other := uploadDocumentAttachment(t, svc, base, "other", "other.bin", "other bytes")
	independentSvc := NewDocumentService(repo, accessTestLogger(t))
	boom := errors.New("write rejected after independent publication")
	var candidate *faultDocumentAttachmentFile
	svc.createAttachmentFile = func(dir string) (documentAttachmentFile, error) {
		file, err := os.CreateTemp(dir, ".document-attachment-")
		if err != nil {
			return nil, err
		}
		t.Cleanup(func() { _ = file.Close() })
		candidate = &faultDocumentAttachmentFile{File: file, fault: "write", err: boom}
		uploadDocumentAttachment(t, independentSvc, base, "artifact", "independent.bin", "independent bytes")
		return candidate, nil
	}
	_, err := svc.UploadAttachment(context.Background(), "task-doc", "artifact", "failed.bin", "application/octet-stream", []byte("failed bytes"), base)
	require.ErrorIs(t, err, boom)
	require.True(t, candidate.closed)
	_, err = os.Stat(candidate.Name())
	require.ErrorIs(t, err, os.ErrNotExist)
	assertDocumentAttachmentBytes(t, independentSvc, "artifact", "independent bytes")
	assertAttachmentFileBytes(t, first.DiskPath, "first bytes")
	assertAttachmentFileBytes(t, other.DiskPath, "other bytes")
}

// @covers AC-TASKS-DOCUMENTS-002.2
func TestDocumentAttachmentFirstPublication(t *testing.T) {
	for _, failure := range []string{"none", "lookup", "create", "missing-task"} {
		t.Run(failure, func(t *testing.T) {
			svc, repo := newDocumentTestService(t)
			base := t.TempDir()
			wrapped := &stubDocRepo{docRepo: repo}
			boom := errors.New("reject first upload")
			taskID := "task-doc"
			switch failure {
			case "lookup":
				wrapped.getDocErr = boom
			case "create":
				wrapped.createErr = boom
			case "missing-task":
				taskID = "absent-task"
			}
			failing := NewDocumentService(wrapped, accessTestLogger(t))
			doc, err := failing.UploadAttachment(context.Background(), taskID, "artifact", "first.bin", "application/octet-stream", []byte("complete first bytes"), base)
			if failure == "none" {
				require.NoError(t, err)
				require.Equal(t, "first.bin", doc.Filename)
				require.EqualValues(t, len("complete first bytes"), doc.SizeBytes)
				assertDocumentAttachmentBytes(t, svc, "artifact", "complete first bytes")
				return
			}
			require.Error(t, err)
			if failure != "missing-task" {
				require.ErrorIs(t, err, boom)
			}
			row, err := repo.GetDocument(context.Background(), taskID, "artifact")
			require.NoError(t, err)
			require.Nil(t, row)
			_, _, err = svc.DownloadAttachment(context.Background(), taskID, "artifact")
			require.ErrorIs(t, err, ErrDocumentNotFound)
			entries, err := os.ReadDir(filepath.Join(base, "attachments", taskID))
			if failure == "lookup" {
				require.True(t, errors.Is(err, os.ErrNotExist) || len(entries) == 0)
			} else {
				require.NoError(t, err)
				require.Len(t, entries, 1)
				assertAttachmentFileBytes(t, filepath.Join(base, "attachments", taskID, entries[0].Name()), "complete first bytes")
			}
		})
	}
}

// @covers AC-TASKS-DOCUMENTS-002.3
func TestDocumentAttachmentSuccessfulReplacement(t *testing.T) {
	for _, filename := range []string{"second.bin", "second.pdf"} {
		t.Run(filename, func(t *testing.T) {
			svc, repo := newDocumentTestService(t)
			base := t.TempDir()
			first := uploadDocumentAttachment(t, svc, base, "artifact", "first.bin", "original")
			second := uploadDocumentAttachment(t, svc, base, "artifact", filename, "complete replacement")
			require.Equal(t, first.ID, second.ID)
			require.Equal(t, first.CreatedAt, second.CreatedAt)
			require.NotEqual(t, first.DiskPath, second.DiskPath)
			require.Equal(t, filepath.Join(base, "attachments", "task-doc"), filepath.Dir(second.DiskPath))
			require.Equal(t, filename, second.Filename)
			require.EqualValues(t, len("complete replacement"), second.SizeBytes)
			assertDocumentAttachmentBytes(t, svc, "artifact", "complete replacement")
			assertAttachmentFileBytes(t, first.DiskPath, "original")
			docs, err := repo.ListDocuments(context.Background(), "task-doc")
			require.NoError(t, err)
			require.Len(t, docs, 1)
			revisions, err := repo.ListDocumentRevisions(context.Background(), "task-doc", "artifact", 10)
			require.NoError(t, err)
			require.Empty(t, revisions)
		})
	}
}

// @covers AC-TASKS-DOCUMENTS-002.3
func TestDocumentAttachmentLegacyDownloadAndDelete(t *testing.T) {
	svc, repo := newDocumentTestService(t)
	dir := filepath.Join(t.TempDir(), "attachments", "task-doc")
	require.NoError(t, os.MkdirAll(dir, 0o750))
	path := filepath.Join(dir, "legacy.bin")
	require.NoError(t, os.WriteFile(path, []byte("legacy bytes"), 0o600))
	require.NoError(t, repo.CreateDocument(context.Background(), &models.TaskDocument{
		TaskID: "task-doc", Key: "legacy", Type: docTypeAttachment, Filename: "legacy.bin", DiskPath: path,
	}))
	assertDocumentAttachmentBytes(t, svc, "legacy", "legacy bytes")
	require.NoError(t, svc.DeleteDocument(context.Background(), "task-doc", "legacy"))
	_, _, err := svc.DownloadAttachment(context.Background(), "task-doc", "legacy")
	require.ErrorIs(t, err, ErrDocumentNotFound)
	assertAttachmentFileBytes(t, path, "legacy bytes")
}

type documentCommitErrorRepo struct {
	docRepo
	afterCommit func(*models.TaskDocument)
	err         error
}

func (r *documentCommitErrorRepo) UpdateDocument(ctx context.Context, doc *models.TaskDocument) error {
	if err := r.docRepo.UpdateDocument(ctx, doc); err != nil {
		return err
	}
	r.afterCommit(doc)
	return r.err
}

// @covers AC-TASKS-DOCUMENTS-002.4
func TestDocumentAttachmentMetadataOutcomeUncertain(t *testing.T) {
	for _, next := range []string{"none", "overwrite", "delete"} {
		t.Run(next, func(t *testing.T) {
			svc, repo := newDocumentTestService(t)
			base := t.TempDir()
			first := uploadDocumentAttachment(t, svc, base, "artifact", "first.bin", "first bytes")
			other := uploadDocumentAttachment(t, svc, base, "other", "other.bin", "other key bytes")
			var resolved, independent string
			boom := errors.New("error after real commit")
			wrapped := &documentCommitErrorRepo{docRepo: repo, err: boom, afterCommit: func(_ *models.TaskDocument) {
				resolved = assertDocumentAttachmentBytes(t, svc, "artifact", "briefly published")
				switch next {
				case "overwrite":
					independent = uploadDocumentAttachment(t, svc, base, "artifact", "independent.bin", "independent bytes").DiskPath
				case "delete":
					require.NoError(t, svc.DeleteDocument(context.Background(), "task-doc", "artifact"))
				}
			}}
			failing := NewDocumentService(wrapped, accessTestLogger(t))
			_, err := failing.UploadAttachment(context.Background(), "task-doc", "artifact", "candidate.bin", "application/octet-stream", []byte("briefly published"), base)
			require.ErrorIs(t, err, boom)
			assertAttachmentFileBytes(t, resolved, "briefly published")
			assertAttachmentFileBytes(t, first.DiskPath, "first bytes")
			assertAttachmentFileBytes(t, other.DiskPath, "other key bytes")
			switch next {
			case "overwrite":
				require.Equal(t, independent, assertDocumentAttachmentBytes(t, svc, "artifact", "independent bytes"))
			case "delete":
				_, _, err = svc.DownloadAttachment(context.Background(), "task-doc", "artifact")
				require.ErrorIs(t, err, ErrDocumentNotFound)
			default:
				require.Equal(t, resolved, assertDocumentAttachmentBytes(t, svc, "artifact", "briefly published"))
			}
		})
	}
}
