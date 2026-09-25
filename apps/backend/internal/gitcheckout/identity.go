package gitcheckout

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/kandev/kandev/internal/common/gitref"
	"github.com/kandev/kandev/internal/task/models"
)

const identityFile = "kandev-checkout-options.json"

func Save(path string, options *models.RepositoryCheckoutOptions) error {
	if options == nil {
		return nil
	}
	dir, err := gitref.ResolveGitDir(path)
	if err != nil {
		return err
	}
	data, err := json.Marshal(options)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, identityFile), data, 0600)
}

// Check preserves user work: an existing checkout may only be reused with its original scope.
func Check(path string, options *models.RepositoryCheckoutOptions) error {
	if options == nil {
		options = &models.RepositoryCheckoutOptions{Version: 1, DownloadMode: models.DownloadStandard, SparseDirectories: []string{}}
	}
	dir, err := gitref.ResolveGitDir(path)
	if err != nil {
		return err
	}
	data, err := os.ReadFile(filepath.Join(dir, identityFile))
	if os.IsNotExist(err) && options.DownloadMode == models.DownloadStandard && len(options.SparseDirectories) == 0 {
		return nil
	}
	if err != nil {
		return fmt.Errorf("checkout options identity is unavailable: %w", err)
	}
	expected, err := json.Marshal(options)
	if err != nil {
		return err
	}
	if !bytes.Equal(data, expected) {
		return fmt.Errorf("checkout options differ from the existing workspace; create a new task")
	}
	return nil
}
