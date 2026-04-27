package service

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/orchestrate/models"
)

const sourceTypeLocalPath = "local_path"

// SkillImportResult holds the outcome of an import operation.
type SkillImportResult struct {
	Skills   []*models.Skill
	Warnings []string
}

// GitHubFetcher abstracts HTTP fetching for testability.
type GitHubFetcher interface {
	Fetch(ctx context.Context, url string) ([]byte, int, error)
}

// httpFetcher is the default production fetcher using net/http.
type httpFetcher struct {
	client *http.Client
}

func newHTTPFetcher() *httpFetcher {
	return &httpFetcher{client: &http.Client{Timeout: 15 * time.Second}}
}

func (f *httpFetcher) Fetch(ctx context.Context, url string) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, 0, err
	}
	resp, err := f.client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20)) // 1 MB limit
	if err != nil {
		return nil, resp.StatusCode, err
	}
	return body, resp.StatusCode, nil
}

// ParsedSource holds the result of parsing a source string.
type ParsedSource struct {
	Owner      string
	Repo       string
	Slug       string
	SourceType string // "git" or "skills_sh" or "local_path"
	LocalPath  string
	IsLocal    bool
}

// orgRepoSlugRe matches "org/repo/skill" or "org/repo" without protocol.
var orgRepoSlugRe = regexp.MustCompile(`^([a-zA-Z0-9_.-]+)/([a-zA-Z0-9_.-]+)(?:/([a-zA-Z0-9_.-]+))?$`)

// ParseSource classifies a source string into its type and components.
func ParseSource(source string) (*ParsedSource, error) {
	source = strings.TrimSpace(source)
	if source == "" {
		return nil, fmt.Errorf("source is required")
	}
	if strings.HasPrefix(source, "/") || strings.HasPrefix(source, "./") {
		return &ParsedSource{IsLocal: true, LocalPath: source, SourceType: sourceTypeLocalPath}, nil
	}
	if strings.HasPrefix(source, "https://skills.sh/") {
		return parseSkillsShURL(source)
	}
	if strings.HasPrefix(source, "https://github.com/") {
		return parseGitHubURL(source)
	}
	if m := orgRepoSlugRe.FindStringSubmatch(source); m != nil {
		return &ParsedSource{
			Owner: m[1], Repo: m[2], Slug: m[3],
			SourceType: "git",
		}, nil
	}
	return nil, fmt.Errorf("unrecognized source format: %q", source)
}

func parseSkillsShURL(source string) (*ParsedSource, error) {
	trimmed := strings.TrimPrefix(source, "https://skills.sh/")
	parts := strings.SplitN(trimmed, "/", 3)
	if len(parts) < 2 {
		return nil, fmt.Errorf("invalid skills.sh URL: %q (need org/repo)", source)
	}
	slug := ""
	if len(parts) == 3 && parts[2] != "" {
		slug = strings.TrimSuffix(parts[2], "/")
	}
	return &ParsedSource{
		Owner: parts[0], Repo: parts[1], Slug: slug,
		SourceType: "skills_sh",
	}, nil
}

func parseGitHubURL(source string) (*ParsedSource, error) {
	trimmed := strings.TrimPrefix(source, "https://github.com/")
	trimmed = strings.TrimSuffix(trimmed, "/")
	// Handle tree paths like org/repo/tree/main/skills/slug
	parts := strings.SplitN(trimmed, "/", 6)
	if len(parts) < 2 {
		return nil, fmt.Errorf("invalid GitHub URL: %q", source)
	}
	ps := &ParsedSource{Owner: parts[0], Repo: parts[1], SourceType: "git"}
	if len(parts) >= 6 && parts[2] == "tree" {
		// org/repo/tree/branch/skills/slug
		ps.Slug = parts[5]
	}
	return ps, nil
}

// ParseFrontmatter extracts name and description from YAML frontmatter in SKILL.md.
func ParseFrontmatter(content string) (name, description string) {
	scanner := bufio.NewScanner(strings.NewReader(content))
	inFrontmatter := false
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "---" {
			if inFrontmatter {
				break // end of frontmatter
			}
			inFrontmatter = true
			continue
		}
		if !inFrontmatter {
			continue
		}
		if k, v := parseYAMLLine(line); k != "" {
			switch k {
			case "name":
				name = v
			case "description":
				description = v
			}
		}
	}
	return name, description
}

func parseYAMLLine(line string) (key, value string) {
	idx := strings.Index(line, ":")
	if idx < 0 {
		return "", ""
	}
	key = strings.TrimSpace(line[:idx])
	value = strings.TrimSpace(line[idx+1:])
	// Strip surrounding quotes.
	if len(value) >= 2 {
		if (value[0] == '"' && value[len(value)-1] == '"') ||
			(value[0] == '\'' && value[len(value)-1] == '\'') {
			value = value[1 : len(value)-1]
		}
	}
	return key, value
}

// ImportFromSource imports one or more skills from an external source.
func (s *Service) ImportFromSource(
	ctx context.Context, workspaceID, source string, fetcher GitHubFetcher,
) (*SkillImportResult, error) {
	if fetcher == nil {
		fetcher = newHTTPFetcher()
	}
	ps, err := ParseSource(source)
	if err != nil {
		return nil, err
	}
	if ps.IsLocal {
		return s.importLocal(ctx, workspaceID, ps.LocalPath)
	}
	return s.importFromGitHub(ctx, workspaceID, ps, fetcher)
}

func (s *Service) importLocal(ctx context.Context, wsID, localPath string) (*SkillImportResult, error) {
	abs, err := filepath.Abs(localPath)
	if err != nil {
		return nil, fmt.Errorf("invalid local path: %w", err)
	}
	allowedRoot := filepath.Clean(s.cfgLoader.BasePath()) + string(os.PathSeparator)
	if !strings.HasPrefix(abs+string(os.PathSeparator), allowedRoot) {
		return nil, fmt.Errorf("local path is outside workspace root")
	}
	skillMD := filepath.Join(abs, "SKILL.md")
	content, err := os.ReadFile(skillMD)
	if err != nil {
		return nil, fmt.Errorf("reading SKILL.md from %s: %w", abs, err)
	}
	name, desc := ParseFrontmatter(string(content))
	slug := filepath.Base(abs)
	if name == "" {
		name = slug
	}
	skill := &models.Skill{
		WorkspaceID:   wsID,
		Name:          name,
		Slug:          GenerateSlug(slug),
		Description:   desc,
		SourceType:    sourceTypeLocalPath,
		SourceLocator: abs,
		Content:       string(content),
	}
	if err := s.createImportedSkill(ctx, skill); err != nil {
		return nil, err
	}
	return &SkillImportResult{Skills: []*models.Skill{skill}}, nil
}

func (s *Service) importFromGitHub(
	ctx context.Context, wsID string, ps *ParsedSource, fetcher GitHubFetcher,
) (*SkillImportResult, error) {
	if ps.Slug != "" {
		return s.importSingleSkill(ctx, wsID, ps, fetcher)
	}
	return s.importRepoSkills(ctx, wsID, ps, fetcher)
}

func (s *Service) importSingleSkill(
	ctx context.Context, wsID string, ps *ParsedSource, fetcher GitHubFetcher,
) (*SkillImportResult, error) {
	content, err := fetchSkillMD(ctx, ps.Owner, ps.Repo, ps.Slug, fetcher)
	if err != nil {
		return nil, err
	}
	name, desc := ParseFrontmatter(content)
	if name == "" {
		name = ps.Slug
	}
	locator := buildSourceLocator(ps)
	skill := &models.Skill{
		WorkspaceID:   wsID,
		Name:          name,
		Slug:          GenerateSlug(ps.Slug),
		Description:   desc,
		SourceType:    ps.SourceType,
		SourceLocator: locator,
		Content:       content,
	}
	if err := s.createImportedSkill(ctx, skill); err != nil {
		return nil, err
	}
	return &SkillImportResult{Skills: []*models.Skill{skill}}, nil
}

func (s *Service) importRepoSkills(
	ctx context.Context, wsID string, ps *ParsedSource, fetcher GitHubFetcher,
) (*SkillImportResult, error) {
	// Try to discover skills by fetching repo tree via GitHub API.
	slugs, err := discoverSkillSlugs(ctx, ps.Owner, ps.Repo, fetcher)
	if err != nil {
		return nil, fmt.Errorf("discovering skills in %s/%s: %w", ps.Owner, ps.Repo, err)
	}
	if len(slugs) == 0 {
		return nil, fmt.Errorf("no skills found in %s/%s", ps.Owner, ps.Repo)
	}
	result := &SkillImportResult{}
	for _, slug := range slugs {
		sps := &ParsedSource{
			Owner: ps.Owner, Repo: ps.Repo, Slug: slug,
			SourceType: ps.SourceType,
		}
		r, err := s.importSingleSkill(ctx, wsID, sps, fetcher)
		if err != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("failed to import %s: %v", slug, err))
			continue
		}
		result.Skills = append(result.Skills, r.Skills...)
	}
	return result, nil
}

func (s *Service) createImportedSkill(ctx context.Context, skill *models.Skill) error {
	if err := s.ValidateAndPrepareSkill(ctx, skill); err != nil {
		return err
	}
	return s.CreateSkill(ctx, skill)
}

func buildSourceLocator(ps *ParsedSource) string {
	if ps.SourceType == "skills_sh" {
		return fmt.Sprintf("https://skills.sh/%s/%s/%s", ps.Owner, ps.Repo, ps.Slug)
	}
	return fmt.Sprintf("https://github.com/%s/%s", ps.Owner, ps.Repo)
}

func fetchSkillMD(
	ctx context.Context, owner, repo, slug string, fetcher GitHubFetcher,
) (string, error) {
	for _, branch := range []string{"main", "master"} {
		url := fmt.Sprintf(
			"https://raw.githubusercontent.com/%s/%s/%s/skills/%s/SKILL.md",
			owner, repo, branch, slug,
		)
		body, status, err := fetcher.Fetch(ctx, url)
		if err != nil {
			continue
		}
		if status == http.StatusOK {
			return string(body), nil
		}
	}
	return "", fmt.Errorf("SKILL.md not found for %s/%s/skills/%s", owner, repo, slug)
}

// discoverSkillSlugs finds skills/*/SKILL.md by fetching a known index approach.
// We try fetching the repo tree for skills/ directory via raw content listing.
func discoverSkillSlugs(
	ctx context.Context, owner, repo string, fetcher GitHubFetcher,
) ([]string, error) {
	// Use the GitHub API to list the skills directory tree.
	url := fmt.Sprintf(
		"https://api.github.com/repos/%s/%s/contents/skills", owner, repo,
	)
	body, status, err := fetcher.Fetch(ctx, url)
	if err != nil || status != http.StatusOK {
		return nil, fmt.Errorf("could not list skills directory (status %d)", status)
	}
	return parseGitHubDirListing(string(body)), nil
}

// parseGitHubDirListing extracts directory names from GitHub contents API JSON.
// Uses simple string scanning to avoid a JSON dependency for this one case.
func parseGitHubDirListing(body string) []string {
	var slugs []string
	// Look for "name": "xxx" entries where "type": "dir"
	lines := strings.Split(body, "\n")
	var currentName string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, `"name":`) {
			_, v := parseJSONStringField(line)
			currentName = v
		}
		if strings.HasPrefix(line, `"type":`) {
			_, v := parseJSONStringField(line)
			if v == "dir" && currentName != "" {
				slugs = append(slugs, currentName)
			}
			currentName = ""
		}
	}
	return slugs
}

func parseJSONStringField(line string) (string, string) {
	idx := strings.Index(line, ":")
	if idx < 0 {
		return "", ""
	}
	value := strings.TrimSpace(line[idx+1:])
	value = strings.TrimSuffix(value, ",")
	value = strings.Trim(value, `"`)
	return "", value
}

// GetSkillFile returns the content of a file within a skill.
func (s *Service) GetSkillFile(ctx context.Context, skillID, path string) (string, error) {
	skill, err := s.GetSkill(ctx, skillID)
	if err != nil {
		return "", err
	}
	if path == "" || path == "SKILL.md" {
		return s.getSkillMainContent(skill)
	}
	if skill.SourceType == sourceTypeLocalPath && skill.SourceLocator != "" {
		return readLocalSkillFile(skill.SourceLocator, path)
	}
	return "", fmt.Errorf("file %q not available for source type %q", path, skill.SourceType)
}

func (s *Service) getSkillMainContent(skill *models.Skill) (string, error) {
	if skill.SourceType == sourceTypeLocalPath && skill.SourceLocator != "" && skill.Content == "" {
		return readLocalSkillFile(skill.SourceLocator, "SKILL.md")
	}
	if skill.Content != "" {
		return skill.Content, nil
	}
	return "", fmt.Errorf("no content available for skill %s", skill.ID)
}

func readLocalSkillFile(basePath, relPath string) (string, error) {
	fullPath := filepath.Join(basePath, relPath)
	// Prevent directory traversal.
	abs, err := filepath.Abs(fullPath)
	if err != nil {
		return "", err
	}
	baseAbs, err := filepath.Abs(basePath)
	if err != nil {
		return "", err
	}
	if !strings.HasPrefix(abs, baseAbs) {
		return "", fmt.Errorf("path traversal not allowed")
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("file not found: %s", relPath)
		}
		return "", err
	}
	return string(data), nil
}
