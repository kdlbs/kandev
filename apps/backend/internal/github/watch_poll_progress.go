package github

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
)

type watchPollKey struct{ kind, workspace, id string }

type watchFilterCursor struct {
	mu          sync.Mutex
	fingerprint string
	next        int
}

// beginWatchFilters retains only a cursor, never provider results or credentials.
// A concurrent check of the same sweep yields instead of holding a poller slot.
func (s *Service) beginWatchFilters(ctx context.Context, key watchPollKey, scope, query string, repos []RepoFilter) *watchFilterCursor {
	if githubWorkClass(ctx) != WorkClassBackground || !nonBlockingGitHubAdmission(ctx) {
		cursor := &watchFilterCursor{}
		cursor.mu.Lock()
		return cursor
	}
	fingerprint, _ := json.Marshal([]any{scope, query, repos})
	s.watchProgressMu.Lock()
	defer s.watchProgressMu.Unlock()
	if s.watchFilterProgress == nil {
		s.watchFilterProgress = make(map[watchPollKey]*watchFilterCursor)
	}
	cursor := s.watchFilterProgress[key]
	if cursor == nil || cursor.fingerprint != string(fingerprint) {
		cursor = &watchFilterCursor{fingerprint: string(fingerprint)}
		s.watchFilterProgress[key] = cursor
	}
	if !cursor.mu.TryLock() {
		return nil
	}
	return cursor
}

func watchReadDeferred(err error) bool {
	kind := FailureKindOf(err)
	return errors.Is(err, ErrBackgroundAdmissionDeferred) || kind == FailurePrimaryRateLimit || kind == FailureSecondaryRateLimit
}

func (s *Service) pruneWatchFilterProgress(kind string, live map[string]bool) {
	s.watchProgressMu.Lock()
	defer s.watchProgressMu.Unlock()
	for key := range s.watchFilterProgress {
		if key.kind == kind && !live[key.id] {
			delete(s.watchFilterProgress, key)
		}
	}
}

func (s *Service) clearWatchFilterProgress(workspace string) {
	s.watchProgressMu.Lock()
	defer s.watchProgressMu.Unlock()
	for key := range s.watchFilterProgress {
		if workspace == "" || key.workspace == workspace {
			delete(s.watchFilterProgress, key)
		}
	}
}
