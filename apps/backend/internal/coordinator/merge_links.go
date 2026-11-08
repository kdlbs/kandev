package coordinator

import (
	"context"
	"fmt"

	"github.com/kandev/kandev/internal/coordinator/mergestate"
)

// ChangeLinks resolves which tasks a change is linked to. It is satisfied by an
// adapter over the provider integrations' task link tables.
type ChangeLinks interface {
	// TasksLinkedToChange returns the IDs of the workspace's tasks linked to
	// the change key (provider|host|repository ID|number).
	TasksLinkedToChange(ctx context.Context, workspaceID, changeKey string) ([]string, error)
	// OpenChanges returns the change keys of the workspace's open changes that
	// are linked to a live task, sorted ascending.
	OpenChanges(ctx context.Context, workspaceID string) ([]string, error)
}

// SetChangeLinks wires the change-to-task link reader; nil until then, and the
// merge wake duties do nothing without it.
func (s *Service) SetChangeLinks(l ChangeLinks) {
	s.wakeMu.Lock()
	s.changeLinks = l
	s.wakeMu.Unlock()
}

// SetChangeObserver wires the provider-backed change observer; nil until then.
func (s *Service) SetChangeObserver(o *ChangeObserver) {
	s.wakeMu.Lock()
	s.changeObserver = o
	s.wakeMu.Unlock()
}

func (s *Service) observer() *ChangeObserver {
	s.wakeMu.Lock()
	defer s.wakeMu.Unlock()
	return s.changeObserver
}

// ObserveChange reads the change (reusing a successful read younger than 60
// seconds unless force), applies the observation to its condition rows and
// records the wake of each new episode for every autonomy-on coordinator whose
// Watches cover a linked task. A failed read changes nothing and returns the
// read result with the last successful values.
func (s *Service) ObserveChange(ctx context.Context, workspaceID string, key mergestate.ChangeKey, force bool) (ReadResult, error) {
	obs, links := s.observer(), s.changeLinkReader()
	if !s.merge || obs == nil {
		return ReadResult{}, nil
	}
	res, err := obs.Read(ctx, workspaceID, key, force)
	if err != nil {
		return res, err
	}
	var targets []WakeTarget
	if links != nil {
		if targets, err = s.wakeTargets(ctx, links, workspaceID, key.String()); err != nil {
			return res, fmt.Errorf("resolve change watchers: %w", err)
		}
	}
	in := ChangeObservationInput{WorkspaceID: workspaceID, ChangeKey: key.String(), Obs: res.Snapshot.Observation(res.StartedAt), Targets: targets}
	if _, err := s.store.ApplyChangeObservation(ctx, in); err != nil {
		return res, err
	}
	return res, nil
}

// maxObservedPerPass bounds the provider reads one backstop pass starts for a
// coordinator; the rest wait for the next pass.
const maxObservedPerPass = 100

// ObserveOpenChanges reads every open change of the workspace (up to
// maxObservedPerPass) through ObserveChange. A failed read of one change does
// not stop the others; the first error is returned.
func (s *Service) ObserveOpenChanges(ctx context.Context, workspaceID string) error {
	links := s.changeLinkReader()
	if !s.merge || links == nil || s.observer() == nil {
		return nil
	}
	keys, err := links.OpenChanges(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("list open changes: %w", err)
	}
	if len(keys) > maxObservedPerPass {
		keys = keys[:maxObservedPerPass]
	}
	var first error
	for _, k := range keys {
		key, ok := mergestate.ParseChangeKey(k)
		if !ok || ctx.Err() != nil {
			continue
		}
		if _, err := s.ObserveChange(ctx, workspaceID, key, false); err != nil && first == nil {
			first = err
		}
	}
	return first
}

// wakeTargets returns the (coordinator, task) pairs a wake for the change is
// recorded for: autonomy-on coordinators of the workspace and the linked tasks
// in their Watches.
func (s *Service) wakeTargets(ctx context.Context, links ChangeLinks, workspaceID, changeKey string) ([]WakeTarget, error) {
	tasks, err := links.TasksLinkedToChange(ctx, workspaceID, changeKey)
	if err != nil || len(tasks) == 0 {
		return nil, err
	}
	coords, err := s.store.ListCoordinators(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	var targets []WakeTarget
	for _, c := range coords {
		if !c.AutonomyEnabled {
			continue
		}
		watched, err := s.watchedTasksFor(ctx, c, tasks)
		if err != nil {
			return nil, err
		}
		for _, id := range tasks {
			if _, ok := watched[id]; ok {
				targets = append(targets, WakeTarget{CoordinatorID: c.ID, TaskID: id})
			}
		}
	}
	return targets, nil
}

func (s *Service) changeLinkReader() ChangeLinks {
	s.wakeMu.Lock()
	defer s.wakeMu.Unlock()
	return s.changeLinks
}
