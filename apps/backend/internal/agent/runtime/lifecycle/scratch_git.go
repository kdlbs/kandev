package lifecycle

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	storageworkspaces "github.com/kandev/kandev/internal/system/storage/workspaces"
)

func excludeWorkspaceOwnershipMarker(gitDir string) error {
	excludePath := filepath.Join(gitDir, "info", "exclude")
	infoDir := filepath.Dir(excludePath)
	if err := ensureRealDirectory(infoDir, "git info directory"); err != nil {
		return err
	}
	if err := validateRegularFileOrMissing(excludePath, "git exclude file"); err != nil {
		return err
	}

	file, err := openGitExcludeNoFollow(gitDir)
	if err != nil {
		return fmt.Errorf("open git exclude file: %w", err)
	}
	existing, err := io.ReadAll(file)
	if err != nil {
		_ = file.Close()
		return fmt.Errorf("read git exclude file: %w", err)
	}

	entry := "/" + storageworkspaces.OwnershipMarkerFilename
	for line := range strings.SplitSeq(string(existing), "\n") {
		if strings.TrimSpace(line) == entry {
			return file.Close()
		}
	}

	prefix := ""
	if len(existing) > 0 && existing[len(existing)-1] != '\n' {
		prefix = "\n"
	}
	if _, err := file.WriteString(prefix + entry + "\n"); err != nil {
		_ = file.Close()
		return fmt.Errorf("write git exclude file: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close git exclude file: %w", err)
	}
	return nil
}

func ensureRealDirectory(path, label string) error {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		if err := os.MkdirAll(path, 0755); err != nil {
			return fmt.Errorf("create %s: %w", label, err)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect %s: %w", label, err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return fmt.Errorf("invalid %s: %s", label, path)
	}
	return nil
}

func validateRegularFileOrMissing(path, label string) error {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect %s: %w", label, err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return fmt.Errorf("invalid %s: %s", label, path)
	}
	return nil
}
