package process

import "github.com/google/uuid"

// turnCheckpointBindingState is the one live generated-branch identity this
// checkout operator observed. Capture and guarded renames maintain it under
// turnCheckpointBindingMu; accepted receipts remain the immutable comparison
// source, so retries never restore this state from an old commit body.
type turnCheckpointBindingState struct {
	ref string
	id  string
}

func validTurnCheckpointBindingID(id string) bool {
	parsed, err := uuid.Parse(id)
	return err == nil && parsed.String() == id
}

// observeTurnCheckpointBinding returns the live binding identity for the
// observed symbolic ref, creating a fresh UUID whenever no binding exists or
// the checkout moved without a verified generated rename.
func (g *GitOperator) observeTurnCheckpointBinding(ref string) string {
	if ref == "" {
		g.clearTurnCheckpointBinding()
		return ""
	}
	g.turnCheckpointBindingMu.Lock()
	defer g.turnCheckpointBindingMu.Unlock()
	if g.turnCheckpointBinding.ref == ref && validTurnCheckpointBindingID(g.turnCheckpointBinding.id) {
		return g.turnCheckpointBinding.id
	}
	id := uuid.NewString()
	g.turnCheckpointBinding = turnCheckpointBindingState{ref: ref, id: id}
	return id
}

func (g *GitOperator) clearTurnCheckpointBinding() {
	g.turnCheckpointBindingMu.Lock()
	defer g.turnCheckpointBindingMu.Unlock()
	g.turnCheckpointBinding = turnCheckpointBindingState{}
}

// transferTurnCheckpointBinding carries the previous identity across a verified
// generated rename. A mismatched or missing live binding produces a fresh UUID
// so the rename cannot inherit continuity from an earlier checkout state.
func (g *GitOperator) transferTurnCheckpointBinding(expectedRef, newRef string) {
	if newRef == "" {
		g.clearTurnCheckpointBinding()
		return
	}
	g.turnCheckpointBindingMu.Lock()
	defer g.turnCheckpointBindingMu.Unlock()
	id := uuid.NewString()
	if g.turnCheckpointBinding.ref == expectedRef && validTurnCheckpointBindingID(g.turnCheckpointBinding.id) {
		id = g.turnCheckpointBinding.id
	}
	g.turnCheckpointBinding = turnCheckpointBindingState{ref: newRef, id: id}
}

// resetTurnCheckpointBinding replaces any live identity with a fresh one for the
// observed ref. An unsafe or partial operation must never keep older continuity.
func (g *GitOperator) resetTurnCheckpointBinding(ref string) {
	if ref == "" {
		g.clearTurnCheckpointBinding()
		return
	}
	g.turnCheckpointBindingMu.Lock()
	defer g.turnCheckpointBindingMu.Unlock()
	g.turnCheckpointBinding = turnCheckpointBindingState{ref: ref, id: uuid.NewString()}
}
