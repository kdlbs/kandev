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
	if links == nil || !s.merge {
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
	watched, err := b.svc.watchedTasksFor(ctx, c, ids)
	if !b.mergeReadOK(ctx, "merge watched tasks", err) {
		return nil, false
	}
	return watched, true
}

// watchedTasksFor returns the live tasks among ids that are in the
// coordinator's Watches. A failed read is the error and the caller fails
// closed.
func (s *Service) watchedTasksFor(ctx context.Context, c *Coordinator, ids []string) (map[string]struct{}, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	set, err := s.store.EffectiveWatchSet(ctx, s.store.ro, c.ID, c.WorkspaceID)
	if err != nil {
		return nil, err
	}
	g, err := s.newWatchGate(ctx, set, c.WorkspaceID)
	if err != nil {
		return nil, err
	}
	workflows, err := s.store.TaskWorkflows(ctx, c.WorkspaceID, ids)
	if err != nil {
		return nil, err
	}
	repos, err := s.taskRepositoryIDs(ctx, g, ids)
	if err != nil {
		return nil, err
	}
	watched := map[string]struct{}{}
	for id, workflow := range workflows {
		if g.task(workflow, repos[id]) {
			watched[id] = struct{}{}
		}
	}
	return watched, nil
}
