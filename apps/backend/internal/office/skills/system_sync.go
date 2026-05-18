package skills

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"gopkg.in/yaml.v3"

	settingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/office/configloader"
	"github.com/kandev/kandev/internal/office/models"
)

// SourceTypeSystem marks an office_skills row as kandev-owned. The
// startup sync upserts every embedded SKILL.md against this type;
// user imports never set it. Kept as a literal so the SQL filter and
// the spec stay in sync.
const SourceTypeSystem = "system"

// SystemSkillSpec is the parsed view of a single embedded SKILL.md
// from `apps/backend/internal/office/configloader/skills/<slug>/`.
type SystemSkillSpec struct {
	Slug            string
	Name            string
	Description     string
	Version         string
	DefaultForRoles []string
	Content         string
	ContentHash     string
}

// SystemSyncRepo is the persistence slice required by
// SyncSystemSkills. Kept narrow so tests can stub it and so this
// file doesn't reach into the wider skillRepo interface used by
// SkillService (which carries dependencies system-sync doesn't
// need).
type SystemSyncRepo interface {
	ListSystemSkills(ctx context.Context, workspaceID string) ([]*models.Skill, error)
	GetSkillBySlug(ctx context.Context, workspaceID, slug string) (*models.Skill, error)
	CreateSkill(ctx context.Context, skill *models.Skill) error
	UpdateSkill(ctx context.Context, skill *models.Skill) error
	DeleteSkill(ctx context.Context, id string) error

	// Agent-profile access used to scrub a deleted system skill's ID
	// out of every agent_profiles.skill_ids JSON array in the same
	// workspace, so retiring a bundled skill doesn't leave dangling
	// references on per-agent profiles.
	ListAgentInstances(ctx context.Context, workspaceID string) ([]*settingsmodels.AgentProfile, error)
	UpdateAgentInstance(ctx context.Context, agent *settingsmodels.AgentProfile) error
}

// SyncReport summarises one sync pass across all workspaces. The
// startup caller surfaces this as a single log line so operators can
// see exactly which slugs landed where after a kandev upgrade.
type SyncReport struct {
	Inserted []string
	Updated  []string
	Removed  []string
}

// SyncSystemSkills idempotently reconciles the office_skills table
// against the embedded bundled set for each workspace passed in.
// Inserts missing rows, updates rows whose content_hash drifted,
// removes rows for slugs no longer in the bundle. Per-agent
// desired_skills references survive across content updates because
// the row id is preserved.
//
// Production callers pass the result of LoadBundledSystemSkills() as
// `bundled`. Tests inject a synthetic spec list to exercise content
// drift and slug-removal branches without mutating the //go:embed FS.
// A nil `bundled` falls back to LoadBundledSystemSkills for backwards
// compatibility with any caller that hasn't been threaded through.
func SyncSystemSkills(
	ctx context.Context,
	repo SystemSyncRepo,
	workspaceIDs []string,
	bundled []SystemSkillSpec,
	log *logger.Logger,
) (SyncReport, error) {
	specs := bundled
	if specs == nil {
		loaded, err := LoadBundledSystemSkills()
		if err != nil {
			return SyncReport{}, fmt.Errorf("load bundled skills: %w", err)
		}
		specs = loaded
	}
	bundledBySlug := make(map[string]SystemSkillSpec, len(specs))
	for _, s := range specs {
		bundledBySlug[s.Slug] = s
	}

	var report SyncReport
	for _, wsID := range workspaceIDs {
		ins, upd, rem, err := syncWorkspace(ctx, repo, wsID, bundledBySlug)
		if err != nil {
			log.Error("system skill sync failed for workspace",
				zap.String("workspace_id", wsID), zap.Error(err))
			continue
		}
		report.Inserted = append(report.Inserted, scope(wsID, ins)...)
		report.Updated = append(report.Updated, scope(wsID, upd)...)
		report.Removed = append(report.Removed, scope(wsID, rem)...)
	}
	log.Info("system skills synced",
		zap.Int("workspaces", len(workspaceIDs)),
		zap.Int("bundled", len(specs)),
		zap.Strings("inserted", report.Inserted),
		zap.Strings("updated", report.Updated),
		zap.Strings("removed", report.Removed),
	)
	return report, nil
}

// syncWorkspace handles one workspace. Returns the (insert, update,
// remove) slug lists for the report. Errors propagate; the caller
// logs and continues to the next workspace so one bad row doesn't
// gate the rest.
func syncWorkspace(
	ctx context.Context,
	repo SystemSyncRepo,
	wsID string,
	bundled map[string]SystemSkillSpec,
) (inserted, updated, removed []string, err error) {
	existing, err := repo.ListSystemSkills(ctx, wsID)
	if err != nil {
		return nil, nil, nil, err
	}
	existingBySlug := make(map[string]*models.Skill, len(existing))
	for _, s := range existing {
		existingBySlug[s.Slug] = s
	}

	// Walk bundled slugs in sorted order so log output is stable.
	slugs := make([]string, 0, len(bundled))
	for slug := range bundled {
		slugs = append(slugs, slug)
	}
	sort.Strings(slugs)

	for _, slug := range slugs {
		spec := bundled[slug]
		cur, ok := existingBySlug[slug]
		if !ok {
			if err := repo.CreateSkill(ctx, newSystemSkillRow(wsID, spec)); err != nil {
				return inserted, updated, removed, fmt.Errorf("insert %s: %w", slug, err)
			}
			inserted = append(inserted, slug)
			continue
		}
		if cur.ContentHash == spec.ContentHash && cur.IsSystem {
			continue
		}
		applySystemSkillUpdate(cur, spec)
		if err := repo.UpdateSkill(ctx, cur); err != nil {
			return inserted, updated, removed, fmt.Errorf("update %s: %w", slug, err)
		}
		updated = append(updated, slug)
	}

	for slug, cur := range existingBySlug {
		if _, kept := bundled[slug]; kept {
			continue
		}
		if err := repo.DeleteSkill(ctx, cur.ID); err != nil {
			return inserted, updated, removed, fmt.Errorf("delete %s: %w", slug, err)
		}
		if err := detachSkillFromAgents(ctx, repo, wsID, cur.ID); err != nil {
			return inserted, updated, removed, fmt.Errorf("detach %s: %w", slug, err)
		}
		removed = append(removed, slug)
	}
	return inserted, updated, removed, nil
}

// detachSkillFromAgents removes the deleted skill's ID from every
// agent_profiles.skill_ids array in the workspace, preventing
// dangling references after a kandev release retires a bundled skill.
// Profiles whose array didn't contain the ID are skipped (no write).
func detachSkillFromAgents(
	ctx context.Context, repo SystemSyncRepo, wsID, skillID string,
) error {
	agents, err := repo.ListAgentInstances(ctx, wsID)
	if err != nil {
		return fmt.Errorf("list agents: %w", err)
	}
	for _, agent := range agents {
		filtered, changed := removeIDFromJSONArray(agent.SkillIDs, skillID)
		if !changed {
			continue
		}
		agent.SkillIDs = filtered
		if err := repo.UpdateAgentInstance(ctx, agent); err != nil {
			return fmt.Errorf("update agent %s: %w", agent.ID, err)
		}
	}
	return nil
}

// removeIDFromJSONArray parses a JSON-array string, removes every
// occurrence of `id`, and returns the re-encoded array along with a
// flag indicating whether anything was removed. Malformed input is
// treated as a no-op so a corrupt profile column doesn't fail the
// sync.
func removeIDFromJSONArray(raw, id string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "null" {
		return raw, false
	}
	var ids []string
	if err := json.Unmarshal([]byte(raw), &ids); err != nil {
		return raw, false
	}
	out := make([]string, 0, len(ids))
	removed := false
	for _, existing := range ids {
		if existing == id {
			removed = true
			continue
		}
		out = append(out, existing)
	}
	if !removed {
		return raw, false
	}
	encoded, err := json.Marshal(out)
	if err != nil {
		return raw, false
	}
	return string(encoded), true
}

func newSystemSkillRow(wsID string, spec SystemSkillSpec) *models.Skill {
	roles, _ := json.Marshal(spec.DefaultForRoles)
	return &models.Skill{
		ID:              uuid.New().String(),
		WorkspaceID:     wsID,
		Name:            spec.Name,
		Slug:            spec.Slug,
		Description:     spec.Description,
		SourceType:      SourceTypeSystem,
		SourceLocator:   "bundled:" + spec.Slug,
		Content:         spec.Content,
		FileInventory:   "[]",
		Version:         spec.Version,
		ContentHash:     spec.ContentHash,
		ApprovalState:   "approved",
		IsSystem:        true,
		SystemVersion:   spec.Version,
		DefaultForRoles: string(roles),
	}
}

func applySystemSkillUpdate(cur *models.Skill, spec SystemSkillSpec) {
	roles, _ := json.Marshal(spec.DefaultForRoles)
	cur.Name = spec.Name
	cur.Description = spec.Description
	cur.SourceType = SourceTypeSystem
	cur.SourceLocator = "bundled:" + spec.Slug
	cur.Content = spec.Content
	cur.Version = spec.Version
	cur.ContentHash = spec.ContentHash
	cur.ApprovalState = "approved"
	cur.IsSystem = true
	cur.SystemVersion = spec.Version
	cur.DefaultForRoles = string(roles)
}

func scope(wsID string, slugs []string) []string {
	if len(slugs) == 0 {
		return nil
	}
	out := make([]string, len(slugs))
	for i, s := range slugs {
		out[i] = wsID + ":" + s
	}
	return out
}

// LoadBundledSystemSkills reads every embedded SKILL.md, parses the
// `kandev:` frontmatter block, and returns a deterministic list
// sorted by slug. The kandev block is mandatory for bundled skills
// — if it's missing or has `system: false`, the file is dropped
// with a warning so a stray test fixture doesn't sneak into the
// office_skills table.
func LoadBundledSystemSkills() ([]SystemSkillSpec, error) {
	slugs, err := configloader.BundledSkillSlugs()
	if err != nil {
		return nil, err
	}
	out := make([]SystemSkillSpec, 0, len(slugs))
	for _, slug := range slugs {
		raw, err := configloader.BundledSkillContent(slug)
		if err != nil {
			return nil, fmt.Errorf("read embedded %s: %w", slug, err)
		}
		spec, err := parseSystemSkill(slug, raw)
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", slug, err)
		}
		if spec == nil {
			continue
		}
		out = append(out, *spec)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Slug < out[j].Slug })
	return out, nil
}

// skillFrontmatter is the parsed YAML block at the top of a
// SKILL.md. The `Kandev` sub-block is the marker that promotes a
// skill from user-imported to kandev-owned.
type skillFrontmatter struct {
	Name        string             `yaml:"name"`
	Description string             `yaml:"description"`
	Kandev      *kandevFrontmatter `yaml:"kandev"`
}

type kandevFrontmatter struct {
	System          bool     `yaml:"system"`
	Version         string   `yaml:"version"`
	DefaultForRoles []string `yaml:"default_for_roles"`
}

// parseSystemSkill splits a SKILL.md into its frontmatter + body,
// validates the kandev block, and returns the spec. nil + nil
// signals "not a system skill" (kandev block missing or system =
// false) — the caller skips it without erroring.
func parseSystemSkill(slug string, raw []byte) (*SystemSkillSpec, error) {
	frontmatterBytes, body, ok := splitFrontmatter(raw)
	if !ok {
		// No frontmatter at all → not a system skill (some bundled
		// fixtures pre-date the kandev frontmatter block). Skip
		// silently rather than failing the whole sync.
		return nil, nil
	}
	var fm skillFrontmatter
	if err := yaml.Unmarshal(frontmatterBytes, &fm); err != nil {
		return nil, fmt.Errorf("yaml: %w", err)
	}
	if fm.Kandev == nil || !fm.Kandev.System {
		return nil, nil
	}
	name := fm.Name
	if name == "" {
		name = slug
	}
	sum := sha256.Sum256(raw)
	return &SystemSkillSpec{
		Slug:            slug,
		Name:            name,
		Description:     fm.Description,
		Version:         fm.Kandev.Version,
		DefaultForRoles: append([]string{}, fm.Kandev.DefaultForRoles...),
		Content:         string(body),
		ContentHash:     hex.EncodeToString(sum[:]),
	}, nil
}

// splitFrontmatter returns the YAML payload and the markdown body
// from a SKILL.md that opens with a `---` delimited block. The
// trailing newline of the delimiter line is stripped from the body
// so the rendered content doesn't begin with a blank line.
func splitFrontmatter(raw []byte) (yamlBytes, body []byte, ok bool) {
	text := string(raw)
	if !strings.HasPrefix(text, "---\n") && !strings.HasPrefix(text, "---\r\n") {
		return nil, nil, false
	}
	rest := strings.TrimPrefix(strings.TrimPrefix(text, "---\r\n"), "---\n")
	end := strings.Index(rest, "\n---")
	if end < 0 {
		return nil, nil, false
	}
	yamlPart := rest[:end]
	body = []byte(strings.TrimPrefix(strings.TrimPrefix(rest[end:], "\n---\r\n"), "\n---\n"))
	return []byte(yamlPart), body, true
}
