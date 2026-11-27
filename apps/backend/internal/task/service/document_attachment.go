package service

import (
	"context"
	"fmt"
	"io"
	"os"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/task/models"
)

type documentAttachmentFile interface {
	io.WriteCloser
	Name() string
}

func createDocumentAttachmentFile(dir string) (documentAttachmentFile, error) {
	return os.CreateTemp(dir, ".document-attachment-")
}

func (s *DocumentService) prepareAttachmentFile(dir string, data []byte) (string, error) {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return "", fmt.Errorf("create attachment dir: %w", err)
	}
	file, err := s.createAttachmentFile(dir)
	if err != nil {
		return "", fmt.Errorf("create attachment file: %w", err)
	}
	path := file.Name()
	n, writeErr := file.Write(data)
	if writeErr == nil && n != len(data) {
		writeErr = io.ErrShortWrite
	}
	closeErr := file.Close()
	if writeErr != nil || closeErr != nil {
		if err := os.Remove(path); err != nil {
			s.logger.Warn("remove unpublished attachment candidate", zap.Error(err))
		}
		if writeErr != nil {
			return "", fmt.Errorf("write attachment file: %w", writeErr)
		}
		return "", fmt.Errorf("close attachment file: %w", closeErr)
	}
	return path, nil
}

func (s *DocumentService) publishAttachment(ctx context.Context, head *models.TaskDocument, create bool) error {
	if create {
		if err := s.repo.CreateDocument(ctx, head); err != nil {
			return fmt.Errorf("create attachment document: %w", err)
		}
	} else if err := s.repo.UpdateDocument(ctx, head); err != nil {
		return fmt.Errorf("update attachment document: %w", err)
	}
	return nil
}
