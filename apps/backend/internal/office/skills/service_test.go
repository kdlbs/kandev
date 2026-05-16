package skills_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/repository/sqlite"
	"github.com/kandev/kandev/internal/office/skills"
)

// noopActivitySkills implements shared.ActivityLogger for tests.
type noopActivitySkills struct{}

func (n *noopActivitySkills) LogActivity(_ context.Context, _, _, _, _, _, _, _ string) {}
func (n *noopActivitySkills) LogActivityWithRun(_ context.Context, _, _, _, _, _, _, _, _, _ string) {
}

// newTestSkillService creates a SkillService backed by in-memory SQLite.
func newTestSkillService(t *testing.T) *skills.SkillService {
	t.Helper()
	db, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	repo, err := sqlite.NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("new repo: %v", err)
	}

	log := logger.Default()
	return skills.NewSkillService(repo, log, &noopActivitySkills{}, nil, nil)
}

// --- GenerateSlug tests ---

func TestGenerateSkillSlug(t *testing.T) {
	tests := []struct {
		name string
		want string
	}{
		{"Code Review", "code-review"},
		{"  Go Testing  ", "go-testing"},
		{"Deploy Runbook!!!", "deploy-runbook"},
		{"MCP--server", "mcp-server"},
		{"test", "test"},
		{"", "skill"},
		{"Already-Kebab", "already-kebab"},
		{"UPPERCASE NAME", "uppercase-name"},
		{"special@#$chars", "special-chars"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := skills.GenerateSlug(tt.name)
			if got != tt.want {
				t.Errorf("GenerateSlug(%q) = %q, want %q", tt.name, got, tt.want)
			}
		})
	}
}

// --- ValidateAndPrepareSkill tests ---

func TestValidateAndPrepareSkill_AutoGeneratesSlug(t *testing.T) {
	svc := newTestSkillService(t)
	ctx := context.Background()

	skill := &models.Skill{
		WorkspaceID: "ws-1",
		Name:        "Code Review",
		SourceType:  "inline",
		Content:     "# Review code",
	}
	if err := svc.ValidateAndPrepareSkill(ctx, skill); err != nil {
		t.Fatalf("validate: %v", err)
	}
	if skill.Slug != "code-review" {
		t.Errorf("slug = %q, want %q", skill.Slug, "code-review")
	}
}

func TestValidateAndPrepareSkill_RejectsEmptyName(t *testing.T) {
	svc := newTestSkillService(t)
	ctx := context.Background()

	skill := &models.Skill{
		WorkspaceID: "ws-1",
		SourceType:  "inline",
	}
	err := svc.ValidateAndPrepareSkill(ctx, skill)
	if err == nil {
		t.Fatal("expected error for empty name")
	}
}

func TestValidateAndPrepareSkill_RejectsDuplicateSlug(t *testing.T) {
	svc := newTestSkillService(t)
	ctx := context.Background()

	s1 := &models.Skill{WorkspaceID: "ws-1", Name: "Test Skill", SourceType: "inline"}
	if err := svc.ValidateAndPrepareSkill(ctx, s1); err != nil {
		t.Fatalf("validate first: %v", err)
	}
	if err := svc.CreateSkill(ctx, s1); err != nil {
		t.Fatalf("create first: %v", err)
	}

	s2 := &models.Skill{WorkspaceID: "ws-1", Name: "Test Skill", SourceType: "inline"}
	err := svc.ValidateAndPrepareSkill(ctx, s2)
	if err == nil {
		t.Fatal("expected duplicate slug error")
	}
}

func TestValidateAndPrepareSkill_RejectsSameSlugInSameWorkspace(t *testing.T) {
	svc := newTestSkillService(t)
	ctx := context.Background()

	s1 := &models.Skill{WorkspaceID: "ws-1", Name: "Test Skill", SourceType: "inline"}
	if err := svc.ValidateAndPrepareSkill(ctx, s1); err != nil {
		t.Fatalf("validate first: %v", err)
	}
	if err := svc.CreateSkill(ctx, s1); err != nil {
		t.Fatalf("create first: %v", err)
	}

	s2 := &models.Skill{WorkspaceID: "ws-1", Name: "Test Skill", SourceType: "inline"}
	err := svc.ValidateAndPrepareSkill(ctx, s2)
	if err == nil {
		t.Fatal("expected duplicate slug error in same workspace")
	}
}

func TestValidateAndPrepareSkill_RejectsInvalidSourceType(t *testing.T) {
	svc := newTestSkillService(t)
	ctx := context.Background()

	skill := &models.Skill{
		WorkspaceID: "ws-1",
		Name:        "Bad Source",
		SourceType:  "ftp",
	}
	err := svc.ValidateAndPrepareSkill(ctx, skill)
	if err == nil {
		t.Fatal("expected error for invalid source type")
	}
}

func TestListSkillsWithUsage(t *testing.T) {
	svc := newTestSkillService(t)
	ctx := context.Background()

	skill := &models.Skill{WorkspaceID: "ws-1", Name: "My Skill", Slug: "my-skill", SourceType: "inline"}
	if err := svc.CreateSkill(ctx, skill); err != nil {
		t.Fatalf("create: %v", err)
	}

	// The lazy system-skill sync fires on the first ListSkills call
	// for a workspace, populating bundled system rows alongside the
	// user skill created above. The test asserts the *user* skill
	// appears with the correct usage count; the system rows are
	// validated separately in system_sync_test.go.
	list, err := svc.ListSkillsWithUsage(ctx, "ws-1")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	var userRow *skills.SkillWithUsage
	for _, row := range list {
		if row.Slug == "my-skill" {
			userRow = row
			break
		}
	}
	if userRow == nil {
		t.Fatalf("user skill not found in list of %d rows", len(list))
	}
	if userRow.UsedByCount != 0 {
		t.Errorf("used_by_count = %d, want 0", userRow.UsedByCount)
	}
}

func TestValidateSourceType_AcceptsSkillsSh(t *testing.T) {
	svc := newTestSkillService(t)
	ctx := context.Background()

	skill := &models.Skill{
		WorkspaceID: "ws-1",
		Name:        "From Skills.sh",
		SourceType:  "skills_sh",
	}
	err := svc.ValidateAndPrepareSkill(ctx, skill)
	if err != nil {
		t.Fatalf("skills_sh should be valid: %v", err)
	}
}

func TestValidateSourceType_AcceptsUserHome(t *testing.T) {
	svc := newTestSkillService(t)
	ctx := context.Background()

	skill := &models.Skill{
		WorkspaceID: "ws-1",
		Name:        "From User Home",
		SourceType:  skills.SkillSourceTypeUserHome,
	}
	if err := svc.ValidateAndPrepareSkill(ctx, skill); err != nil {
		t.Fatalf("user_home should be valid: %v", err)
	}
}

func TestUserHomeSkillDiscoveryAndImportSnapshotsToDB(t *testing.T) {
	svc := newTestSkillService(t)
	ctx := context.Background()
	home := t.TempDir()
	writeUserHomeSkill(t, home, ".codex/skills/review",
		"---\nname: Code Review\ndescription: Review changes\n---\n# Review\n",
		map[string]string{"guide.md": "Look for regressions.\n"})
	svc.SetUserHomeResolver(func() (string, error) { return home, nil })
	svc.SetUserSkillDirResolver(func(provider string) (string, bool) {
		if provider == "codex-acp" {
			return ".codex/skills", true
		}
		return "", false
	})

	discovered, err := svc.DiscoverUserSkills(ctx, "codex-acp")
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	if len(discovered) != 1 {
		t.Fatalf("discovered count = %d, want 1", len(discovered))
	}
	if discovered[0].Key != "review" || discovered[0].Name != "Code Review" {
		t.Fatalf("discovered = %+v, want review/Code Review", discovered[0])
	}
	if discovered[0].Files[0].Content != "" {
		t.Fatal("discovery should not include supporting file content")
	}

	imported, err := svc.ImportUserHomeSkill(ctx, "ws-1", "codex-acp", "review")
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if imported.SourceType != skills.SkillSourceTypeUserHome {
		t.Fatalf("source_type = %q, want user_home", imported.SourceType)
	}
	if imported.SourceLocator != "codex-acp:review" {
		t.Fatalf("source_locator = %q, want codex-acp:review", imported.SourceLocator)
	}
	if imported.Content == "" {
		t.Fatal("expected SKILL.md content to be snapshotted")
	}
	var files []skills.UserHomeSkillFile
	if err := json.Unmarshal([]byte(imported.FileInventory), &files); err != nil {
		t.Fatalf("decode inventory: %v", err)
	}
	if len(files) != 1 || files[0].Path != "guide.md" || files[0].Content == "" {
		t.Fatalf("files = %+v, want snapshotted guide.md", files)
	}
	content, err := svc.GetSkillFile(ctx, imported.ID, "guide.md")
	if err != nil {
		t.Fatalf("get supporting file: %v", err)
	}
	if content != "Look for regressions.\n" {
		t.Fatalf("supporting content = %q", content)
	}

	writeUserHomeSkill(t, home, ".codex/skills/review",
		"---\nname: Code Review Updated\n---\n# Review\n",
		map[string]string{"guide.md": "Updated.\n"})
	refreshed, err := svc.ImportUserHomeSkill(ctx, "ws-1", "codex-acp", "review")
	if err != nil {
		t.Fatalf("refresh import: %v", err)
	}
	if refreshed.ID != imported.ID {
		t.Fatalf("refresh ID = %q, want %q", refreshed.ID, imported.ID)
	}
	if refreshed.Name != "Code Review Updated" {
		t.Fatalf("refresh name = %q", refreshed.Name)
	}
}

func writeUserHomeSkill(t *testing.T, home, relDir, skillMD string, files map[string]string) {
	t.Helper()
	root := filepath.Join(home, filepath.FromSlash(relDir))
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatalf("mkdir skill root: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "SKILL.md"), []byte(skillMD), 0o644); err != nil {
		t.Fatalf("write SKILL.md: %v", err)
	}
	for rel, content := range files {
		path := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir supporting file dir: %v", err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("write supporting file: %v", err)
		}
	}
}

// --- ParseSource tests ---

func TestParseSource_OrgRepoSlug(t *testing.T) {
	ps, err := skills.ParseSource("myorg/myrepo/my-skill")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ps.Owner != "myorg" || ps.Repo != "myrepo" || ps.Slug != "my-skill" {
		t.Errorf("got owner=%q repo=%q slug=%q", ps.Owner, ps.Repo, ps.Slug)
	}
	if ps.SourceType != "git" {
		t.Errorf("source_type = %q, want git", ps.SourceType)
	}
}

func TestParseSource_OrgRepoOnly(t *testing.T) {
	ps, err := skills.ParseSource("myorg/myrepo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ps.Owner != "myorg" || ps.Repo != "myrepo" || ps.Slug != "" {
		t.Errorf("got owner=%q repo=%q slug=%q", ps.Owner, ps.Repo, ps.Slug)
	}
}

func TestParseSource_SkillsShURL(t *testing.T) {
	ps, err := skills.ParseSource("https://skills.sh/acme/tools/deploy")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ps.Owner != "acme" || ps.Repo != "tools" || ps.Slug != "deploy" {
		t.Errorf("got owner=%q repo=%q slug=%q", ps.Owner, ps.Repo, ps.Slug)
	}
	if ps.SourceType != "skills_sh" {
		t.Errorf("source_type = %q, want skills_sh", ps.SourceType)
	}
}

func TestParseSource_GitHubURL(t *testing.T) {
	ps, err := skills.ParseSource("https://github.com/acme/tools")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ps.Owner != "acme" || ps.Repo != "tools" {
		t.Errorf("got owner=%q repo=%q", ps.Owner, ps.Repo)
	}
	if ps.SourceType != "git" {
		t.Errorf("source_type = %q, want git", ps.SourceType)
	}
}

func TestParseSource_GitHubTreeURL(t *testing.T) {
	ps, err := skills.ParseSource("https://github.com/acme/tools/tree/main/skills/deploy")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ps.Owner != "acme" || ps.Repo != "tools" || ps.Slug != "deploy" {
		t.Errorf("got owner=%q repo=%q slug=%q", ps.Owner, ps.Repo, ps.Slug)
	}
}

func TestParseSource_LocalPath(t *testing.T) {
	ps, err := skills.ParseSource("/home/user/skills/my-skill")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ps.IsLocal || ps.LocalPath != "/home/user/skills/my-skill" {
		t.Errorf("expected local path, got %+v", ps)
	}
	if ps.SourceType != "local_path" {
		t.Errorf("source_type = %q, want local_path", ps.SourceType)
	}
}

func TestParseSource_RelativePath(t *testing.T) {
	ps, err := skills.ParseSource("./skills/my-skill")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ps.IsLocal {
		t.Error("expected local path for relative path")
	}
}

func TestParseSource_Empty(t *testing.T) {
	_, err := skills.ParseSource("")
	if err == nil {
		t.Fatal("expected error for empty source")
	}
}

func TestParseSource_Invalid(t *testing.T) {
	_, err := skills.ParseSource("not-a-valid-source")
	if err == nil {
		t.Fatal("expected error for invalid source")
	}
}

// --- ParseFrontmatter tests ---

func TestParseFrontmatter_Basic(t *testing.T) {
	content := `---
name: Code Review
description: Review code changes for quality
---
# Code Review Skill
`
	name, desc := skills.ParseFrontmatter(content)
	if name != "Code Review" {
		t.Errorf("name = %q, want %q", name, "Code Review")
	}
	if desc != "Review code changes for quality" {
		t.Errorf("description = %q, want %q", desc, "Review code changes for quality")
	}
}

func TestParseFrontmatter_QuotedValues(t *testing.T) {
	content := `---
name: "Deploy Helper"
description: 'Helps with deployments'
---
`
	name, desc := skills.ParseFrontmatter(content)
	if name != "Deploy Helper" {
		t.Errorf("name = %q, want %q", name, "Deploy Helper")
	}
	if desc != "Helps with deployments" {
		t.Errorf("description = %q, want %q", desc, "Helps with deployments")
	}
}

func TestParseFrontmatter_NoFrontmatter(t *testing.T) {
	content := "# Just a Markdown File\nNo frontmatter here."
	name, desc := skills.ParseFrontmatter(content)
	if name != "" || desc != "" {
		t.Errorf("expected empty name/desc, got name=%q desc=%q", name, desc)
	}
}

// --- mockFetcher for ImportFromSource tests ---

type mockFetcher struct {
	responses map[string]fetchResp
}

type fetchResp struct {
	body   []byte
	status int
	err    error
}

func (m *mockFetcher) Fetch(_ context.Context, url string) ([]byte, int, error) {
	if r, ok := m.responses[url]; ok {
		return r.body, r.status, r.err
	}
	return nil, http.StatusNotFound, nil
}

// --- ImportFromSource tests ---

func TestImportFromSource_GitHubSingleSkill(t *testing.T) {
	svc := newTestSkillService(t)
	ctx := context.Background()

	skillContent := `---
name: Deploy
description: Deployment helper
---
# Deploy Skill
`
	fetcher := &mockFetcher{responses: map[string]fetchResp{
		"https://raw.githubusercontent.com/acme/tools/main/skills/deploy/SKILL.md": {
			body: []byte(skillContent), status: http.StatusOK,
		},
	}}

	result, err := svc.ImportFromSource(ctx, "ws-1", "acme/tools/deploy", fetcher)
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if len(result.Skills) != 1 {
		t.Fatalf("skills count = %d, want 1", len(result.Skills))
	}
	sk := result.Skills[0]
	if sk.Name != "Deploy" {
		t.Errorf("name = %q, want %q", sk.Name, "Deploy")
	}
	if sk.SourceType != "git" {
		t.Errorf("source_type = %q, want git", sk.SourceType)
	}
	if sk.Description != "Deployment helper" {
		t.Errorf("description = %q, want %q", sk.Description, "Deployment helper")
	}
}

func TestImportFromSource_SkillsShURL(t *testing.T) {
	svc := newTestSkillService(t)
	ctx := context.Background()

	fetcher := &mockFetcher{responses: map[string]fetchResp{
		"https://raw.githubusercontent.com/acme/tools/main/skills/deploy/SKILL.md": {
			body: []byte("---\nname: Deploy\n---\n# Deploy"), status: http.StatusOK,
		},
	}}

	result, err := svc.ImportFromSource(ctx, "ws-1", "https://skills.sh/acme/tools/deploy", fetcher)
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if len(result.Skills) != 1 {
		t.Fatalf("skills count = %d, want 1", len(result.Skills))
	}
	if result.Skills[0].SourceType != "skills_sh" {
		t.Errorf("source_type = %q, want skills_sh", result.Skills[0].SourceType)
	}
}

func TestImportFromSource_MasterBranchFallback(t *testing.T) {
	svc := newTestSkillService(t)
	ctx := context.Background()

	fetcher := &mockFetcher{responses: map[string]fetchResp{
		"https://raw.githubusercontent.com/acme/tools/main/skills/deploy/SKILL.md": {
			body: nil, status: http.StatusNotFound,
		},
		"https://raw.githubusercontent.com/acme/tools/master/skills/deploy/SKILL.md": {
			body: []byte("---\nname: Deploy\n---\n"), status: http.StatusOK,
		},
	}}

	result, err := svc.ImportFromSource(ctx, "ws-1", "acme/tools/deploy", fetcher)
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if len(result.Skills) != 1 {
		t.Fatalf("skills count = %d, want 1", len(result.Skills))
	}
}

func TestImportFromSource_NotFound(t *testing.T) {
	svc := newTestSkillService(t)
	ctx := context.Background()

	fetcher := &mockFetcher{responses: map[string]fetchResp{}}
	_, err := svc.ImportFromSource(ctx, "ws-1", "acme/tools/nonexistent", fetcher)
	if err == nil {
		t.Fatal("expected error for missing SKILL.md")
	}
}

func TestGetSkillFile_InlineSkill(t *testing.T) {
	svc := newTestSkillService(t)
	ctx := context.Background()

	fetcher := &mockFetcher{responses: map[string]fetchResp{
		"https://raw.githubusercontent.com/acme/tools/main/skills/test/SKILL.md": {
			body: []byte("---\nname: Test\n---\n# Test Skill Content"), status: http.StatusOK,
		},
	}}

	result, err := svc.ImportFromSource(ctx, "ws-1", "acme/tools/test", fetcher)
	if err != nil {
		t.Fatalf("import: %v", err)
	}

	content, err := svc.GetSkillFile(ctx, result.Skills[0].ID, "SKILL.md")
	if err != nil {
		t.Fatalf("get file: %v", err)
	}
	if content != "---\nname: Test\n---\n# Test Skill Content" {
		t.Errorf("content = %q", content)
	}
}

func TestGetSkillFile_EmptyPathReturnsSKILLMD(t *testing.T) {
	svc := newTestSkillService(t)
	ctx := context.Background()

	fetcher := &mockFetcher{responses: map[string]fetchResp{
		"https://raw.githubusercontent.com/acme/tools/main/skills/test/SKILL.md": {
			body: []byte("# content"), status: http.StatusOK,
		},
	}}

	result, err := svc.ImportFromSource(ctx, "ws-1", "acme/tools/test", fetcher)
	if err != nil {
		t.Fatalf("import: %v", err)
	}

	content, err := svc.GetSkillFile(ctx, result.Skills[0].ID, "")
	if err != nil {
		t.Fatalf("get file: %v", err)
	}
	if content != "# content" {
		t.Errorf("content = %q", content)
	}
}

func TestImportFromSource_RepoDiscovery(t *testing.T) {
	svc := newTestSkillService(t)
	ctx := context.Background()

	dirListing := `[
  {
    "name": "deploy",
    "type": "dir"
  },
  {
    "name": "review",
    "type": "dir"
  },
  {
    "name": "README.md",
    "type": "file"
  }
]`
	fetcher := &mockFetcher{responses: map[string]fetchResp{
		"https://api.github.com/repos/acme/tools/contents/skills": {
			body: []byte(dirListing), status: http.StatusOK,
		},
		"https://raw.githubusercontent.com/acme/tools/main/skills/deploy/SKILL.md": {
			body: []byte("---\nname: Deploy\n---\n"), status: http.StatusOK,
		},
		"https://raw.githubusercontent.com/acme/tools/main/skills/review/SKILL.md": {
			body: []byte("---\nname: Review\n---\n"), status: http.StatusOK,
		},
	}}

	result, err := svc.ImportFromSource(ctx, "ws-1", "acme/tools", fetcher)
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if len(result.Skills) != 2 {
		t.Fatalf("skills count = %d, want 2", len(result.Skills))
	}
}

func TestImportFromSource_RepoDiscovery_PartialFailure(t *testing.T) {
	svc := newTestSkillService(t)
	ctx := context.Background()

	dirListing := `[
  {
    "name": "deploy",
    "type": "dir"
  },
  {
    "name": "broken",
    "type": "dir"
  }
]`
	fetcher := &mockFetcher{responses: map[string]fetchResp{
		"https://api.github.com/repos/acme/tools/contents/skills": {
			body: []byte(dirListing), status: http.StatusOK,
		},
		"https://raw.githubusercontent.com/acme/tools/main/skills/deploy/SKILL.md": {
			body: []byte("---\nname: Deploy\n---\n"), status: http.StatusOK,
		},
	}}

	result, err := svc.ImportFromSource(ctx, "ws-1", "acme/tools", fetcher)
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if len(result.Skills) != 1 {
		t.Errorf("skills count = %d, want 1", len(result.Skills))
	}
	if len(result.Warnings) != 1 {
		t.Errorf("warnings count = %d, want 1", len(result.Warnings))
	}
}

func TestImportFromSource_FetchError(t *testing.T) {
	svc := newTestSkillService(t)
	ctx := context.Background()

	fetcher := &mockFetcher{responses: map[string]fetchResp{
		"https://raw.githubusercontent.com/acme/tools/main/skills/deploy/SKILL.md": {
			err: fmt.Errorf("network error"),
		},
		"https://raw.githubusercontent.com/acme/tools/master/skills/deploy/SKILL.md": {
			err: fmt.Errorf("network error"),
		},
	}}

	_, err := svc.ImportFromSource(ctx, "ws-1", "acme/tools/deploy", fetcher)
	if err == nil {
		t.Fatal("expected error for fetch failure")
	}
}

// --- MaterializeSkills tests ---

func TestMaterializeSkills_Inline(t *testing.T) {
	svc := newTestSkillService(t)
	ctx := context.Background()

	skill := &models.Skill{
		WorkspaceID: "ws-1",
		Name:        "Test Inline",
		Slug:        "test-inline",
		SourceType:  "inline",
		Content:     "# Test\nDo the thing.",
	}
	if err := svc.CreateSkill(ctx, skill); err != nil {
		t.Fatalf("create: %v", err)
	}

	cacheDir := t.TempDir()
	dirs, err := svc.MaterializeSkills(ctx, []string{skill.ID}, cacheDir)
	if err != nil {
		t.Fatalf("materialize: %v", err)
	}
	if len(dirs) != 1 {
		t.Fatalf("dirs count = %d, want 1", len(dirs))
	}
	if dirs[0].Slug != "test-inline" {
		t.Errorf("slug = %q, want %q", dirs[0].Slug, "test-inline")
	}

	content, err := os.ReadFile(filepath.Join(dirs[0].Path, "SKILL.md"))
	if err != nil {
		t.Fatalf("read SKILL.md: %v", err)
	}
	if string(content) != "# Test\nDo the thing." {
		t.Errorf("content = %q", string(content))
	}
}

func TestMaterializeSkills_LocalPath(t *testing.T) {
	svc := newTestSkillService(t)
	ctx := context.Background()

	skill := &models.Skill{
		WorkspaceID: "ws-1",
		Name:        "Local Skill",
		Slug:        "local-skill",
		SourceType:  "inline",
		Content:     "# Local skill content",
	}
	if err := svc.CreateSkill(ctx, skill); err != nil {
		t.Fatalf("create: %v", err)
	}

	cacheDir := t.TempDir()
	dirs, err := svc.MaterializeSkills(ctx, []string{skill.ID}, cacheDir)
	if err != nil {
		t.Fatalf("materialize: %v", err)
	}
	if len(dirs) != 1 {
		t.Fatalf("dirs count = %d, want 1", len(dirs))
	}
	if dirs[0].Slug != "local-skill" {
		t.Errorf("slug = %q, want %q", dirs[0].Slug, "local-skill")
	}
}

func TestMaterializeSkills_NonexistentSkillID(t *testing.T) {
	svc := newTestSkillService(t)
	ctx := context.Background()

	cacheDir := t.TempDir()
	dirs, err := svc.MaterializeSkills(ctx, []string{"nonexistent-skill"}, cacheDir)
	if err != nil {
		t.Fatalf("materialize should not fail overall: %v", err)
	}
	if len(dirs) != 0 {
		t.Errorf("dirs count = %d, want 0 (nonexistent skill should be skipped)", len(dirs))
	}
}

// --- SymlinkSkills and CleanupSymlinks tests ---

func TestSymlinkSkills(t *testing.T) {
	agentHome := t.TempDir()
	skillDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("test"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	dirs := []skills.SkillDir{
		{Slug: "my-skill", Path: skillDir},
	}
	if err := skills.SymlinkSkills(agentHome, dirs); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	link := filepath.Join(agentHome, ".claude", "skills", "my-skill")
	target, err := os.Readlink(link)
	if err != nil {
		t.Fatalf("readlink: %v", err)
	}
	if target != skillDir {
		t.Errorf("symlink target = %q, want %q", target, skillDir)
	}
}

func TestCleanupSymlinks(t *testing.T) {
	agentHome := t.TempDir()
	skillsDir := filepath.Join(agentHome, ".claude", "skills")
	if err := os.MkdirAll(skillsDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	target := t.TempDir()
	link := filepath.Join(skillsDir, "test-skill")
	if err := os.Symlink(target, link); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	if err := skills.CleanupSymlinks(agentHome, []string{"test-skill"}); err != nil {
		t.Fatalf("cleanup: %v", err)
	}

	if _, err := os.Lstat(link); !os.IsNotExist(err) {
		t.Errorf("symlink should be removed, got err: %v", err)
	}
}

func TestCleanupSymlinks_NonexistentIsNoop(t *testing.T) {
	agentHome := t.TempDir()
	skillsDir := filepath.Join(agentHome, ".claude", "skills")
	if err := os.MkdirAll(skillsDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	if err := skills.CleanupSymlinks(agentHome, []string{"nonexistent"}); err != nil {
		t.Fatalf("cleanup nonexistent should not error: %v", err)
	}
}
