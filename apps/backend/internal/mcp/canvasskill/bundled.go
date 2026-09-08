// Package canvasskill owns the Kandev canvas-authoring system skill.
//
// Canvas authoring is deliberately separate from the Office skill embed. The
// skill is materialized under Kandev's system-skills directory and is read by
// the canvas MCP surface without being copied into an execution workspace.
package canvasskill

import (
	"embed"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
)

const (
	// Slug is the stable system-skill identity returned by canvas MCP tools.
	Slug = "kandev-canvas-authoring"
	// Version changes when the authoring contract or its supporting references
	// change. Canvas releases keep the version in their authoring metadata.
	Version = "2"

	materializedDirectory = "system-skills"
	versionFileName       = ".kandev-version"
	maxReadableFileBytes  = 1 << 20
)

// The files directory is the complete allowlisted inventory. Do not add an
// Office skill path here: the two embeds have independent lifecycle and
// ownership rules.
//
//go:embed files
var bundledFiles embed.FS

// BundleFile is a file returned as part of the compact core bundle or used by
// the generated source scaffold. Paths are always relative to the canvas
// source directory and Content is copied before it is returned.
type BundleFile struct {
	Path    string
	Content []byte
}

var canonicalInventory = []string{
	"SKILL.md",
	"references/browser-api.md",
	"references/data-and-state.md",
	"references/events-and-recovery.md",
	"references/manifest.md",
	"references/security.md",
	"references/ui-patterns.md",
	"scaffold/appearance.js",
	"scaffold/index.html",
	"scaffold/script.js",
	"scaffold/styles.css",
}

var coreInventory = []string{
	"SKILL.md",
	"scaffold/index.html",
	"scaffold/appearance.js",
	"scaffold/script.js",
	"scaffold/styles.css",
}

var scaffoldInventory = []string{
	"manifest.yaml",
	"index.html",
	"appearance.js",
	"script.js",
	"styles.css",
}

// Inventory returns the stable, slash-separated list of files that Canvas MCP
// may read. The returned slice is a new value on every call.
func Inventory() []string {
	return append([]string(nil), canonicalInventory...)
}

// CoreInventory returns the single-read bundle in delivery order. Detailed
// references remain available through Inventory and ReadMaterialized.
func CoreInventory() []string {
	return append([]string(nil), coreInventory...)
}

// ScaffoldInventory returns the exact files materialized in an assigned
// canvas source directory. The manifest is generated from the canvas request;
// the remaining files come from the embedded templates.
func ScaffoldInventory() []string {
	return append([]string(nil), scaffoldInventory...)
}

// ScaffoldTemplateFiles returns the static files copied into every new canvas
// source directory. The returned byte slices are independent of the embed.
func ScaffoldTemplateFiles() ([]BundleFile, error) {
	files := make([]BundleFile, 0, len(coreInventory)-1)
	for _, rel := range coreInventory[1:] {
		content, err := fs.ReadFile(bundledFiles, filepath.ToSlash(filepath.Join("files", rel)))
		if err != nil {
			return nil, fmt.Errorf("read embedded canvas scaffold %q: %w", rel, err)
		}
		files = append(files, BundleFile{
			Path:    strings.TrimPrefix(rel, "scaffold/"),
			Content: append([]byte(nil), content...),
		})
	}
	return files, nil
}

// EnsureMaterialized writes the embedded skill into Kandev's system-owned
// directory. Existing files are replaced atomically. The directory is not a
// user skill directory and is never scanned by Office skill synchronization.
func EnsureMaterialized(home string) error {
	root, err := materializedRoot(home)
	if err != nil {
		return err
	}
	if err := ensureDirectory(root); err != nil {
		return err
	}

	for _, rel := range Inventory() {
		content, err := fs.ReadFile(bundledFiles, filepath.ToSlash(filepath.Join("files", rel)))
		if err != nil {
			return fmt.Errorf("read embedded canvas skill %q: %w", rel, err)
		}
		target, err := materializedFilePath(root, rel)
		if err != nil {
			return err
		}
		if err := ensureDirectory(filepath.Dir(target)); err != nil {
			return err
		}
		if err := writeAtomic(target, content, 0o644); err != nil {
			return fmt.Errorf("materialize canvas skill %q: %w", rel, err)
		}
	}

	if err := writeAtomic(filepath.Join(root, versionFileName), []byte(Version+"\n"), 0o644); err != nil {
		return fmt.Errorf("materialize canvas skill version: %w", err)
	}
	return nil
}

// MaterializedVersion returns the version marker written by
// EnsureMaterialized. An empty result means the system skill is not
// materialized or its marker cannot be read.
func MaterializedVersion(home string) string {
	root, err := materializedRoot(home)
	if err != nil {
		return ""
	}
	content, err := os.ReadFile(filepath.Join(root, versionFileName))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(content))
}

// ReadMaterialized reads one file from the current materialized inventory.
// Empty path selects SKILL.md. Absolute paths, traversal, links, and files
// outside the embedded inventory are rejected before the file is opened.
func ReadMaterialized(home, requestedPath string) ([]byte, error) {
	root, err := materializedRoot(home)
	if err != nil {
		return nil, err
	}
	rel, err := allowlistedPath(requestedPath)
	if err != nil {
		return nil, err
	}
	if !containsInventory(rel) {
		return nil, fmt.Errorf("canvas skill path %q is not allowlisted", requestedPath)
	}
	if err := ensureDirectory(root); err != nil {
		return nil, err
	}
	target, err := materializedFilePath(root, rel)
	if err != nil {
		return nil, err
	}
	info, err := os.Lstat(target)
	if err != nil {
		return nil, fmt.Errorf("read canvas skill %q: %w", rel, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("canvas skill path %q is a symlink", rel)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("canvas skill path %q is not a regular file", rel)
	}
	if info.Size() > maxReadableFileBytes {
		return nil, fmt.Errorf("canvas skill path %q exceeds the read limit", rel)
	}

	file, err := os.Open(target)
	if err != nil {
		return nil, fmt.Errorf("read canvas skill %q: %w", rel, err)
	}
	defer func() { _ = file.Close() }()
	content, err := io.ReadAll(io.LimitReader(file, maxReadableFileBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read canvas skill %q: %w", rel, err)
	}
	if len(content) > maxReadableFileBytes {
		return nil, fmt.Errorf("canvas skill path %q exceeds the read limit", rel)
	}
	return content, nil
}

func materializedRoot(home string) (string, error) {
	if strings.TrimSpace(home) == "" {
		return "", errors.New("kandev home is required for canvas skill materialization")
	}
	absHome, err := filepath.Abs(home)
	if err != nil {
		return "", fmt.Errorf("resolve Kandev home: %w", err)
	}
	return filepath.Join(absHome, materializedDirectory, Slug), nil
}

func allowlistedPath(requestedPath string) (string, error) {
	if requestedPath == "" {
		requestedPath = "SKILL.md"
	}
	if strings.IndexByte(requestedPath, 0) >= 0 || filepath.IsAbs(requestedPath) || path.IsAbs(filepath.ToSlash(requestedPath)) || filepath.VolumeName(requestedPath) != "" {
		return "", fmt.Errorf("canvas skill path %q must be relative", requestedPath)
	}
	if strings.Contains(requestedPath, `\`) {
		return "", fmt.Errorf("canvas skill path %q must use slash-separated relative paths", requestedPath)
	}
	clean := path.Clean(requestedPath)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", fmt.Errorf("canvas skill path %q contains traversal", requestedPath)
	}
	return clean, nil
}

func containsInventory(rel string) bool {
	for _, candidate := range Inventory() {
		if candidate == rel {
			return true
		}
	}
	return false
}

func materializedFilePath(root, rel string) (string, error) {
	rel, err := allowlistedPath(rel)
	if err != nil {
		return "", err
	}
	target := filepath.Join(root, filepath.FromSlash(rel))
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("resolve canvas skill root: %w", err)
	}
	targetAbs, err := filepath.Abs(target)
	if err != nil {
		return "", fmt.Errorf("resolve canvas skill path: %w", err)
	}
	relToRoot, err := filepath.Rel(rootAbs, targetAbs)
	if err != nil || relToRoot == ".." || strings.HasPrefix(relToRoot, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("canvas skill path %q escapes the skill root", rel)
	}
	return targetAbs, nil
}

func ensureDirectory(directory string) error {
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return fmt.Errorf("create canvas skill directory: %w", err)
	}
	info, err := os.Lstat(directory)
	if err != nil {
		return fmt.Errorf("inspect canvas skill directory: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("canvas skill directory %q is a symlink", directory)
	}
	if !info.IsDir() {
		return fmt.Errorf("canvas skill path %q is not a directory", directory)
	}
	return nil
}

func writeAtomic(target string, content []byte, mode os.FileMode) error {
	file, err := os.CreateTemp(filepath.Dir(target), ".canvas-skill-*")
	if err != nil {
		return err
	}
	temporary := file.Name()
	defer func() { _ = os.Remove(temporary) }()
	if err := file.Chmod(mode); err != nil {
		_ = file.Close()
		return err
	}
	if _, err := file.Write(content); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(temporary, target)
}
