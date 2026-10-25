package configsync

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/repository/sqlite"
)

const kindSkill = "skill"

// fetchedSkill is one collision-resolved, successfully parsed skill
// directory ready to apply.
type fetchedSkill struct {
	Key        string // directory name (AC-OFFICE-CONFIG-SYNC-003.1)
	SourcePath string
	Proj       SkillProjection
}

// buildFetchedSkills parses every skill directory the walk selected and
// resolves AC-OFFICE-CONFIG-SYNC-003.3's collision rule. A skill directory
// with no readable SKILL.md is not included here: one whose SKILL.md was
// present but unreadable is warned by skillFetchWarnings instead
// (AC-OFFICE-CONFIG-SYNC-003.6a/.12), and one with no SKILL.md at all is
// warned by skillMissingDefinitionWarnings — a separate function because
// AC-OFFICE-CONFIG-SYNC-004.5c places that warning in the walk phase, not
// this function's parse phase. unparsedDirs names every directory whose
// SKILL.md was readable but failed to parse, for skillDeletionExemptions'
// manifest lookup (AC-OFFICE-CONFIG-SYNC-003.12).
func buildFetchedSkills(dirs []skillFiles) (fetched []fetchedSkill, warnings, unparsedDirs []string) {
	type parsed struct {
		skill *parsedSkill
		path  string
	}
	var ok []parsed
	for _, sf := range dirs {
		ps, err := parseSkill(sf)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("skill %q: %v; skipping", sf.dirPath, err))
			unparsedDirs = append(unparsedDirs, sf.dirPath)
			continue
		}
		if ps == nil {
			continue
		}
		ok = append(ok, parsed{skill: ps, path: ps.sourcePath})
	}

	keyed := make([]keyedPath, len(ok))
	byPath := make(map[string]*parsedSkill, len(ok))
	for i, p := range ok {
		keyed[i] = keyedPath{Key: p.skill.dirName, Path: p.path}
		byPath[p.path] = p.skill
	}
	winners, collisionWarnings := resolveKeyCollisions(kindSkill, keyed)
	warnings = append(warnings, collisionWarnings...)

	for key, winnerPath := range winners {
		ps := byPath[winnerPath]
		fetched = append(fetched, fetchedSkill{
			Key:        key,
			SourcePath: winnerPath,
			Proj: SkillProjection{
				Name:          ps.name,
				Description:   ps.description,
				SourceType:    models.SkillSourceTypeInline,
				Content:       ps.content,
				FileInventory: ps.fileInventory,
			},
		})
	}
	return fetched, warnings, unparsedDirs
}

// skillFetchWarnings renders one AC-OFFICE-CONFIG-SYNC-002.4a/002.6a warning
// per unreadable SKILL.md or reference file.
func skillFetchWarnings(dirs []skillFiles) []string {
	var warnings []string
	for _, sf := range dirs {
		if sf.skillMDUnread != nil {
			warnings = append(warnings, fmt.Sprintf(
				"skill %q: SKILL.md unreadable (%s); leaving it untouched", sf.dirPath, sf.skillMDUnread.reason))
		}
		for _, u := range sf.unreadableRefs {
			warnings = append(warnings, fmt.Sprintf(
				"skill %q: reference file %q unreadable (%s); leaving it untouched", sf.dirPath, u.path, u.reason))
		}
	}
	return warnings
}

// skillDeletionExemptions implements AC-OFFICE-CONFIG-SYNC-003.6a/003.12 for
// skills, mirroring kindExemptions' manifest-source_path lookup: an
// unreadable-or-unparsed skill directory whose path matches a manifest
// entry's source_path exempts just that entity; one that matches no manifest
// entry exempts every skill's deletion sweep this run, since a directory
// whose definition cannot be read cannot say which entity — possibly renamed
// since the last run — it defines. Matching by the directory's own current
// path (as the prior, narrower exemption did) misses exactly that rename
// case: the old manifest entry's path is gone from the listing and the new
// directory's path was never recorded, so neither end matches without this
// coarse fallback.
//
// Only an unreadable or unparseable SKILL.md creates that ambiguity. An
// unreadable reference file under a directory whose SKILL.md parsed fine
// does not: the directory's identity is already known and it is applied
// normally this run, so it is not a deletion candidate regardless of
// exemption state, and treating it as one would exempt unrelated,
// genuinely-removed skills for as long as the reference stays unreadable.
func skillDeletionExemptions(dirs []skillFiles, unparsedDirs []string, manifestForKind []ManifestEntry) (exemptKeys map[string]bool, coarseExempt bool) {
	byPath := make(map[string]string, len(manifestForKind))
	for _, m := range manifestForKind {
		byPath[normalizePathFrame(m.SourcePath)] = m.EntityKey
	}
	exemptKeys = map[string]bool{}
	check := func(p string) {
		if key, ok := byPath[normalizePathFrame(p)]; ok {
			exemptKeys[key] = true
			return
		}
		coarseExempt = true
	}
	for _, sf := range dirs {
		if sf.skillMDUnread != nil {
			check(sf.dirPath)
		}
	}
	for _, p := range unparsedDirs {
		check(p)
	}
	return exemptKeys, coarseExempt
}

// skillMissingDefinitionWarnings renders one AC-OFFICE-CONFIG-SYNC-003.2a
// walk-phase warning per skill directory with no SKILL.md at all. A
// directory whose SKILL.md exists but could not be read is not named here —
// skillFetchWarnings already warns that case in the fetch phase.
func skillMissingDefinitionWarnings(dirs []skillFiles) []string {
	var warnings []string
	for _, sf := range dirs {
		if sf.skillMD == nil && sf.skillMDUnread == nil {
			warnings = append(warnings, fmt.Sprintf(
				"skill directory %q: no %s found; defining no skill for it", sf.dirPath, skillDefinitionName))
		}
	}
	return warnings
}

// applySkills runs the six-case table for the skill kind. Skills use a
// bespoke apply path rather than the generic engine because their writer
// (skillwriter.go) needs a CAS guard on source_locator that the generic
// entityOps.update signature has no room for (AC-OFFICE-CONFIG-SYNC-003.5d,
// R5-F1), and because SkillProjection deliberately excludes content_hash and
// source_locator from the change-comparison (AC-OFFICE-CONFIG-SYNC-003.5e).
func applySkills(
	ctx context.Context, repo *sqlite.Repository, store *Store, workspaceID string,
	fetched []fetchedSkill, manifest []ManifestEntry, exemptKeys map[string]bool, coarseExempt bool,
) (*kindApplyResult, error) {
	existing, err := repo.ListSkills(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("list existing skill entities: %w", err)
	}
	existingByKey := make(map[string]*models.Skill, len(existing))
	existingByID := make(map[string]*models.Skill, len(existing))
	for _, s := range existing {
		existingByKey[s.Slug] = s
		existingByID[s.ID] = s
	}
	manifestByKey := make(map[string]ManifestEntry, len(manifest))
	for _, m := range manifest {
		manifestByKey[m.EntityKey] = m
	}
	fetchedByKey := make(map[string]fetchedSkill, len(fetched))
	for _, f := range fetched {
		fetchedByKey[f.Key] = f
	}

	res := newKindApplyResult()
	if err := applySkillCreatesAndUpdates(ctx, repo, store, workspaceID, fetched, manifestByKey, existingByKey, existingByID, res); err != nil {
		return nil, err
	}
	if err := applySkillDeletions(ctx, repo, store, workspaceID, fetchedByKey, manifest, existingByID, exemptKeys, coarseExempt, res); err != nil {
		return res, err
	}
	return res, nil
}

// applySkillsCreatesOnly is applySkills's forward half, split out so the
// orchestrator can run every kind's creates/updates before any kind's
// deletions (AC-OFFICE-CONFIG-SYNC-003.9's fixed kind order — skills first).
func applySkillsCreatesOnly(
	ctx context.Context, repo *sqlite.Repository, store *Store, workspaceID string,
	fetched []fetchedSkill, manifest []ManifestEntry,
) (*kindApplyResult, error) {
	existing, err := repo.ListSkills(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("list existing skill entities: %w", err)
	}
	existingByKey := make(map[string]*models.Skill, len(existing))
	existingByID := make(map[string]*models.Skill, len(existing))
	for _, s := range existing {
		existingByKey[s.Slug] = s
		existingByID[s.ID] = s
	}
	manifestByKey := make(map[string]ManifestEntry, len(manifest))
	for _, m := range manifest {
		manifestByKey[m.EntityKey] = m
	}

	res := newKindApplyResult()
	if err := applySkillCreatesAndUpdates(ctx, repo, store, workspaceID, fetched, manifestByKey, existingByKey, existingByID, res); err != nil {
		return res, err
	}
	return res, nil
}

// applySkillsDeletesOnly is applySkills's reverse half — the last kind to run
// in AC-OFFICE-CONFIG-SYNC-003.9's reverse deletion order.
func applySkillsDeletesOnly(
	ctx context.Context, repo *sqlite.Repository, store *Store, workspaceID string,
	fetched []fetchedSkill, manifest []ManifestEntry, exemptKeys map[string]bool, coarseExempt bool, res *kindApplyResult,
) error {
	existing, err := repo.ListSkills(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("list existing skill entities: %w", err)
	}
	existingByID := make(map[string]*models.Skill, len(existing))
	for _, s := range existing {
		existingByID[s.ID] = s
	}
	fetchedByKey := make(map[string]fetchedSkill, len(fetched))
	for _, f := range fetched {
		fetchedByKey[f.Key] = f
	}
	return applySkillDeletions(ctx, repo, store, workspaceID, fetchedByKey, manifest, existingByID, exemptKeys, coarseExempt, res)
}

func applySkillCreatesAndUpdates(
	ctx context.Context, repo *sqlite.Repository, store *Store, workspaceID string,
	fetched []fetchedSkill, manifestByKey map[string]ManifestEntry,
	existingByKey, existingByID map[string]*models.Skill, res *kindApplyResult,
) error {
	ordered := append([]fetchedSkill(nil), fetched...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].SourcePath < ordered[j].SourcePath })

	for _, fs := range ordered {
		manifestEntry, inManifest := manifestByKey[fs.Key]
		manifestEntityExists := inManifest && existingByID[manifestEntry.EntityID] != nil
		existingSkillRow, hasExistingRow := existingByKey[fs.Key]
		var trackedID string
		if inManifest {
			trackedID = manifestEntry.EntityID
		}
		unmanagedHoldsKey := hasExistingRow && existingSkillRow.ID != trackedID

		switch decideKey(true, inManifest, manifestEntityExists, unmanagedHoldsKey, false) {
		case decisionForeign:
			res.Warnings = append(res.Warnings, fmt.Sprintf(
				"skill %q: an unmanaged entity already uses this name; leaving both untouched", fs.Key))
		case decisionNew:
			id, err := createSkillEntity(ctx, repo, store, workspaceID, fs)
			if err != nil {
				return err
			}
			res.Created = append(res.Created, fs.Key)
			res.IDsByKey[fs.Key] = id
		case decisionExisting:
			existingSkill := existingByID[manifestEntry.EntityID]
			changed, warn, err := updateSkillEntityIfChanged(ctx, repo, store, workspaceID, fs, existingSkill, manifestEntry.SourcePath)
			if err != nil {
				return err
			}
			if warn != "" {
				res.Warnings = append(res.Warnings, warn)
			} else if changed {
				res.Updated = append(res.Updated, fs.Key)
			}
			res.IDsByKey[fs.Key] = existingSkill.ID
		}
	}
	return nil
}

func createSkillEntity(ctx context.Context, repo *sqlite.Repository, store *Store, workspaceID string, fs fetchedSkill) (string, error) {
	tx, err := repo.Writer().BeginTxx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback() }()

	id, err := CreateSkill(ctx, tx, repo.Writer(), workspaceID, fs.Key, fs.SourcePath, fs.Proj)
	if err != nil {
		return "", fmt.Errorf("create skill %q: %w", fs.Key, err)
	}
	if err := store.UpsertManifestEntryTx(ctx, tx, workspaceID, kindSkill, fs.Key, id, fs.SourcePath); err != nil {
		return "", fmt.Errorf("record manifest for skill %q: %w", fs.Key, err)
	}
	if err := tx.Commit(); err != nil {
		return "", err
	}
	return id, nil
}

// skillProjectionOf reads an existing skill row's owned-field projection —
// the same shape SkillProjection uses — for change comparison.
func skillProjectionOf(s *models.Skill) SkillProjection {
	return SkillProjection{
		Name:          s.Name,
		Description:   s.Description,
		SourceType:    s.SourceType,
		Content:       s.Content,
		FileInventory: s.FileInventory,
	}
}

// updateSkillEntityIfChanged writes fs onto an existing skill only when its
// owned projection differs. A CAS failure (ErrSkillLocatorChanged) is not an
// error for the run: it returns a warning naming the skill and leaves the
// row untouched for the next run to reconcile (AC-OFFICE-CONFIG-SYNC-003.5e).
func updateSkillEntityIfChanged(
	ctx context.Context, repo *sqlite.Repository, store *Store, workspaceID string,
	fs fetchedSkill, existing *models.Skill, oldSourcePath string,
) (changed bool, warning string, err error) {
	changed = skillProjectionOf(existing) != fs.Proj
	if !changed && oldSourcePath == fs.SourcePath {
		return false, "", nil
	}
	tx, err := repo.Writer().BeginTxx(ctx, nil)
	if err != nil {
		return false, "", err
	}
	defer func() { _ = tx.Rollback() }()

	if changed {
		if uerr := UpdateSkillProjection(ctx, tx, repo.Writer(), existing.ID, existing.SourceLocator, fs.Proj); uerr != nil {
			if errors.Is(uerr, ErrSkillLocatorChanged) {
				return false, fmt.Sprintf(
					"skill %q: another writer changed it concurrently; leaving untouched this run", fs.Key), nil
			}
			return false, "", fmt.Errorf("update skill %q: %w", fs.Key, uerr)
		}
	}
	if merr := store.UpsertManifestEntryTx(ctx, tx, workspaceID, kindSkill, fs.Key, existing.ID, fs.SourcePath); merr != nil {
		return false, "", fmt.Errorf("refresh manifest for skill %q: %w", fs.Key, merr)
	}
	if cerr := tx.Commit(); cerr != nil {
		return false, "", cerr
	}
	return changed, "", nil
}

func applySkillDeletions(
	ctx context.Context, repo *sqlite.Repository, store *Store, workspaceID string,
	fetchedByKey map[string]fetchedSkill, manifest []ManifestEntry,
	existingByID map[string]*models.Skill, exemptKeys map[string]bool, coarseExempt bool, res *kindApplyResult,
) error {
	ordered := append([]ManifestEntry(nil), manifest...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].EntityKey < ordered[j].EntityKey })

	for _, m := range ordered {
		if _, inFetched := fetchedByKey[m.EntityKey]; inFetched {
			continue
		}
		_, manifestEntityExists := existingByID[m.EntityID]
		exempt := coarseExempt || exemptKeys[m.EntityKey]

		switch decideKey(false, true, manifestEntityExists, false, exempt) {
		case decisionGoneOutOfBand:
			if err := store.DeleteManifestEntry(ctx, workspaceID, kindSkill, m.EntityKey); err != nil {
				return fmt.Errorf("drop stale manifest entry for skill %q: %w", m.EntityKey, err)
			}
		case decisionExempt:
			res.Warnings = append(res.Warnings, fmt.Sprintf(
				"skill %q: could not confirm removal (an unreadable file may be a renamed or broken version of it); leaving it in place",
				m.EntityKey))
		case decisionRemovedUpstream:
			if err := deleteSkillEntity(ctx, repo, store, workspaceID, m); err != nil {
				return fmt.Errorf("delete skill %q: %w", m.EntityKey, err)
			}
			res.Deleted = append(res.Deleted, m.EntityKey)
		}
	}
	return nil
}

func deleteSkillEntity(ctx context.Context, repo *sqlite.Repository, store *Store, workspaceID string, m ManifestEntry) error {
	tx, err := repo.Writer().BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if err := repo.DeleteSkillTx(ctx, tx, m.EntityID); err != nil {
		return err
	}
	if err := store.DeleteManifestEntryTx(ctx, tx, workspaceID, kindSkill, m.EntityKey); err != nil {
		return err
	}
	return tx.Commit()
}
