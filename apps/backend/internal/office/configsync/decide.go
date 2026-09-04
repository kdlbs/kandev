package configsync

// applyDecision is the outcome of classifying one (kind, key) against the
// fetched set and the applied manifest — the six-case table of
// AC-OFFICE-CONFIG-SYNC-003.4 through AC-OFFICE-CONFIG-SYNC-003.8, plus
// AC-OFFICE-CONFIG-SYNC-003.15's "gone out of band" case.
type applyDecision int

const (
	// decisionSkip means the key is in neither the fetched set nor the
	// manifest — nothing in this run concerns it.
	decisionSkip applyDecision = iota
	// decisionNew means an entity shall be created and added to the
	// manifest (AC-OFFICE-CONFIG-SYNC-003.4).
	decisionNew
	// decisionExisting means the owned projection shall be written to the
	// fetched definition if it differs (AC-OFFICE-CONFIG-SYNC-003.5).
	decisionExisting
	// decisionForeign means the key collides with an unmanaged entity:
	// neither modified nor adopted (AC-OFFICE-CONFIG-SYNC-003.7).
	decisionForeign
	// decisionRemovedUpstream means the entity shall be deleted and its
	// manifest row removed (AC-OFFICE-CONFIG-SYNC-003.6).
	decisionRemovedUpstream
	// decisionExempt means a manifest entry that would otherwise be
	// removed-upstream is left untouched because this run could not read
	// or parse a file that might define it (AC-OFFICE-CONFIG-SYNC-003.6a)
	// or because AC-OFFICE-CONFIG-SYNC-003.12 exempted it by path.
	decisionExempt
	// decisionGoneOutOfBand means the manifest names an entity that no
	// longer exists; the manifest row is dropped and the run continues
	// (AC-OFFICE-CONFIG-SYNC-003.15).
	decisionGoneOutOfBand
)

// decideKey classifies one (kind, key) pair. manifestEntityExists is
// meaningless when inManifest is false. unmanagedHoldsKey means some
// existing entity other than the manifest's own tracked one already holds
// this key; it is meaningless unless inFetched && (!inManifest ||
// !manifestEntityExists). exempt is meaningless unless !inFetched &&
// inManifest && manifestEntityExists.
//
// A key both fetched and manifest-recorded whose manifest entity no longer
// exists is not one of the specification's six rows — that combination
// requires an out-of-band delete of an entity a file *still defines* this
// run, which the requirements do not name. This function folds it into
// decisionGoneOutOfBand immediately followed by re-evaluating the key as
// unmanifested: if a different existing entity now holds the key, that is
// AC-OFFICE-CONFIG-SYNC-003.7's Foreign collision, not a fresh create.
// Otherwise the stale manifest row is simply replaced by a freshly created
// entity.
func decideKey(inFetched, inManifest, manifestEntityExists, unmanagedHoldsKey, exempt bool) applyDecision {
	switch {
	case inFetched && !inManifest:
		if unmanagedHoldsKey {
			return decisionForeign
		}
		return decisionNew
	case inFetched && inManifest:
		if !manifestEntityExists {
			if unmanagedHoldsKey {
				return decisionForeign
			}
			return decisionNew
		}
		return decisionExisting
	case !inFetched && inManifest:
		if !manifestEntityExists {
			return decisionGoneOutOfBand
		}
		if exempt {
			return decisionExempt
		}
		return decisionRemovedUpstream
	default:
		return decisionSkip
	}
}
