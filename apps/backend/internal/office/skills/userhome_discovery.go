package skills

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/kandev/kandev/internal/office/models"
)

const (
	userHomeSkillMaxDepth      = 4
	userHomeSkillMaxFiles      = 128
	userHomeSkillMaxFileBytes  = int64(1 << 20)
	userHomeSkillMaxBundleSize = int64(8 << 20)
)

// UserSkillDirResolver maps an agent provider ID to its user-home skill dir.
type UserSkillDirResolver func(provider string) (dir string, ok bool)

// UserHomeSkillFile is a snapshotted supporting file for a user-home skill.
type UserHomeSkillFile struct {
	Path    string `json:"path"`
	Size    int64  `json:"size"`
	SHA256  string `json:"sha256"`
	Content string `json:"content,omitempty"`
}

// DiscoveredUserSkill is a skill found under a provider's user skill dir.
type DiscoveredUserSkill struct {
	Provider    string              `json:"provider"`
	Key         string              `json:"key"`
	Name        string              `json:"name"`
	Description string              `json:"description"`
	SourcePath  string              `json:"source_path"`
	FileCount   int                 `json:"file_count"`
	Files       []UserHomeSkillFile `json:"files"`
}

func osUserHomeDir() (string, error) {
	return os.UserHomeDir()
}

// SetUserSkillDirResolver configures provider-specific user skill directories.
func (s *SkillService) SetUserSkillDirResolver(fn UserSkillDirResolver) {
	s.userSkillDirResolver = fn
}

// SetUserHomeResolver overrides user-home lookup, primarily for tests.
func (s *SkillService) SetUserHomeResolver(fn func() (string, error)) {
	if fn == nil {
		s.userHomeResolver = osUserHomeDir
		return
	}
	s.userHomeResolver = fn
}

// DiscoverUserSkills lists importable skills from a provider's user skill dir.
func (s *SkillService) DiscoverUserSkills(
	ctx context.Context, provider string,
) ([]DiscoveredUserSkill, error) {
	root, allowedBase, err := s.resolveUserSkillLocation(provider)
	if err != nil {
		return nil, err
	}
	return discoverUserSkillsAt(ctx, provider, root, allowedBase)
}

// ImportUserHomeSkill snapshots one discovered user-home skill into the DB.
func (s *SkillService) ImportUserHomeSkill(
	ctx context.Context, workspaceID, provider, key string,
) (*models.Skill, error) {
	root, allowedBase, err := s.resolveUserSkillLocation(provider)
	if err != nil {
		return nil, err
	}
	discovered, content, err := loadUserHomeSkill(ctx, provider, root, allowedBase, key, true)
	if err != nil {
		return nil, err
	}
	inventory, err := json.Marshal(discovered.Files)
	if err != nil {
		return nil, fmt.Errorf("encode file inventory: %w", err)
	}
	skill := &models.Skill{
		WorkspaceID:   workspaceID,
		Name:          discovered.Name,
		Slug:          GenerateSlug(discovered.Key),
		Description:   discovered.Description,
		SourceType:    SkillSourceTypeUserHome,
		SourceLocator: userHomeSourceLocator(provider, key),
		Content:       content,
		FileInventory: string(inventory),
	}
	existing, err := s.findUserHomeSkill(ctx, workspaceID, provider, key)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		skill.ID = existing.ID
		skill.Slug = existing.Slug
		skill.CreatedAt = existing.CreatedAt
		if err := s.UpdateSkill(ctx, skill); err != nil {
			return nil, err
		}
		return skill, nil
	}
	if err := s.ValidateAndPrepareSkill(ctx, skill); err != nil {
		return nil, err
	}
	if err := s.CreateSkill(ctx, skill); err != nil {
		return nil, err
	}
	return skill, nil
}

func (s *SkillService) resolveUserSkillLocation(provider string) (root, allowedBase string, err error) {
	provider = strings.TrimSpace(provider)
	if provider == "" {
		return "", "", fmt.Errorf("provider is required")
	}
	if s.userSkillDirResolver == nil {
		return "", "", fmt.Errorf("user skill discovery is not configured")
	}
	dir, ok := s.userSkillDirResolver(provider)
	if !ok || dir == "" {
		return "", "", fmt.Errorf("user skill discovery is not supported for provider %q", provider)
	}
	home, err := s.userHomeResolver()
	if err != nil {
		return "", "", fmt.Errorf("resolve user home: %w", err)
	}
	if filepath.IsAbs(dir) {
		clean := filepath.Clean(dir)
		return clean, clean, nil
	}
	return filepath.Join(home, filepath.FromSlash(dir)), home, nil
}

func (s *SkillService) findUserHomeSkill(
	ctx context.Context, workspaceID, provider, key string,
) (*models.Skill, error) {
	skills, err := s.repo.ListSkills(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	locator := userHomeSourceLocator(provider, key)
	for _, skill := range skills {
		if skill.SourceType == SkillSourceTypeUserHome && skill.SourceLocator == locator {
			return skill, nil
		}
	}
	return nil, nil
}

func userHomeSourceLocator(provider, key string) string {
	return strings.TrimSpace(provider) + ":" + strings.Trim(strings.TrimSpace(key), "/")
}

// DiscoverUserSkillsAt scans an already resolved provider user skill root.
func DiscoverUserSkillsAt(
	ctx context.Context, provider, root string,
) ([]DiscoveredUserSkill, error) {
	return discoverUserSkillsAt(ctx, provider, root, root)
}

func discoverUserSkillsAt(
	ctx context.Context, provider, root, allowedBase string,
) ([]DiscoveredUserSkill, error) {
	if _, err := os.Stat(root); err != nil {
		if os.IsNotExist(err) {
			return []DiscoveredUserSkill{}, nil
		}
		return nil, err
	}
	roots, err := findUserSkillDirs(ctx, root, allowedBase)
	if err != nil {
		return nil, err
	}
	out := make([]DiscoveredUserSkill, 0, len(roots))
	for _, skillRoot := range roots {
		key, err := filepath.Rel(root, skillRoot)
		if err != nil {
			return nil, err
		}
		discovered, _, err := loadUserHomeSkill(ctx, provider, root, allowedBase, filepath.ToSlash(key), false)
		if err != nil {
			return nil, err
		}
		out = append(out, discovered)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out, nil
}

func findUserSkillDirs(ctx context.Context, root, allowedBase string) ([]string, error) {
	var found []string
	visited := map[string]bool{}
	if err := walkUserSkillDirs(ctx, allowedBase, root, 0, visited, &found); err != nil {
		return nil, err
	}
	return found, nil
}

func walkUserSkillDirs(
	ctx context.Context,
	allowedBase, dir string,
	depth int,
	visited map[string]bool,
	found *[]string,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	realDir, err := filepath.EvalSymlinks(dir)
	if err == nil {
		if !isWithinDir(realDir, allowedBase) {
			return nil
		}
		if visited[realDir] {
			return nil
		}
		visited[realDir] = true
	}
	if _, err := os.Stat(filepath.Join(dir, "SKILL.md")); err == nil {
		*found = append(*found, dir)
		return nil
	}
	if depth >= userHomeSkillMaxDepth {
		return nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
			if err := walkUserSkillDirs(ctx, allowedBase, filepath.Join(dir, entry.Name()), depth+1, visited, found); err != nil {
				return err
			}
		}
	}
	return nil
}

func loadUserHomeSkill(
	ctx context.Context,
	provider, root, allowedBase, key string,
	includeContent bool,
) (DiscoveredUserSkill, string, error) {
	key = strings.Trim(strings.TrimSpace(key), "/")
	if key == "" || strings.Contains(key, "..") {
		return DiscoveredUserSkill{}, "", fmt.Errorf("invalid skill key: %q", key)
	}
	skillRoot := filepath.Join(root, filepath.FromSlash(key))
	if !isWithinDir(skillRoot, allowedBase) {
		return DiscoveredUserSkill{}, "", fmt.Errorf("skill key escapes allowed provider directory")
	}
	content, err := readLimitedFile(filepath.Join(skillRoot, "SKILL.md"), userHomeSkillMaxFileBytes)
	if err != nil {
		return DiscoveredUserSkill{}, "", fmt.Errorf("read SKILL.md: %w", err)
	}
	name, desc := ParseFrontmatter(string(content))
	if name == "" {
		name = filepath.Base(skillRoot)
	}
	files, err := collectUserSkillFiles(ctx, skillRoot, includeContent)
	if err != nil {
		return DiscoveredUserSkill{}, "", err
	}
	return DiscoveredUserSkill{
		Provider:    provider,
		Key:         filepath.ToSlash(key),
		Name:        name,
		Description: desc,
		SourcePath:  skillRoot,
		FileCount:   len(files) + 1,
		Files:       files,
	}, string(content), nil
}

func collectUserSkillFiles(
	ctx context.Context, skillRoot string, includeContent bool,
) ([]UserHomeSkillFile, error) {
	var (
		files []UserHomeSkillFile
		total int64
	)
	err := filepath.WalkDir(skillRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		rel, err := filepath.Rel(skillRoot, path)
		if err != nil {
			return err
		}
		if filepath.ToSlash(rel) == "SKILL.md" {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if len(files) >= userHomeSkillMaxFiles {
			return fmt.Errorf("skill bundle has too many files")
		}
		if info.Size() > userHomeSkillMaxFileBytes {
			return fmt.Errorf("skill file %s exceeds max size", rel)
		}
		total += info.Size()
		if total > userHomeSkillMaxBundleSize {
			return fmt.Errorf("skill bundle exceeds max size")
		}
		file, err := snapshotUserSkillFile(path, filepath.ToSlash(rel), info.Size(), includeContent)
		if err != nil {
			return err
		}
		files = append(files, file)
		return nil
	})
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return files, err
}

func snapshotUserSkillFile(path, rel string, size int64, includeContent bool) (UserHomeSkillFile, error) {
	content, err := readLimitedFile(path, userHomeSkillMaxFileBytes)
	if err != nil {
		return UserHomeSkillFile{}, err
	}
	sum := sha256.Sum256(content)
	file := UserHomeSkillFile{
		Path:   rel,
		Size:   size,
		SHA256: hex.EncodeToString(sum[:]),
	}
	if includeContent {
		file.Content = string(content)
	}
	return file, nil
}

func readLimitedFile(path string, maxBytes int64) ([]byte, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if info.Size() > maxBytes {
		return nil, fmt.Errorf("file exceeds max size")
	}
	return os.ReadFile(path)
}

func isWithinDir(path, base string) bool {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	absBase, err := filepath.Abs(base)
	if err != nil {
		return false
	}
	if realPath, err := filepath.EvalSymlinks(absPath); err == nil {
		absPath = realPath
	}
	if realBase, err := filepath.EvalSymlinks(absBase); err == nil {
		absBase = realBase
	}
	rel, err := filepath.Rel(absBase, absPath)
	if err != nil {
		return false
	}
	return rel == "." || !strings.HasPrefix(rel, ".."+string(os.PathSeparator)) && rel != ".."
}
