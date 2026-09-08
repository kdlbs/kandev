package skills

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"go.uber.org/zap"
	"gopkg.in/yaml.v3"

	settingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/common/skillslug"
	"github.com/kandev/kandev/internal/office/configloader"
	"github.com/kandev/kandev/internal/office/models"
)

// SourceTypeSystem marks an office_skills row as kandev-owned. The
// startup sync upserts every embedded SKILL.md against this type;
// user imports never set it. Kept as a literal so the SQL filter and
// the spec stay in sync.
const SourceTypeSystem = "system"

var retiredDefaultSkillReplacements = map[string]string{
	"kandev-agent-edit":    "kandev-team-admin",
	"kandev-budget":        "kandev-team-admin",
	"kandev-config-export": "kandev-config-sync",
	"kandev-config-import": "kandev-config-sync",
	"kandev-hiring":        "kandev-team-admin",
	"kandev-task-comment":  "kandev-task-ops",
	"kandev-tasks":         "kandev-task-ops",
	"kandev-team":          "kandev-team-admin",
	"memory":               "kandev-memory",
}

// SystemSkillSpec is the parsed view of a single embedded SKILL.md
// from `apps/backend/internal/office/configloader/skills/<slug>/`.
type SystemSkillSpec struct {
	Slug            string
	Name            string
	Description     string
	Version         string
	DefaultForRoles []string
	Content         string
	FileInventory   string
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

	// ListNonSystemSkills returns every user/provider-imported row
	// (is_system = false) in the workspace. Used by the bundled-insert
	// slug-conflict pre-check (AC-003.3) and the user-skill slug
	// normalization pass (AC-003.8/AC-003.9).
	ListNonSystemSkills(ctx context.Context, workspaceID string) ([]*models.Skill, error)

	// NormalizeSkillSlug atomically changes a non-system skill's slug and
	// rewrites all matching agent desired_skills references. It returns
	// false when the row no longer matches the supplied identity, which
	// means another writer already handled or removed it.
	NormalizeSkillSlug(ctx context.Context, workspaceID, skillID, oldSlug, newSlug string) (bool, error)

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
//
// Every per-workspace list is sorted lexicographically by slug; the
// scoped `<workspace_id>:<slug>` entries in the aggregate report are
// sorted by workspace ID then slug (AC-003.5).
type SyncReport struct {
	Inserted []string
	Updated  []string
	Removed  []string

	// Blocked lists retired system skills whose configured replacement
	// was not found in this workspace this pass (e.g. its insert was
	// withheld by a slug conflict). The retired row is left in place
	// rather than deleted, so no agent silently loses the capability
	// (AC-003.1); a later pass retries once the conflict resolves.
	Blocked []string

	// Normalized lists user-skill slugs rewritten from a well-formed
	// but non-canonical form to their canonical kandev-prefixed form,
	// as "<old>->new>" entries (AC-003.8).
	Normalized []string

	// Conflicted lists slugs left untouched because normalizing (or
	// inserting a bundled row) would collide with another row's
	// current slug (AC-003.3/AC-003.9).
	Conflicted []string
}

// workspaceSyncLocks serializes SyncSystemSkills passes per
// workspace. SyncSystemSkills has multiple concurrent call sites in
// one process (lazy per-workspace sync, startup sync, config import,
// etc.); a pass that can't acquire the lock waits rather than
// skipping, so two concurrent passes over the same workspace never
// interleave their insert/retire/normalize phases (AC-003.7).
var workspaceSyncLocks sync.Map // map[string]*sync.Mutex

func lockForWorkspace(wsID string) *sync.Mutex {
	v, _ := workspaceSyncLocks.LoadOrStore(wsID, &sync.Mutex{})
	return v.(*sync.Mutex)
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
		result, err := syncWorkspace(ctx, repo, wsID, bundledBySlug, log)
		if err != nil {
			log.Error("system skill sync failed for workspace",
				zap.String("workspace_id", wsID), zap.Error(err))
			continue
		}
		report.Inserted = append(report.Inserted, scope(wsID, result.Inserted)...)
		report.Updated = append(report.Updated, scope(wsID, result.Updated)...)
		report.Removed = append(report.Removed, scope(wsID, result.Removed)...)
		report.Blocked = append(report.Blocked, scope(wsID, result.Blocked)...)
		report.Normalized = append(report.Normalized, scope(wsID, result.Normalized)...)
		report.Conflicted = append(report.Conflicted, scope(wsID, result.Conflicted)...)
	}
	sort.Strings(report.Inserted)
	sort.Strings(report.Updated)
	sort.Strings(report.Removed)
	sort.Strings(report.Blocked)
	sort.Strings(report.Normalized)
	sort.Strings(report.Conflicted)
	log.Info("system skills synced",
		zap.Int("workspaces", len(workspaceIDs)),
		zap.Int("bundled", len(specs)),
		zap.Strings("inserted", report.Inserted),
		zap.Strings("updated", report.Updated),
		zap.Strings("removed", report.Removed),
		zap.Strings("blocked", report.Blocked),
		zap.Strings("normalized", report.Normalized),
		zap.Strings("conflicted", report.Conflicted),
	)
	return report, nil
}

// workspaceSyncResult carries one workspace's sync outcome between
// syncWorkspace's phases and back to SyncSystemSkills, which scopes
// each list into the aggregate SyncReport.
type workspaceSyncResult struct {
	Inserted   []string
	Updated    []string
	Removed    []string
	Blocked    []string
	Normalized []string
	Conflicted []string
}

// syncWorkspace handles one workspace: reconcile bundled system
// skills (insert/update), retire orphaned system rows, then normalize
// non-canonical user-skill slugs. Errors propagate; the caller logs
// and continues to the next workspace so one bad row doesn't gate the
// rest. Holds the per-workspace lock for the whole pass so concurrent
// SyncSystemSkills call sites never interleave these phases
// (AC-003.7).
func syncWorkspace(
	ctx context.Context,
	repo SystemSyncRepo,
	wsID string,
	bundled map[string]SystemSkillSpec,
	log *logger.Logger,
) (workspaceSyncResult, error) {
	mu := lockForWorkspace(wsID)
	mu.Lock()
	defer mu.Unlock()

	var result workspaceSyncResult

	existing, err := repo.ListSystemSkills(ctx, wsID)
	if err != nil {
		return result, err
	}
	existingBySlug := make(map[string]*models.Skill, len(existing))
	for _, s := range existing {
		existingBySlug[s.Slug] = s
	}

	nonSystem, err := repo.ListNonSystemSkills(ctx, wsID)
	if err != nil {
		return result, fmt.Errorf("list non-system skills: %w", err)
	}
	nonSystemBySlug := make(map[string]*models.Skill, len(nonSystem))
	for _, s := range nonSystem {
		nonSystemBySlug[s.Slug] = s
	}

	if err := reconcileBundledSkills(ctx, repo, wsID, bundled, existingBySlug, nonSystemBySlug, &result); err != nil {
		return result, err
	}
	if err := retireOrphanedSystemSkills(ctx, repo, wsID, bundled, existingBySlug, &result); err != nil {
		return result, err
	}
	if err := normalizeUserSkillSlugs(ctx, repo, wsID, nonSystem, existingBySlug, nonSystemBySlug, &result, log); err != nil {
		return result, err
	}

	sort.Strings(result.Inserted)
	sort.Strings(result.Updated)
	sort.Strings(result.Removed)
	sort.Strings(result.Blocked)
	sort.Strings(result.Normalized)
	sort.Strings(result.Conflicted)
	return result, nil
}

// reconcileBundledSkills walks the bundled spec set in sorted order,
// inserting missing rows and updating drifted ones. A bundled slug
// not yet present as a system row but already held by a non-system
// row is skipped and recorded as a conflict rather than inserted
// (AC-003.3); the CreateSkill unique-constraint fallback below covers
// the same outcome for the race between this pre-check and the
// insert.
func reconcileBundledSkills(
	ctx context.Context,
	repo SystemSyncRepo,
	wsID string,
	bundled map[string]SystemSkillSpec,
	existingBySlug map[string]*models.Skill,
	nonSystemBySlug map[string]*models.Skill,
	result *workspaceSyncResult,
) error {
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
			if _, taken := nonSystemBySlug[slug]; taken {
				result.Conflicted = append(result.Conflicted, slug)
				continue
			}
			row := newSystemSkillRow(wsID, spec)
			if err := repo.CreateSkill(ctx, row); err != nil {
				if isSlugUniqueConstraintErr(err) {
					result.Conflicted = append(result.Conflicted, slug)
					continue
				}
				return fmt.Errorf("insert %s: %w", slug, err)
			}
			existingBySlug[slug] = row
			result.Inserted = append(result.Inserted, slug)
			continue
		}
		if systemSkillUpToDate(cur, spec) {
			continue
		}
		applySystemSkillUpdate(cur, spec)
		if err := repo.UpdateSkill(ctx, cur); err != nil {
			return fmt.Errorf("update %s: %w", slug, err)
		}
		result.Updated = append(result.Updated, slug)
	}
	return nil
}

// retireOrphanedSystemSkills deletes system rows whose slug is no
// longer in the bundle. A retirement with a configured replacement
// (retiredDefaultSkillReplacements) only proceeds once that
// replacement row actually exists in this workspace: deleting the
// retired row first would strand every agent that referenced it with
// no rewritten reference (AC-003.1). When the replacement is missing
// this pass (e.g. its insert was withheld by a slug conflict above),
// the retired row is left in place and reported as blocked so a later
// pass can retry once the conflict resolves.
//
// A retired row is removed from existingBySlug along with the DB row:
// normalizeUserSkillSlugs runs after this pass and treats existingBySlug
// as the set of slugs currently taken, so a stale entry for a slug this
// same pass just freed would report a false conflict for a user skill
// that could otherwise normalize onto it.
func retireOrphanedSystemSkills(
	ctx context.Context,
	repo SystemSyncRepo,
	wsID string,
	bundled map[string]SystemSkillSpec,
	existingBySlug map[string]*models.Skill,
	result *workspaceSyncResult,
) error {
	// The map is a snapshot, but the order of retirement still matters when
	// one retired slug names another as its replacement. Walk it in a stable
	// order so the outcome does not depend on Go's map iteration seed.
	slugs := make([]string, 0, len(existingBySlug))
	for slug := range existingBySlug {
		slugs = append(slugs, slug)
	}
	sort.Strings(slugs)
	for _, slug := range slugs {
		cur, present := existingBySlug[slug]
		if !present {
			continue
		}
		if _, kept := bundled[slug]; kept {
			continue
		}
		replacementSlug, hasReplacement := retiredDefaultSkillReplacements[slug]
		if hasReplacement {
			if _, bundledReplacement := bundled[replacementSlug]; !bundledReplacement {
				result.Blocked = append(result.Blocked, slug)
				continue
			}
			replacement, found := existingBySlug[replacementSlug]
			if !found {
				result.Blocked = append(result.Blocked, slug)
				continue
			}
			if err := replaceSkillOnAgents(ctx, repo, wsID, cur, replacement); err != nil {
				return fmt.Errorf("replace %s: %w", slug, err)
			}
		}
		if err := repo.DeleteSkill(ctx, cur.ID); err != nil {
			return fmt.Errorf("delete %s: %w", slug, err)
		}
		if err := detachSkillFromAgents(ctx, repo, wsID, cur.ID); err != nil {
			return fmt.Errorf("detach %s: %w", slug, err)
		}
		delete(existingBySlug, slug)
		result.Removed = append(result.Removed, slug)
	}
	return nil
}

// normalizeUserSkillSlugs rewrites a well-formed but non-canonical
// user/provider skill slug to its canonical kandev-prefixed form
// (AC-003.8). The row ID is preserved, so skill_ids needs no
// rewrite — only slug-keyed desired_skills references are updated. A
// not-well-formed slug (a legacy artifact predating write-time
// validation) is left untouched and logged, never normalized
// (AC-003.9). A normalized value already held by another row is left
// untouched on both sides and logged as a conflict.
//
// The repository performs the row rename and agent-reference rewrite in
// one transaction. This prevents a concurrent writer from claiming the
// canonical slug after the snapshot and leaving references pointed at a
// different row, while also preserving retryability after a failed write.
func normalizeUserSkillSlugs(
	ctx context.Context,
	repo SystemSyncRepo,
	wsID string,
	nonSystem []*models.Skill,
	existingBySlug map[string]*models.Skill,
	nonSystemBySlug map[string]*models.Skill,
	result *workspaceSyncResult,
	log *logger.Logger,
) error {
	taken := make(map[string]bool, len(existingBySlug)+len(nonSystemBySlug))
	for slug := range existingBySlug {
		taken[slug] = true
	}
	for slug := range nonSystemBySlug {
		taken[slug] = true
	}

	for _, row := range nonSystem {
		if skillslug.Canonical(row.Slug) {
			continue
		}
		if !skillslug.WellFormed(row.Slug) {
			log.Warn("system skill sync: leaving not-well-formed user skill slug untouched",
				zap.String("workspace_id", wsID), zap.String("skill_id", row.ID), zap.String("slug", row.Slug))
			continue
		}
		normalized := skillslug.Normalize(row.Slug)
		if taken[normalized] {
			log.Warn("system skill sync: leaving conflicting user skill slug unnormalized",
				zap.String("workspace_id", wsID), zap.String("skill_id", row.ID),
				zap.String("slug", row.Slug), zap.String("normalized_slug", normalized))
			result.Conflicted = append(result.Conflicted, row.Slug)
			continue
		}
		oldSlug := row.Slug
		changed, err := repo.NormalizeSkillSlug(ctx, wsID, row.ID, oldSlug, normalized)
		if err != nil {
			if isSlugUniqueConstraintErr(err) {
				result.Conflicted = append(result.Conflicted, oldSlug)
				continue
			}
			return fmt.Errorf("normalize slug %s: %w", oldSlug, err)
		}
		if !changed {
			continue
		}
		taken[normalized] = true
		result.Normalized = append(result.Normalized, oldSlug+"->"+normalized)
	}
	return nil
}

// isSlugUniqueConstraintErr reports whether err is a UNIQUE constraint
// violation for a skill slug. SQLite's driver requires string matching,
// while pgx exposes a typed SQLSTATE 23505 error. The PostgreSQL
// constraint-name check avoids treating an unrelated duplicate (for
// example, a primary-key collision) as a slug conflict.
func isSlugUniqueConstraintErr(err error) bool {
	if err == nil {
		return false
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23505" &&
			(pgErr.ConstraintName == "" || pgErr.ConstraintName == "office_skills_workspace_id_slug_key")
	}
	msg := err.Error()
	return strings.Contains(msg, "UNIQUE constraint failed") ||
		strings.Contains(msg, "constraint failed: UNIQUE")
}

func replaceSkillOnAgents(
	ctx context.Context,
	repo SystemSyncRepo,
	wsID string,
	retired *models.Skill,
	replacement *models.Skill,
) error {
	agents, err := repo.ListAgentInstances(ctx, wsID)
	if err != nil {
		return fmt.Errorf("list agents: %w", err)
	}
	for _, agent := range agents {
		newSkillIDs, skillIDsChanged := replaceJSONArrayValue(agent.SkillIDs, retired.ID, replacement.ID)
		newDesired, desiredChanged := replaceJSONArrayValue(agent.DesiredSkills, retired.Slug, replacement.Slug)
		if !skillIDsChanged && !desiredChanged {
			continue
		}
		if skillIDsChanged {
			agent.SkillIDs = newSkillIDs
		}
		if desiredChanged {
			agent.DesiredSkills = newDesired
		}
		if err := repo.UpdateAgentInstance(ctx, agent); err != nil {
			return fmt.Errorf("update agent %s: %w", agent.ID, err)
		}
	}
	return nil
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

func systemSkillUpToDate(cur *models.Skill, spec SystemSkillSpec) bool {
	return cur.IsSystem &&
		cur.ContentHash == spec.ContentHash &&
		cur.Content == spec.Content &&
		cur.FileInventory == normalizedFileInventory(spec.FileInventory) &&
		cur.Name == spec.Name &&
		cur.Description == spec.Description &&
		cur.Version == spec.Version &&
		cur.SystemVersion == spec.Version
}

// replaceJSONArrayValue rewrites every occurrence of oldValue to
// newValue in a JSON-array-encoded agent_profiles column, also
// de-duplicating and dropping empties. raw may also be a legacy
// comma-separated value (see ParseDesiredSlugs in injection.go, which
// reads both formats); a legacy value is parsed the same way and, if
// changed, re-encoded as the canonical JSON-array form — this is how
// a legacy desired_skills column migrates format the next time a
// rename or replacement touches one of its entries. An unchanged
// legacy value is left exactly as found (no unnecessary rewrite).
func replaceJSONArrayValue(raw, oldValue, newValue string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "null" {
		return raw, false
	}
	values, ok := parseJSONOrLegacyCSVArray(raw)
	if !ok {
		return raw, false
	}
	out := make([]string, 0, len(values))
	changed := false
	seen := make(map[string]bool, len(values))
	for _, value := range values {
		if value == oldValue {
			value = newValue
			changed = true
		}
		if value == "" || seen[value] {
			if value != "" {
				changed = true
			}
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	if !changed {
		return raw, false
	}
	encoded, err := json.Marshal(out)
	if err != nil {
		return raw, false
	}
	return string(encoded), true
}

// parseJSONOrLegacyCSVArray parses raw as a JSON string array, falling
// back to splitting it as a legacy comma-separated value when raw
// isn't a JSON array. Mirrors ParseDesiredSlugs's format detection
// (injection.go) so both readers of these columns agree on what
// counts as "legacy". Returns ok=false only for malformed JSON input
// (a corrupt column stays a safe no-op for the caller).
func parseJSONOrLegacyCSVArray(raw string) ([]string, bool) {
	if strings.HasPrefix(raw, "[") {
		var values []string
		if err := json.Unmarshal([]byte(raw), &values); err != nil {
			return nil, false
		}
		return values, true
	}
	parts := strings.Split(raw, ",")
	out := make([]string, len(parts))
	for i, p := range parts {
		out[i] = strings.TrimSpace(p)
	}
	return out, true
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
		FileInventory:   normalizedFileInventory(spec.FileInventory),
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
	cur.FileInventory = normalizedFileInventory(spec.FileInventory)
	cur.Version = spec.Version
	cur.ContentHash = spec.ContentHash
	cur.ApprovalState = "approved"
	cur.IsSystem = true
	cur.SystemVersion = spec.Version
	cur.DefaultForRoles = string(roles)
}

func normalizedFileInventory(raw string) string {
	if strings.TrimSpace(raw) == "" {
		return "[]"
	}
	return raw
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
		inventory, err := bundledSkillFileInventory(slug)
		if err != nil {
			return nil, fmt.Errorf("inventory %s: %w", slug, err)
		}
		spec.FileInventory = inventory
		spec.ContentHash = bundledSkillContentHash([]byte(spec.Content), inventory)
		out = append(out, *spec)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Slug < out[j].Slug })
	return out, nil
}

type bundledSkillInventoryFile struct {
	Path    string `json:"path"`
	Size    int64  `json:"size"`
	SHA256  string `json:"sha256"`
	Content string `json:"content,omitempty"`
}

func bundledSkillFileInventory(slug string) (string, error) {
	files, err := configloader.BundledSkillFiles(slug)
	if err != nil {
		return "", err
	}
	inventory := make([]bundledSkillInventoryFile, 0, len(files))
	for _, file := range files {
		sum := sha256.Sum256(file.Content)
		inventory = append(inventory, bundledSkillInventoryFile{
			Path:    file.Path,
			Size:    int64(len(file.Content)),
			SHA256:  hex.EncodeToString(sum[:]),
			Content: string(file.Content),
		})
	}
	data, err := json.Marshal(inventory)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func bundledSkillContentHash(content []byte, inventory string) string {
	if inventory == "[]" || strings.TrimSpace(inventory) == "" {
		sum := sha256.Sum256(content)
		return hex.EncodeToString(sum[:])
	}
	sum := sha256.Sum256([]byte(string(content) + "\x00" + inventory))
	return hex.EncodeToString(sum[:])
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

// parseSystemSkill validates a SKILL.md frontmatter block and returns
// the spec while preserving the original file content for runtime
// delivery. nil + nil signals "not a system skill" (kandev block
// missing or system = false) — the caller skips it without erroring.
func parseSystemSkill(slug string, raw []byte) (*SystemSkillSpec, error) {
	frontmatterBytes, _, ok := splitFrontmatter(raw)
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
		Content:         string(raw),
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
