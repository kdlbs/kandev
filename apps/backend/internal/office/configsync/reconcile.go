package configsync

import (
	"context"
	"fmt"
	"sort"

	"github.com/jmoiron/sqlx"
)

// entityRow is one already-persisted entity of a given kind, as read at the
// start of an apply pass — managed or not (AC-OFFICE-CONFIG-SYNC-003.7's
// Foreign case needs to see unmanaged rows too).
type entityRow[P comparable] struct {
	ID         string
	Key        string // declared name (agent/project/routine) or slug (skill)
	Projection P
}

// entityOps supplies one kind's reads and writes to applyKind, so the
// six-case table (AC-OFFICE-CONFIG-SYNC-003.4 through .8) runs once instead
// of once per kind.
type entityOps[P comparable] struct {
	kind string
	// list returns every entity of this kind currently in the workspace,
	// managed or not.
	list func(ctx context.Context, workspaceID string) ([]entityRow[P], error)
	// create inserts a brand-new entity for key, seeded from proj, and
	// returns its new ID.
	create func(ctx context.Context, tx *sqlx.Tx, workspaceID, key, sourcePath string, proj P) (string, error)
	// update writes proj's owned fields onto the entity id already has.
	update func(ctx context.Context, tx *sqlx.Tx, id string, proj P) error
	// del deletes the entity by id.
	del func(ctx context.Context, tx *sqlx.Tx, id string) error
}

// fetchedEntity is one collision-resolved, successfully parsed definition
// ready to apply (AC-OFFICE-CONFIG-SYNC-003.3 already applied).
type fetchedEntity[P comparable] struct {
	Key        string
	SourcePath string
	Projection P
}

// kindApplyResult is one kind's contribution to a run's SyncResult.
type kindApplyResult struct {
	Created, Updated, Deleted []string
	Warnings                  []string
	// IDsByKey maps every key created or confirmed existing this run
	// (decisionNew/decisionExisting) to its entity ID, so a later pass
	// (the agent kind's reports_to second pass) can resolve names to IDs
	// without a second read.
	IDsByKey map[string]string
	// ForeignKeys names every key this run fetched but left untouched
	// because an unmanaged entity already held it (decisionForeign). The
	// agent kind's reports_to second pass folds this into unappliedKeys so
	// a reference to one of these names gets AC-OFFICE-CONFIG-SYNC-003.10b's
	// "fetched but not applied" warning instead of the generic "not managed
	// by this sync" one a name that appears nowhere gets.
	ForeignKeys map[string]bool
	// DeletedIDs carries the entity ID alongside every key in Deleted, in
	// the same order, for the agent kind's post-delete session-termination
	// cascade (Runner.terminateDeletedAgentSessions) — the only kind that
	// needs the ID of what it just deleted rather than only its name.
	DeletedIDs []string
}

func newKindApplyResult() *kindApplyResult {
	return &kindApplyResult{IDsByKey: map[string]string{}, ForeignKeys: map[string]bool{}}
}

// applyKind runs the six-case table for one kind. fetched is the
// collision-resolved, successfully parsed set; exemptKeys names
// manifest-only keys AC-OFFICE-CONFIG-SYNC-003.12 exempts by path;
// coarseExempt is AC-OFFICE-CONFIG-SYNC-003.6a's kind-wide "skip every
// deletion this run" switch, set when at least one unreadable file of this
// kind couldn't be attributed to any manifest entry by path.
func applyKind[P comparable](
	ctx context.Context, writer *sqlx.DB, store *Store, ops entityOps[P],
	workspaceID string, fetched []fetchedEntity[P], manifest []ManifestEntry,
	exemptKeys map[string]bool, coarseExempt bool,
) (*kindApplyResult, error) {
	existing, err := ops.list(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("list existing %s entities: %w", ops.kind, err)
	}
	existingByKey := make(map[string]entityRow[P], len(existing))
	existingByID := make(map[string]entityRow[P], len(existing))
	for _, e := range existing {
		existingByKey[e.Key] = e
		existingByID[e.ID] = e
	}
	manifestByKey := make(map[string]ManifestEntry, len(manifest))
	for _, m := range manifest {
		manifestByKey[m.EntityKey] = m
	}
	fetchedByKey := make(map[string]fetchedEntity[P], len(fetched))
	for _, f := range fetched {
		fetchedByKey[f.Key] = f
	}

	res := newKindApplyResult()
	if err := applyKindCreatesAndUpdates(
		ctx, writer, store, ops, workspaceID, fetched, manifestByKey, existingByKey, existingByID, res,
	); err != nil {
		return nil, err
	}
	if err := applyKindDeletions(
		ctx, writer, store, ops, workspaceID, fetchedByKey, manifest, existingByID, exemptKeys, coarseExempt, res,
	); err != nil {
		return nil, err
	}
	return res, nil
}

// applyKindCreatesOnly runs the forward half of one kind's apply pass
// (decisionNew, decisionExisting, decisionForeign). It exists so a caller
// juggling all four kinds (AC-OFFICE-CONFIG-SYNC-003.9's fixed kind order:
// skills, agents, projects, routines forward; reverse for deletions) can run
// every kind's creates/updates before any kind's deletions, rather than
// finishing one kind's full six-case pass before starting the next.
func applyKindCreatesOnly[P comparable](
	ctx context.Context, writer *sqlx.DB, store *Store, ops entityOps[P],
	workspaceID string, fetched []fetchedEntity[P], manifest []ManifestEntry,
) (*kindApplyResult, error) {
	existing, err := ops.list(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("list existing %s entities: %w", ops.kind, err)
	}
	existingByKey := make(map[string]entityRow[P], len(existing))
	existingByID := make(map[string]entityRow[P], len(existing))
	for _, e := range existing {
		existingByKey[e.Key] = e
		existingByID[e.ID] = e
	}
	manifestByKey := make(map[string]ManifestEntry, len(manifest))
	for _, m := range manifest {
		manifestByKey[m.EntityKey] = m
	}

	res := newKindApplyResult()
	if err := applyKindCreatesAndUpdates(
		ctx, writer, store, ops, workspaceID, fetched, manifestByKey, existingByKey, existingByID, res,
	); err != nil {
		return res, err
	}
	return res, nil
}

// applyKindDeletesOnly runs the reverse half of one kind's apply pass
// (decisionRemovedUpstream, decisionExempt, decisionGoneOutOfBand), appending
// to a result a prior applyKindCreatesOnly call for the same kind produced.
// It re-lists existing entities rather than reusing applyKindCreatesOnly's
// snapshot: by the time deletions run for this kind, every other kind's
// creates/updates in this run have already committed, and re-listing is the
// only way to see a manifest entry's entity if this kind's own creates phase
// just wrote it (AC-OFFICE-CONFIG-SYNC-003.9's ordering exists across kinds,
// not to let this kind's own read go stale).
func applyKindDeletesOnly[P comparable](
	ctx context.Context, writer *sqlx.DB, store *Store, ops entityOps[P],
	workspaceID string, fetched []fetchedEntity[P], manifest []ManifestEntry,
	exemptKeys map[string]bool, coarseExempt bool, res *kindApplyResult,
) error {
	existing, err := ops.list(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("list existing %s entities: %w", ops.kind, err)
	}
	existingByID := make(map[string]entityRow[P], len(existing))
	for _, e := range existing {
		existingByID[e.ID] = e
	}
	fetchedByKey := make(map[string]fetchedEntity[P], len(fetched))
	for _, f := range fetched {
		fetchedByKey[f.Key] = f
	}
	return applyKindDeletions(ctx, writer, store, ops, workspaceID, fetchedByKey, manifest, existingByID, exemptKeys, coarseExempt, res)
}

// applyKindCreatesAndUpdates handles every fetched key (decisionNew,
// decisionExisting, decisionForeign), in ascending source_path byte-wise
// order (AC-OFFICE-CONFIG-SYNC-003.9d).
func applyKindCreatesAndUpdates[P comparable](
	ctx context.Context, writer *sqlx.DB, store *Store, ops entityOps[P],
	workspaceID string, fetched []fetchedEntity[P],
	manifestByKey map[string]ManifestEntry, existingByKey, existingByID map[string]entityRow[P],
	res *kindApplyResult,
) error {
	ordered := append([]fetchedEntity[P](nil), fetched...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].SourcePath < ordered[j].SourcePath })

	for _, fe := range ordered {
		manifestEntry, inManifest := manifestByKey[fe.Key]
		manifestEntityExists := inManifest && func() bool {
			_, ok := existingByID[manifestEntry.EntityID]
			return ok
		}()
		existingRow, hasExistingRow := existingByKey[fe.Key]
		var trackedID string
		if inManifest {
			trackedID = manifestEntry.EntityID
		}
		unmanagedHoldsKey := hasExistingRow && existingRow.ID != trackedID

		switch decideKey(true, inManifest, manifestEntityExists, unmanagedHoldsKey, false) {
		case decisionForeign:
			res.Warnings = append(res.Warnings, fmt.Sprintf(
				"%s %q: an unmanaged entity already uses this name; leaving both untouched", ops.kind, fe.Key))
			res.ForeignKeys[fe.Key] = true
		case decisionNew:
			id, err := applyCreate(ctx, writer, store, ops, workspaceID, fe)
			if err != nil {
				return err
			}
			res.Created = append(res.Created, fe.Key)
			res.IDsByKey[fe.Key] = id
		case decisionExisting:
			existingRow := existingByID[manifestEntry.EntityID]
			changed, err := applyUpdateIfChanged(ctx, writer, store, ops, workspaceID, fe, existingRow, manifestEntry.SourcePath)
			if err != nil {
				return err
			}
			if changed {
				res.Updated = append(res.Updated, fe.Key)
			}
			res.IDsByKey[fe.Key] = existingRow.ID
		}
	}
	return nil
}

// applyKindDeletions handles every manifest-only key not in this run's
// fetched set (decisionRemovedUpstream, decisionExempt,
// decisionGoneOutOfBand), in ascending entity_key byte-wise order
// (AC-OFFICE-CONFIG-SYNC-003.9e).
func applyKindDeletions[P comparable](
	ctx context.Context, writer *sqlx.DB, store *Store, ops entityOps[P],
	workspaceID string, fetchedByKey map[string]fetchedEntity[P], manifest []ManifestEntry,
	existingByID map[string]entityRow[P], exemptKeys map[string]bool, coarseExempt bool,
	res *kindApplyResult,
) error {
	ordered := append([]ManifestEntry(nil), manifest...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].EntityKey < ordered[j].EntityKey })

	for _, m := range ordered {
		if _, inFetched := fetchedByKey[m.EntityKey]; inFetched {
			continue // handled by applyKindCreatesAndUpdates
		}
		_, manifestEntityExists := existingByID[m.EntityID]
		exempt := coarseExempt || exemptKeys[m.EntityKey]

		switch decideKey(false, true, manifestEntityExists, false, exempt) {
		case decisionGoneOutOfBand:
			if err := store.DeleteManifestEntry(ctx, workspaceID, ops.kind, m.EntityKey); err != nil {
				return fmt.Errorf("drop stale manifest entry for %s %q: %w", ops.kind, m.EntityKey, err)
			}
		case decisionExempt:
			res.Warnings = append(res.Warnings, fmt.Sprintf(
				"%s %q: could not confirm removal (an unreadable file may be a renamed or broken version of it); leaving it in place",
				ops.kind, m.EntityKey))
		case decisionRemovedUpstream:
			if err := applyDelete(ctx, writer, store, ops, workspaceID, m); err != nil {
				return err
			}
			res.Deleted = append(res.Deleted, m.EntityKey)
			res.DeletedIDs = append(res.DeletedIDs, m.EntityID)
		}
	}
	return nil
}

// applyCreate inserts a new entity and its manifest row in one transaction
// (AC-OFFICE-CONFIG-SYNC-003.14).
func applyCreate[P comparable](
	ctx context.Context, writer *sqlx.DB, store *Store, ops entityOps[P],
	workspaceID string, fe fetchedEntity[P],
) (string, error) {
	tx, err := writer.BeginTxx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback() }()

	id, err := ops.create(ctx, tx, workspaceID, fe.Key, fe.SourcePath, fe.Projection)
	if err != nil {
		return "", fmt.Errorf("create %s %q: %w", ops.kind, fe.Key, err)
	}
	if err := store.UpsertManifestEntryTx(ctx, tx, workspaceID, ops.kind, fe.Key, id, fe.SourcePath); err != nil {
		return "", fmt.Errorf("record manifest for %s %q: %w", ops.kind, fe.Key, err)
	}
	if err := tx.Commit(); err != nil {
		return "", err
	}
	return id, nil
}

// applyUpdateIfChanged writes proj onto an existing entity only when its
// owned projection actually differs from what is fetched
// (AC-OFFICE-CONFIG-SYNC-003.5b); the manifest's source_path is refreshed
// either way, since a file may have moved within its kind's directory
// without any owned field changing.
func applyUpdateIfChanged[P comparable](
	ctx context.Context, writer *sqlx.DB, store *Store, ops entityOps[P],
	workspaceID string, fe fetchedEntity[P], existingRow entityRow[P], oldSourcePath string,
) (bool, error) {
	changed := existingRow.Projection != fe.Projection
	if !changed && oldSourcePath == fe.SourcePath {
		return false, nil
	}
	tx, err := writer.BeginTxx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()

	if changed {
		if err := ops.update(ctx, tx, existingRow.ID, fe.Projection); err != nil {
			return false, fmt.Errorf("update %s %q: %w", ops.kind, fe.Key, err)
		}
	}
	if err := store.UpsertManifestEntryTx(ctx, tx, workspaceID, ops.kind, fe.Key, existingRow.ID, fe.SourcePath); err != nil {
		return false, fmt.Errorf("refresh manifest for %s %q: %w", ops.kind, fe.Key, err)
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return changed, nil
}

// applyDelete removes an entity and its manifest row in one transaction.
func applyDelete[P comparable](
	ctx context.Context, writer *sqlx.DB, store *Store, ops entityOps[P], workspaceID string, m ManifestEntry,
) error {
	tx, err := writer.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if err := ops.del(ctx, tx, m.EntityID); err != nil {
		return fmt.Errorf("delete %s %q: %w", ops.kind, m.EntityKey, err)
	}
	if err := store.DeleteManifestEntryTx(ctx, tx, workspaceID, ops.kind, m.EntityKey); err != nil {
		return fmt.Errorf("drop manifest entry for %s %q: %w", ops.kind, m.EntityKey, err)
	}
	return tx.Commit()
}
