package repoclone

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/kandev/kandev/internal/task/models"
)

type managedAttachmentContextKey struct{}
type attachmentAncestor struct {
	path string
	info os.FileInfo
}
type managedAttachmentGuard struct {
	repository models.Repository
	ancestors  []attachmentAncestor
}

// ManagedAttachmentContext restricts existing clone lanes to one admitted current destination.
func (c *Cloner) ManagedAttachmentContext(ctx context.Context, repository *models.Repository) (context.Context, error) {
	if _, err := c.InspectManagedAttachmentPath(ctx, repository, repository.LocalPath); err != nil {
		return nil, err
	}
	guard := &managedAttachmentGuard{repository: *repository}
	path, err := c.managedAttachmentDestination(repository)
	if err != nil {
		return nil, err
	}
	for current := filepath.Clean(path); ; current = filepath.Dir(current) {
		info, err := os.Lstat(current)
		if err == nil {
			guard.ancestors = append(guard.ancestors, attachmentAncestor{path: current, info: info})
		} else if !os.IsNotExist(err) {
			return nil, err
		}
		if filepath.Dir(current) == current {
			break
		}
	}
	return context.WithValue(ctx, managedAttachmentContextKey{}, guard), nil
}

// validateManagedAttachmentMutation runs while the existing per-path clone mutex is held.
func (c *Cloner) validateManagedAttachmentMutation(ctx context.Context, path string) error {
	guard, ok := ctx.Value(managedAttachmentContextKey{}).(*managedAttachmentGuard)
	if !ok {
		return nil
	}
	for _, ancestor := range guard.ancestors {
		current, err := os.Lstat(ancestor.path)
		if err != nil {
			return err
		}
		if !os.SameFile(ancestor.info, current) {
			return fmt.Errorf("managed attachment path identity changed")
		}
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	_, err = c.InspectManagedAttachmentPath(ctx, &guard.repository, absolute)
	return err
}
