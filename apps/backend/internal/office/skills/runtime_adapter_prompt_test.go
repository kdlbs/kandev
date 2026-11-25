package skills

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jmoiron/sqlx"

	runtimeskill "github.com/kandev/kandev/internal/agent/runtime/lifecycle/skill"
	settingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/office/models"
	promptservice "github.com/kandev/kandev/internal/prompts/service"
	promptstore "github.com/kandev/kandev/internal/prompts/store"
)

const (
	adapterTestPromptName = "shared-rules"
	adapterTestRules      = "Prefer the simplest change that works."
	adapterTestContent    = "---\nname: reviewer\ndescription: Reviews changes\n---\n# Reviewer\n\nFollow @shared-rules."
	expansionMarker       = "EXPANDED PROMPT REFERENCES:"
)

func newAdapterPromptService(t *testing.T) (*promptservice.Service, *sqlx.DB) {
	t.Helper()
	conn, err := db.OpenSQLite(filepath.Join(t.TempDir(), "prompts.db"))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	sqlxDB := sqlx.NewDb(conn, "sqlite3")
	repo, cleanup, err := promptstore.Provide(sqlxDB, sqlxDB)
	if err != nil {
		t.Fatalf("create prompt repo: %v", err)
	}
	t.Cleanup(func() {
		_ = cleanup()
		_ = sqlxDB.Close()
	})
	return promptservice.NewService(repo), sqlxDB
}

func adapterTestSkill() *models.Skill {
	return &models.Skill{Slug: "reviewer", Content: adapterTestContent}
}

func TestSkillReaderAdapterExpandsSavedPromptReferences(t *testing.T) {
	ctx := context.Background()
	prompts, _ := newAdapterPromptService(t)
	saved, err := prompts.CreatePrompt(ctx, adapterTestPromptName, adapterTestRules)
	if err != nil {
		t.Fatalf("create prompt: %v", err)
	}
	stored := adapterTestSkill()
	adapter := NewSkillReaderAdapter(stubOfficeSkillReader{skill: stored}).WithPromptReferenceExpander(prompts, nil)

	got, err := adapter.GetSkillFromConfig(ctx, "reviewer")
	if err != nil {
		t.Fatalf("GetSkillFromConfig: %v", err)
	}
	if !strings.HasPrefix(got.Content, adapterTestContent+"\n\n") {
		t.Fatalf("authored content must stay first and unchanged: %q", got.Content)
	}
	if strings.Count(got.Content, expansionMarker) != 1 || !strings.Contains(got.Content, "### @shared-rules\n"+adapterTestRules) {
		t.Fatalf("expected one expansion with the saved definition: %q", got.Content)
	}
	if stored.Content != adapterTestContent {
		t.Fatalf("stored skill was mutated: %q", stored.Content)
	}

	updated := "Updated shared rules."
	if _, err := prompts.UpdatePrompt(ctx, saved.ID, nil, &updated); err != nil {
		t.Fatalf("update prompt: %v", err)
	}
	got, err = adapter.GetSkillFromConfig(ctx, "reviewer")
	if err != nil {
		t.Fatalf("GetSkillFromConfig after update: %v", err)
	}
	if !strings.Contains(got.Content, "### @shared-rules\n"+updated) || strings.Contains(got.Content, adapterTestRules) {
		t.Fatalf("expected the updated saved definition: %q", got.Content)
	}
}

func TestSkillReaderAdapterLeavesContentWithoutExpansion(t *testing.T) {
	ctx := context.Background()
	prompts, sqlxDB := newAdapterPromptService(t)
	if _, err := prompts.CreatePrompt(ctx, adapterTestPromptName, adapterTestRules); err != nil {
		t.Fatalf("create prompt: %v", err)
	}

	plain := &models.Skill{Slug: "plain", Content: "# Plain\n\nNo references here."}
	cases := map[string]*SkillReaderAdapter{
		"nil expander": NewSkillReaderAdapter(stubOfficeSkillReader{skill: adapterTestSkill()}),
		"no reference": NewSkillReaderAdapter(stubOfficeSkillReader{skill: plain}).WithPromptReferenceExpander(prompts, nil),
	}
	want := map[string]string{"nil expander": adapterTestContent, "no reference": plain.Content}
	for name, adapter := range cases {
		got, err := adapter.GetSkillFromConfig(ctx, "any")
		if err != nil {
			t.Fatalf("%s: GetSkillFromConfig: %v", name, err)
		}
		if got.Content != want[name] {
			t.Fatalf("%s: expected unchanged content, got %q", name, got.Content)
		}
	}

	if err := sqlxDB.Close(); err != nil {
		t.Fatalf("close prompt db: %v", err)
	}
	failing := NewSkillReaderAdapter(stubOfficeSkillReader{skill: adapterTestSkill()}).WithPromptReferenceExpander(prompts, nil)
	got, err := failing.GetSkillFromConfig(ctx, "reviewer")
	if err != nil {
		t.Fatalf("lookup failure must not fail the read: %v", err)
	}
	if got.Content != adapterTestContent {
		t.Fatalf("lookup failure must leave content unchanged, got %q", got.Content)
	}
}

func TestSkillReaderAdapterExpansionReachesDeployedSkillFile(t *testing.T) {
	ctx := context.Background()
	prompts, _ := newAdapterPromptService(t)
	if _, err := prompts.CreatePrompt(ctx, adapterTestPromptName, adapterTestRules); err != nil {
		t.Fatalf("create prompt: %v", err)
	}
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "console"})
	if err != nil {
		t.Fatalf("logger: %v", err)
	}
	deployer, err := runtimeskill.New(runtimeskill.Config{
		Logger:                  log,
		BasePath:                t.TempDir(),
		SkillReader:             NewSkillReaderAdapter(stubOfficeSkillReader{skill: adapterTestSkill()}).WithPromptReferenceExpander(prompts, nil),
		ProjectSkillDirResolver: func(string) string { return ".claude/skills" },
	})
	if err != nil {
		t.Fatalf("runtimeskill.New: %v", err)
	}
	worktree := t.TempDir()
	if _, err := deployer.Deploy(ctx, runtimeskill.Request{
		Profile:       &settingsmodels.AgentProfile{ID: "profile-1", AgentID: "claude-acp", SkillIDs: `["reviewer"]`},
		ExecutorType:  "local_pc",
		WorkspaceID:   "ws-1",
		WorkspacePath: worktree,
	}); err != nil {
		t.Fatalf("Deploy: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(worktree, ".claude", "skills", "kandev-reviewer", "SKILL.md"))
	if err != nil {
		t.Fatalf("read SKILL.md: %v", err)
	}
	written := string(data)
	if !strings.HasPrefix(written, "---\nname: reviewer\n") {
		t.Fatalf("frontmatter must stay first: %q", written)
	}
	if !strings.Contains(written, "Follow @shared-rules.") || !strings.Contains(written, "### @shared-rules\n"+adapterTestRules) {
		t.Fatalf("SKILL.md missing reference or expansion: %q", written)
	}
}
