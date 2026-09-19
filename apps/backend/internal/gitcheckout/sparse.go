// Package gitcheckout applies task-owned checkout scopes before file population.
package gitcheckout

import (
	"context"
	"fmt"
	"strings"

	"github.com/kandev/kandev/internal/task/models"
)

// Run executes Git with the caller's credentials, admission, and cancellation.
type Run func(context.Context, []string, string) ([]byte, error)

func ApplySparse(ctx context.Context, path string, options *models.RepositoryCheckoutOptions, run Run) error {
	options, err := models.NormalizeRepositoryCheckoutOptions(options)
	if err != nil || options == nil || len(options.SparseDirectories) == 0 {
		return err
	}
	for _, directory := range options.SparseDirectories {
		out, err := run(ctx, []string{"-C", path, "--literal-pathspecs", "ls-tree", "-d", "-z", "HEAD", "--", directory}, "")
		if err != nil {
			return fmt.Errorf("inspect selected checkout directory: %w", err)
		}
		if !strings.HasPrefix(string(out), "040000 tree ") || !strings.HasSuffix(string(out), "\t"+directory+"\x00") {
			return &DirectoryError{Directory: directory}
		}
	}
	if err := ConfigureSparse(ctx, path, options, run); err != nil {
		return err
	}
	// A --no-checkout worktree has no populated index for sparse-checkout set
	// to update. Read HEAD only after the sparse patterns are installed.
	if _, err := run(ctx, []string{"-C", path, "read-tree", "-mu", "HEAD"}, ""); err != nil {
		return fmt.Errorf("populate selected checkout folders: %w", err)
	}
	return nil
}

// ConfigureSparse installs patterns on an empty index before a branch checkout.
func ConfigureSparse(ctx context.Context, path string, options *models.RepositoryCheckoutOptions, run Run) error {
	if options == nil || len(options.SparseDirectories) == 0 {
		return nil
	}
	_, err := run(ctx, []string{"-C", path, "sparse-checkout", "set", "--cone", "--stdin"}, SparseInput(options.SparseDirectories))
	if err != nil {
		return fmt.Errorf("configure selected checkout folders: %w", err)
	}
	return nil
}

// SparseInput uses Git's quoted pathname format, preserving quotes and Unicode.
func SparseInput(directories []string) string {
	quoted := make([]string, 0, len(directories))
	for _, directory := range directories {
		quoted = append(quoted, `"`+strings.ReplaceAll(directory, `"`, `\"`)+`"`)
	}
	return strings.Join(quoted, "\n") + "\n"
}

// DirectoryError contains only a validated repository-relative directory.
type DirectoryError struct{ Directory string }

func (e *DirectoryError) Error() string {
	return fmt.Sprintf("selected folder %q is not a directory at the selected revision", e.Directory)
}
