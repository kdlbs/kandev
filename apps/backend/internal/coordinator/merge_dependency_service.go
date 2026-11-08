package coordinator

import (
	"context"
	"encoding/json"
	"sort"

	"github.com/kandev/kandev/internal/authz"
	"github.com/kandev/kandev/internal/coordinator/mergestate"
)

const auditDependenciesChanged = "dependencies_changed"

// graphFor builds the dependency graph of one coordinator: its sets, the live
// tasks' linked changes among every task the sets, the cards and the proposed
// references name, and which change keys a live task links.
func (s *Service) graphFor(ctx context.Context, workspaceID string, sets map[string]DepSet, cards []string, proposed []DepRef) (depGraph, error) {
	links := s.changeLinkReader()
	g := depGraph{deps: map[string][]DepRef{}, changes: map[string][]string{}, linked: map[string]bool{}}
	if links == nil {
		return g, nil
	}
	tasks := map[string]struct{}{}
	for _, t := range cards {
		tasks[t] = struct{}{}
	}
	changeRefs := map[string]struct{}{}
	note := func(refs []DepRef) {
		for _, r := range refs {
			if r.Task != "" {
				tasks[r.Task] = struct{}{}
			} else {
				changeRefs[r.Change] = struct{}{}
			}
		}
	}
	for card, set := range sets {
		g.deps[card] = set.Refs
		tasks[card] = struct{}{}
		note(set.Refs)
	}
	note(proposed)
	ids := make([]string, 0, len(tasks))
	for t := range tasks {
		ids = append(ids, t)
	}
	sort.Strings(ids)
	live, err := s.store.TaskWorkflows(ctx, workspaceID, ids)
	if err != nil {
		return g, err
	}
	for _, id := range ids {
		if _, ok := live[id]; !ok {
			continue
		}
		keys, err := links.ChangesLinkedToTask(ctx, workspaceID, id)
		if err != nil {
			return g, err
		}
		g.changes[id] = keys
		for _, k := range keys {
			g.linked[k] = true
		}
	}
	for k := range changeRefs {
		if g.linked[k] {
			continue
		}
		linkedTasks, err := links.TasksLinkedToChange(ctx, workspaceID, k)
		if err != nil {
			return g, err
		}
		g.linked[k] = len(linkedTasks) > 0
	}
	return g, nil
}

// GetDependencies returns a card's predecessor set; an absent set is empty at
// revision 0.
func (s *Service) GetDependencies(ctx context.Context, workspaceID, coordinatorID, taskID string) (DepSet, error) {
	if err := s.authz.AuthorizeWorkspaceScope(ctx, workspaceID, authz.ScopeWorkspaceRead); err != nil {
		return DepSet{}, err
	}
	if _, err := s.store.GetCoordinator(ctx, workspaceID, coordinatorID); err != nil {
		return DepSet{}, err
	}
	sets, err := s.store.loadDepSets(ctx, s.store.ro, coordinatorID)
	if err != nil {
		return DepSet{}, err
	}
	set := sets[taskID]
	if set.Refs == nil {
		set.Refs = []DepRef{}
	}
	return set, nil
}

// SaveDependencies replaces a card's complete predecessor set. expected is the
// revision the manager read (0 for a first save); it is required, and a stale
// one is a revision_conflict. A save that does not change the set stores
// nothing. Only a workspace manager may save.
func (s *Service) SaveDependencies(ctx context.Context, workspaceID, coordinatorID, taskID string, haveRevision bool, expected int, refs []DepRef) (DepSet, error) {
	if err := s.authz.AuthorizeWorkspaceScope(ctx, workspaceID, authz.ScopeWorkspaceManage); err != nil {
		return DepSet{}, err
	}
	if !s.merge {
		return DepSet{}, &SettingsError{Code: codeMergeUnavailable, Field: "merge", Message: "merge settings are not available"}
	}
	if _, err := s.store.GetCoordinator(ctx, workspaceID, coordinatorID); err != nil {
		return DepSet{}, err
	}
	if refs == nil {
		refs = []DepRef{}
	}
	var out DepSet
	err := s.store.withCoordinatorLock(ctx, coordinatorID, func(tx coordinatorExec) error {
		sets, err := s.store.loadDepSets(ctx, tx, coordinatorID)
		if err != nil {
			return err
		}
		out, err = s.applyDependencies(ctx, tx, workspaceID, coordinatorID, taskID, sets, haveRevision, expected, refs)
		return err
	})
	return out, err
}

func (s *Service) applyDependencies(ctx context.Context, tx coordinatorExec, workspaceID, coordinatorID, taskID string, sets map[string]DepSet, haveRevision bool, expected int, refs []DepRef) (DepSet, error) {
	cur := sets[taskID]
	if !haveRevision {
		return DepSet{}, &SettingsError{Code: codeRevisionRequired, Field: "dependency_revision", Message: "dependency_revision is required"}
	}
	if expected != cur.Revision {
		return DepSet{}, &MergeConflictError{Code: codeRevisionConflict}
	}
	g, err := s.graphFor(ctx, workspaceID, sets, []string{taskID}, refs)
	if err != nil {
		return DepSet{}, err
	}
	if _, live := g.changes[taskID]; !live {
		return DepSet{}, &SettingsError{Code: codeDependencyUnknown, Field: "task_id", Message: "card is not a live task of this workspace"}
	}
	if verr := validateDepRefs(taskID, refs, g); verr != nil {
		return DepSet{}, verr
	}
	if depRefsEqual(cur.Refs, refs) {
		if cur.Refs == nil {
			cur.Refs = []DepRef{}
		}
		return cur, nil
	}
	next := DepSet{Refs: refs, Revision: cur.Revision + 1}
	if err := s.store.writeDepSet(ctx, tx, coordinatorID, taskID, next); err != nil {
		return DepSet{}, err
	}
	before, _ := json.Marshal(cur.Refs)
	after, _ := json.Marshal(refs)
	err = s.store.insertMergeAudit(ctx, tx, coordinatorID, decidingUserID(ctx), PolicyAudit{
		Kind: auditDependenciesChanged, Subject: taskID, Before: string(before), After: string(after), Revision: next.Revision,
	})
	return next, err
}

func depRefsEqual(a, b []DepRef) bool {
	if len(a) != len(b) {
		return false
	}
	norm := func(x []DepRef) []DepRef {
		c := append([]DepRef(nil), x...)
		sort.Slice(c, func(i, j int) bool {
			if c[i].Task != c[j].Task {
				return c[i].Task < c[j].Task
			}
			return c[i].Change < c[j].Change
		})
		return c
	}
	na, nb := norm(a), norm(b)
	for i := range na {
		if na[i] != nb[i] {
			return false
		}
	}
	return true
}

// predecessorState reads the card's set under the coordinator lock, so the
// returned revision names exactly the set evaluated, then reads each
// predecessor change uncached. Met means every predecessor merged; Unmet that
// the set is blocked or a predecessor is not merged; Unavailable that a read
// failed.
func (s *Service) predecessorState(ctx context.Context, workspaceID, coordinatorID, taskID string) (mergestate.Tri, int, error) {
	var (
		set DepSet
		g   depGraph
	)
	err := s.store.withCoordinatorLock(ctx, coordinatorID, func(tx coordinatorExec) error {
		sets, err := s.store.loadDepSets(ctx, tx, coordinatorID)
		if err != nil {
			return err
		}
		set = sets[taskID]
		g, err = s.graphFor(ctx, workspaceID, sets, []string{taskID}, nil)
		return err
	})
	if err != nil {
		return mergestate.Unavailable, 0, err
	}
	keys, blocked := g.Predecessors(taskID)
	if blocked {
		return mergestate.Unmet, set.Revision, nil
	}
	for _, k := range keys {
		res, err := s.ObserveChange(ctx, workspaceID, k, true)
		if err != nil {
			return mergestate.Unavailable, set.Revision, nil
		}
		if res.Snapshot.State != mergestate.StateMerged {
			return mergestate.Unmet, set.Revision, nil
		}
	}
	return mergestate.Met, set.Revision, nil
}
