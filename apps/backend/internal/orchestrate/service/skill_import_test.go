package service_test

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/kandev/kandev/internal/orchestrate/models"
	"github.com/kandev/kandev/internal/orchestrate/service"
)

// mockFetcher implements service.GitHubFetcher for testing.
type mockFetcher struct {
	responses map[string]fetchResponse
}

type fetchResponse struct {
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

func TestParseSource_OrgRepoSlug(t *testing.T) {
	ps, err := service.ParseSource("myorg/myrepo/my-skill")
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
	ps, err := service.ParseSource("myorg/myrepo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ps.Owner != "myorg" || ps.Repo != "myrepo" || ps.Slug != "" {
		t.Errorf("got owner=%q repo=%q slug=%q", ps.Owner, ps.Repo, ps.Slug)
	}
}

func TestParseSource_SkillsShURL(t *testing.T) {
	ps, err := service.ParseSource("https://skills.sh/acme/tools/deploy")
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
	ps, err := service.ParseSource("https://github.com/acme/tools")
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
	ps, err := service.ParseSource("https://github.com/acme/tools/tree/main/skills/deploy")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ps.Owner != "acme" || ps.Repo != "tools" || ps.Slug != "deploy" {
		t.Errorf("got owner=%q repo=%q slug=%q", ps.Owner, ps.Repo, ps.Slug)
	}
}

func TestParseSource_LocalPath(t *testing.T) {
	ps, err := service.ParseSource("/home/user/skills/my-skill")
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
	ps, err := service.ParseSource("./skills/my-skill")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ps.IsLocal {
		t.Error("expected local path for relative path")
	}
}

func TestParseSource_Empty(t *testing.T) {
	_, err := service.ParseSource("")
	if err == nil {
		t.Fatal("expected error for empty source")
	}
}

func TestParseSource_Invalid(t *testing.T) {
	_, err := service.ParseSource("not-a-valid-source")
	if err == nil {
		t.Fatal("expected error for invalid source")
	}
}

func TestParseFrontmatter_Basic(t *testing.T) {
	content := `---
name: Code Review
description: Review code changes for quality
---
# Code Review Skill
`
	name, desc := service.ParseFrontmatter(content)
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
	name, desc := service.ParseFrontmatter(content)
	if name != "Deploy Helper" {
		t.Errorf("name = %q, want %q", name, "Deploy Helper")
	}
	if desc != "Helps with deployments" {
		t.Errorf("description = %q, want %q", desc, "Helps with deployments")
	}
}

func TestParseFrontmatter_NoFrontmatter(t *testing.T) {
	content := "# Just a Markdown File\nNo frontmatter here."
	name, desc := service.ParseFrontmatter(content)
	if name != "" || desc != "" {
		t.Errorf("expected empty name/desc, got name=%q desc=%q", name, desc)
	}
}

func TestImportFromSource_GitHubSingleSkill(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	skillContent := `---
name: Deploy
description: Deployment helper
---
# Deploy Skill
`
	fetcher := &mockFetcher{responses: map[string]fetchResponse{
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
	svc := newTestService(t)
	ctx := context.Background()

	fetcher := &mockFetcher{responses: map[string]fetchResponse{
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
	svc := newTestService(t)
	ctx := context.Background()

	fetcher := &mockFetcher{responses: map[string]fetchResponse{
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
	svc := newTestService(t)
	ctx := context.Background()

	fetcher := &mockFetcher{responses: map[string]fetchResponse{}}
	_, err := svc.ImportFromSource(ctx, "ws-1", "acme/tools/nonexistent", fetcher)
	if err == nil {
		t.Fatal("expected error for missing SKILL.md")
	}
}

func TestGetSkillFile_InlineSkill(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	fetcher := &mockFetcher{responses: map[string]fetchResponse{
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
	svc := newTestService(t)
	ctx := context.Background()

	fetcher := &mockFetcher{responses: map[string]fetchResponse{
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
	svc := newTestService(t)
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
	fetcher := &mockFetcher{responses: map[string]fetchResponse{
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
	svc := newTestService(t)
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
	fetcher := &mockFetcher{responses: map[string]fetchResponse{
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
	svc := newTestService(t)
	ctx := context.Background()

	fetcher := &mockFetcher{responses: map[string]fetchResponse{
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

func TestValidateSourceType_AcceptsSkillsSh(t *testing.T) {
	svc := newTestService(t)
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
