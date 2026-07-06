package configloader

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
)

//go:embed skills/*
var bundledSkills embed.FS

// EnsureBundledSkills writes the embedded system skills to basePath/skills/.
// Each skill is a directory containing at least a SKILL.md file.
// Existing files are overwritten to ensure the latest version is deployed.
func EnsureBundledSkills(basePath string) error {
	targetDir := filepath.Join(basePath, "skills")
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return fmt.Errorf("create skills dir: %w", err)
	}
	return fs.WalkDir(bundledSkills, "skills", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		// Compute target path relative to the "skills" root in the embed.
		rel, relErr := filepath.Rel("skills", path)
		if relErr != nil {
			return relErr
		}
		target := filepath.Join(targetDir, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		return copyEmbeddedFile(path, target)
	})
}

// copyEmbeddedFile reads a file from the embedded FS and writes it to disk.
func copyEmbeddedFile(embedPath, targetPath string) error {
	data, err := bundledSkills.ReadFile(embedPath)
	if err != nil {
		return fmt.Errorf("read embedded %s: %w", embedPath, err)
	}
	return os.WriteFile(targetPath, data, 0o644)
}

// BundledSkillSlugs returns the slugs of all bundled system skills.
func BundledSkillSlugs() ([]string, error) {
	entries, err := bundledSkills.ReadDir("skills")
	if err != nil {
		return nil, err
	}
	slugs := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			slugs = append(slugs, e.Name())
		}
	}
	return slugs, nil
}

// BundledSkillContent returns the raw SKILL.md bytes for a bundled
// skill slug. Used by the office system-skill sync so it can parse
// frontmatter directly from the embedded FS without rereading from
// disk after EnsureBundledSkills wrote them.
func BundledSkillContent(slug string) ([]byte, error) {
	return bundledSkills.ReadFile(filepath.ToSlash(filepath.Join("skills", slug, "SKILL.md")))
}

// BundledSkillFile is a non-SKILL.md file bundled next to a system skill.
type BundledSkillFile struct {
	Path    string
	Content []byte
}

// BundledSkillFiles returns supporting files embedded under a system skill
// directory. Paths are relative to the skill root and use slash separators.
func BundledSkillFiles(slug string) ([]BundledSkillFile, error) {
	root := filepath.ToSlash(filepath.Join("skills", slug))
	var files []BundledSkillFile
	err := fs.WalkDir(bundledSkills, root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}
		rel = filepath.ToSlash(rel)
		if rel == "SKILL.md" {
			return nil
		}
		content, readErr := bundledSkills.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		files = append(files, BundledSkillFile{Path: rel, Content: content})
		return nil
	})
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return files, err
}
