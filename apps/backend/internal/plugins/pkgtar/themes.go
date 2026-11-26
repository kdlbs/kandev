package pkgtar

import (
	"fmt"
	"io"
	"io/fs"
	"os"

	"github.com/kandev/kandev/internal/plugins/manifest"
	"github.com/kandev/kandev/internal/plugins/themes"
)

const MaxThemePackageBytes = 8 << 20

func validateThemeFiles(m *manifest.Manifest, files map[string][]byte) error {
	allowed := map[string]int{manifestFileName: 64 << 10, checksumsFileName: 64 << 10, checksumsSigFileName: 64 << 10}
	for _, name := range []string{"README", "README.md", "README.txt", "LICENSE", "LICENSE.txt", "NOTICE", "NOTICE.txt"} {
		allowed[name] = 64 << 10
	}
	for _, theme := range m.Themes {
		allowed[theme.Path] = themes.MaxDefinitionBytes
		data, ok := files[theme.Path]
		if !ok {
			return fmt.Errorf("theme_missing: %s", theme.Path)
		}
		if _, err := themes.Parse(data, theme.Base); err != nil {
			return fmt.Errorf("theme %s: %w", theme.Key, err)
		}
	}
	total := 0
	for name, data := range files {
		limit, ok := allowed[name]
		if !ok {
			return fmt.Errorf("theme_file: undeclared file %q", name)
		}
		if len(data) > limit {
			return fmt.Errorf("theme_size: %s exceeds %d bytes", name, limit)
		}
		total += len(data)
	}
	if len(files) > 128 || total > MaxThemePackageBytes {
		return fmt.Errorf("theme_size: package limit exceeded")
	}
	return nil
}

// ReadThemeDirectory revalidates persisted data before enabling or restoring it.
func ReadThemeDirectory(dir string) (*manifest.Manifest, map[string]*themes.Definition, error) {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = root.Close() }()
	files, err := readThemeDirectoryFiles(root)
	if err != nil {
		return nil, nil, err
	}
	if _, err = verifyPackageIntegrity(files); err != nil {
		return nil, nil, err
	}
	m, err := validatePackageManifest(files, false)
	if err != nil {
		return nil, nil, err
	}
	if !m.IsThemeDistribution() {
		return nil, nil, fmt.Errorf("theme_profile: theme package required")
	}
	definitions := make(map[string]*themes.Definition, len(m.Themes))
	for _, theme := range m.Themes {
		definition, err := themes.Parse(files[theme.Path], theme.Base)
		if err != nil {
			return nil, nil, err
		}
		definitions[theme.Key] = definition
	}
	return m, definitions, nil
}

func readThemeDirectoryFiles(root *os.Root) (map[string][]byte, error) {
	files := map[string][]byte{}
	total := 0
	err := fs.WalkDir(root.FS(), ".", func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("theme_file: regular files required")
		}
		if len(files) >= 128 {
			return fmt.Errorf("theme_size: too many files")
		}
		file, err := root.Open(name)
		if err != nil {
			return err
		}
		data, readErr := readCapped(file, themes.MaxDefinitionBytes)
		closeErr := file.Close()
		if readErr != nil {
			return readErr
		}
		if closeErr != nil {
			return closeErr
		}
		total += len(data)
		if total > MaxThemePackageBytes {
			return fmt.Errorf("theme_size: expanded limit exceeded")
		}
		files[name] = data
		return nil
	})
	return files, err
}

// A bounded compressed spool lets manifest discovery precede allocation of
// expanded files even when the manifest is the last archive entry.
func readProfileArchive(r io.Reader) (map[string][]byte, error) {
	spool, err := os.CreateTemp("", "kandev-package-*")
	if err != nil {
		return nil, err
	}
	defer func() { _ = spool.Close(); _ = os.Remove(spool.Name()) }()
	const compressedLimit = 200 << 20
	size, err := io.Copy(spool, io.LimitReader(r, compressedLimit+1))
	if err != nil {
		return nil, err
	}
	if size > compressedLimit {
		return nil, fmt.Errorf("pkgtar: compressed size limit exceeded")
	}
	if _, err = spool.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	m, err := Inspect(spool)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrManifestInvalid, err)
	}
	if _, err = spool.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	if m.IsThemeDistribution() {
		if size > MaxThemePackageBytes {
			return nil, fmt.Errorf("theme_size: compressed size limit exceeded")
		}
		return readArchiveLimited(spool, themes.MaxDefinitionBytes, MaxThemePackageBytes, 128)
	}
	return readArchive(spool)
}
