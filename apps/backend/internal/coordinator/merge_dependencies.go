package coordinator

import (
	"sort"

	"github.com/kandev/kandev/internal/coordinator/mergestate"
)

// DepRef names one predecessor: a task ID or a canonical change key.
type DepRef struct {
	Task   string `json:"task,omitempty"`
	Change string `json:"change,omitempty"`
}

func (r DepRef) valid() bool { return (r.Task == "") != (r.Change == "") }

// DepSet is a card's complete predecessor set and its revision.
type DepSet struct {
	Refs     []DepRef `json:"refs"`
	Revision int      `json:"revision"`
}

// Dependency refusal codes.
const (
	codeDependencyInvalid   = "dependency_invalid"
	codeDependencyDuplicate = "dependency_duplicate"
	codeDependencySelf      = "dependency_self"
	codeDependencyUnknown   = "dependency_unknown"
	maxDependencyRefs       = 50
	fieldDepRefs            = "refs"
)

// depGraph is the state the dependency rules read: the coordinator's sets, the
// live tasks' linked changes and which change keys any live task links.
type depGraph struct {
	deps    map[string][]DepRef // card task ID to its set
	changes map[string][]string // live task ID to its linked change keys
	linked  map[string]bool     // change keys linked by some live task
}

// resolve returns the change key a reference names. ok is false when it does
// not resolve: a deleted or archived task, a task with no or several linked
// changes, or a change no live task links.
func (g depGraph) resolve(r DepRef) (string, bool) {
	if r.Change != "" {
		return r.Change, g.linked[r.Change]
	}
	keys := g.changes[r.Task]
	if len(keys) != 1 {
		return "", false
	}
	return keys[0], true
}

// ownChange is the card's change when it has exactly one.
func (g depGraph) ownChange(card string) (string, bool) {
	keys := g.changes[card]
	if len(keys) != 1 {
		return "", false
	}
	return keys[0], true
}

// edges maps each change key that is a card's only change to the change keys
// its set resolves to.
func (g depGraph) edges() map[string][]string {
	out := map[string][]string{}
	for card, refs := range g.deps {
		from, ok := g.ownChange(card)
		if !ok {
			continue
		}
		for _, r := range refs {
			if to, ok := g.resolve(r); ok {
				out[from] = append(out[from], to)
			}
		}
	}
	return out
}

// inCycle reports whether the change key can reach itself.
func inCycle(edges map[string][]string, start string) bool {
	seen := map[string]bool{}
	stack := append([]string(nil), edges[start]...)
	for len(stack) > 0 {
		n := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if n == start {
			return true
		}
		if seen[n] {
			continue
		}
		seen[n] = true
		stack = append(stack, edges[n]...)
	}
	return false
}

// Predecessors resolves the card's set to change keys sorted by provider,
// host, repository ID and number. blocked is true when any reference does not
// resolve, names the card's own change, or the card is in a cycle; a blocked
// card is never ready, whatever the other predecessors are.
func (g depGraph) Predecessors(card string) (keys []mergestate.ChangeKey, blocked bool) {
	own, hasOwn := g.ownChange(card)
	if hasOwn && inCycle(g.edges(), own) {
		blocked = true
	}
	seen := map[string]bool{}
	for _, r := range g.deps[card] {
		k, ok := g.resolve(r)
		if !ok || (hasOwn && k == own) {
			blocked = true
			continue
		}
		if seen[k] {
			continue
		}
		seen[k] = true
		if ck, ok := mergestate.ParseChangeKey(k); ok {
			keys = append(keys, ck)
		} else {
			blocked = true
		}
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i].Less(keys[j]) })
	return keys, blocked
}

// validateDepRefs checks a save's references against the live graph: shape,
// duplicates (two references naming one change are duplicates), self
// references and references unknown in the workspace.
func validateDepRefs(card string, refs []DepRef, g depGraph) *SettingsError {
	if len(refs) > maxDependencyRefs {
		return &SettingsError{Code: codeDependencyInvalid, Field: fieldDepRefs, Message: "too many predecessors"}
	}
	own, hasOwn := g.ownChange(card)
	seen := map[string]bool{}
	for _, r := range refs {
		if !r.valid() {
			return &SettingsError{Code: codeDependencyInvalid, Field: fieldDepRefs, Message: "each reference names exactly one task or change"}
		}
		if r.Task == card {
			return &SettingsError{Code: codeDependencySelf, Field: fieldDepRefs, Message: "a card cannot depend on itself"}
		}
		if r.Change != "" {
			if _, ok := mergestate.ParseChangeKey(r.Change); !ok {
				return &SettingsError{Code: codeDependencyInvalid, Field: fieldDepRefs, Message: "malformed change reference"}
			}
		}
		key, ok := g.resolve(r)
		if !ok {
			return &SettingsError{Code: codeDependencyUnknown, Field: fieldDepRefs, Message: "reference does not resolve to one change of this workspace"}
		}
		if hasOwn && key == own {
			return &SettingsError{Code: codeDependencySelf, Field: fieldDepRefs, Message: "a card cannot depend on its own change"}
		}
		if seen[key] {
			return &SettingsError{Code: codeDependencyDuplicate, Field: fieldDepRefs, Message: "duplicate predecessor"}
		}
		seen[key] = true
	}
	return nil
}
