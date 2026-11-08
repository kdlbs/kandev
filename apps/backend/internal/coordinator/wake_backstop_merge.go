package coordinator

import (
	"context"
	"sort"

	"github.com/kandev/kandev/internal/coordinator/mergestate"
)

// recordMergeWakes derives the merge wakes that a transition could not store
// (the pending cap, or autonomy and a watch gained later) from the workspace's
// open episodes: one wake per episode and watched linked task. It reports false
// when a read failed, abandoning only the merge duty for the pass.
func (b *WakeBackstop) recordMergeWakes(ctx context.Context, c *Coordinator) bool {
	s := b.svc
	links := s.changeLinkReader()
	if links == nil {
		return true
	}
	open, err := s.store.ListOpenEpisodes(ctx, c.WorkspaceID)
	if err != nil || len(open) == 0 {
		return b.mergeReadOK(ctx, "open merge episodes", err)
	}
	linked, ok := b.linkedTasks(ctx, links, c.WorkspaceID, open)
	if !ok {
		return false
	}
	watched, ok := b.watchedLinkedTasks(ctx, c, linked)
	if !ok {
		return false
	}
	for _, ep := range open {
		for _, taskID := range linked[ep.ChangeKey] {
			if _, in := watched[taskID]; !in || ctx.Err() != nil {
				continue
			}
			_, _ = s.RecordWake(ctx, c.ID, taskID, WakeKind("change_"+string(ep.Kind)), mergestate.EpisodeKey(ep.ChangeKey, ep.Generation))
		}
	}
	return true
}

func (b *WakeBackstop) mergeReadOK(ctx context.Context, what string, err error) bool {
	if err != nil && ctx.Err() == nil {
		b.skip(what, err)
	}
	return err == nil
}

func (b *WakeBackstop) linkedTasks(ctx context.Context, links ChangeLinks, workspaceID string, open []OpenEpisode) (map[string][]string, bool) {
	out := map[string][]string{}
	for _, ep := range open {
		if _, done := out[ep.ChangeKey]; done {
			continue
		}
		ids, err := links.TasksLinkedToChange(ctx, workspaceID, ep.ChangeKey)
		if !b.mergeReadOK(ctx, "tasks linked to change", err) {
			return nil, false
		}
		sort.Strings(ids)
		out[ep.ChangeKey] = ids
	}
	return out, true
}

// watchedLinkedTasks returns the linked tasks that are live and in the
// coordinator's Watches.
func (b *WakeBackstop) watchedLinkedTasks(ctx context.Context, c *Coordinator, linked map[string][]string) (map[string]struct{}, bool) {
	s := b.svc
	seen := map[string]struct{}{}
	var ids []string
	for _, tasks := range linked {
		for _, id := range tasks {
			if _, dup := seen[id]; !dup {
				seen[id] = struct{}{}
				ids = append(ids, id)
			}
		}
	}
	if len(ids) == 0 {
		return nil, true
	}
	set, err := s.store.EffectiveWatchSet(ctx, s.store.ro, c.ID, c.WorkspaceID)
	if !b.mergeReadOK(ctx, "merge watch set", err) {
		return nil, false
	}
	g, err := s.newWatchGate(ctx, set, c.WorkspaceID)
	if !b.mergeReadOK(ctx, "merge watch gate", err) {
		return nil, false
	}
	workflows, err := s.store.TaskWorkflows(ctx, c.WorkspaceID, ids)
	if !b.mergeReadOK(ctx, "merge task workflows", err) {
		return nil, false
	}
	repos, err := s.taskRepositoryIDs(ctx, g, ids)
	if !b.mergeReadOK(ctx, "merge task repositories", err) {
		return nil, false
	}
	watched := map[string]struct{}{}
	for id, workflow := range workflows {
		if g.task(workflow, repos[id]) {
			watched[id] = struct{}{}
		}
	}
	return watched, true
}
