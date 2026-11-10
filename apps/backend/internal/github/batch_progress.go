package github

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	batchedPRProgressTTL        = 24 * time.Hour
	batchedPRProgressMaxEntries = 256
)

type batchedPRProgressEntry struct {
	progress *batchedPRQueryProgress
	refsKey  string
	updated  time.Time
	active   int
}

type batchedBranchProgressEntry struct {
	progress *batchedBranchQueryProgress
	refsKey  string
	updated  time.Time
	active   int
}

type workflowPhaseProgressEntry struct {
	progress *workflowPhaseProgress
	refsKey  string
	updated  time.Time
	active   int
}

func batchedPRProgressRefsKey(refs []graphQLPRRef) string {
	parts := make([]string, 0, len(refs))
	for _, ref := range refs {
		parts = append(parts, fmt.Sprintf("%s/%s#%d", strings.ToLower(ref.Owner), strings.ToLower(ref.Repo), ref.Number))
	}
	return strings.Join(parts, "|")
}

func batchedBranchProgressRefsKey(refs []graphQLBranchRef) string {
	parts := make([]string, 0, len(refs))
	for _, ref := range refs {
		parts = append(parts, fmt.Sprintf("%s/%s@%s", strings.ToLower(ref.Owner), strings.ToLower(ref.Repo), ref.Branch))
	}
	return strings.Join(parts, "|")
}

func (s *Service) batchedPRQueryProgress(key, refsKey string) *batchedPRQueryProgress {
	s.batchedProgressMu.Lock()
	defer s.batchedProgressMu.Unlock()
	now := time.Now()
	s.expireBatchedProgressLocked(now)
	if s.batchedPRProgress == nil {
		s.batchedPRProgress = make(map[string]*batchedPRProgressEntry)
	}
	entry := s.batchedPRProgress[key]
	if entry != nil && entry.refsKey != refsKey {
		delete(s.batchedPRProgress, key)
		entry = nil
	}
	if entry == nil {
		s.evictOldestBatchedPRProgressLocked()
		entry = &batchedPRProgressEntry{progress: &batchedPRQueryProgress{}, refsKey: refsKey, updated: now}
		s.batchedPRProgress[key] = entry
	}
	entry.active++
	entry.updated = now
	return entry.progress
}

func (s *Service) finishBatchedPRQueryProgress(key string, progress *batchedPRQueryProgress, keep bool) {
	s.batchedProgressMu.Lock()
	defer s.batchedProgressMu.Unlock()
	entry := s.batchedPRProgress[key]
	if entry == nil || entry.progress != progress {
		return
	}
	if entry.active > 0 {
		entry.active--
	}
	if !keep {
		delete(s.batchedPRProgress, key)
		return
	}
	entry.updated = time.Now()
}

func (s *Service) evictOldestBatchedPRProgressLocked() {
	for len(s.batchedPRProgress) >= batchedPRProgressMaxEntries {
		oldestKey := ""
		var oldest time.Time
		for key, entry := range s.batchedPRProgress {
			if entry.active != 0 {
				continue
			}
			if oldestKey == "" || entry.updated.Before(oldest) {
				oldestKey = key
				oldest = entry.updated
			}
		}
		if oldestKey == "" {
			return
		}
		delete(s.batchedPRProgress, oldestKey)
	}
}

func (s *Service) clearBatchedPRProgress() {
	s.batchedProgressMu.Lock()
	s.batchedPRProgress = nil
	s.batchedBranchProgress = nil
	s.batchedWorkflowProgress = nil
	s.batchedProgressMu.Unlock()
}

func (s *Service) discardBatchedPRQueryProgress(key string) {
	s.batchedProgressMu.Lock()
	delete(s.batchedPRProgress, key)
	s.batchedProgressMu.Unlock()
}

func (s *Service) batchedBranchQueryProgress(key, refsKey string) *batchedBranchQueryProgress {
	s.batchedProgressMu.Lock()
	defer s.batchedProgressMu.Unlock()
	now := time.Now()
	s.expireBatchedProgressLocked(now)
	if s.batchedBranchProgress == nil {
		s.batchedBranchProgress = make(map[string]*batchedBranchProgressEntry)
	}
	entry := s.batchedBranchProgress[key]
	if entry != nil && entry.refsKey != refsKey {
		delete(s.batchedBranchProgress, key)
		entry = nil
	}
	if entry == nil {
		s.evictOldestBatchedBranchProgressLocked()
		entry = &batchedBranchProgressEntry{progress: &batchedBranchQueryProgress{}, refsKey: refsKey, updated: now}
		s.batchedBranchProgress[key] = entry
	}
	entry.active++
	entry.updated = now
	return entry.progress
}

func (s *Service) expireBatchedProgressLocked(now time.Time) {
	for key, entry := range s.batchedPRProgress {
		if entry.active == 0 && now.Sub(entry.updated) >= batchedPRProgressTTL {
			delete(s.batchedPRProgress, key)
		}
	}
	for key, entry := range s.batchedBranchProgress {
		if entry.active == 0 && now.Sub(entry.updated) >= batchedPRProgressTTL {
			delete(s.batchedBranchProgress, key)
		}
	}
	for key, entry := range s.batchedWorkflowProgress {
		if entry.active == 0 && now.Sub(entry.updated) >= batchedPRProgressTTL {
			delete(s.batchedWorkflowProgress, key)
		}
	}
}

// workflowPhaseProgress returns the retained workflow evidence for one phase
// identity. A changed target set replaces the entry so a stale head or ref set
// can never satisfy a new pass.
func (s *Service) workflowPhaseProgress(key, refsKey string) *workflowPhaseProgress {
	s.batchedProgressMu.Lock()
	defer s.batchedProgressMu.Unlock()
	now := time.Now()
	s.expireBatchedProgressLocked(now)
	if s.batchedWorkflowProgress == nil {
		s.batchedWorkflowProgress = make(map[string]*workflowPhaseProgressEntry)
	}
	entry := s.batchedWorkflowProgress[key]
	if entry != nil && entry.refsKey != refsKey {
		delete(s.batchedWorkflowProgress, key)
		entry = nil
	}
	if entry == nil {
		s.evictOldestWorkflowProgressLocked()
		entry = &workflowPhaseProgressEntry{progress: newWorkflowPhaseProgress(), refsKey: refsKey, updated: now}
		s.batchedWorkflowProgress[key] = entry
	}
	entry.active++
	entry.updated = now
	return entry.progress
}

func (s *Service) finishWorkflowPhaseProgress(key string, progress *workflowPhaseProgress, keep bool) {
	s.batchedProgressMu.Lock()
	defer s.batchedProgressMu.Unlock()
	entry := s.batchedWorkflowProgress[key]
	if entry == nil || entry.progress != progress {
		return
	}
	if entry.active > 0 {
		entry.active--
	}
	if !keep {
		delete(s.batchedWorkflowProgress, key)
		return
	}
	entry.updated = time.Now()
}

func (s *Service) evictOldestWorkflowProgressLocked() {
	for len(s.batchedWorkflowProgress) >= batchedPRProgressMaxEntries {
		oldestKey := ""
		var oldest time.Time
		for key, entry := range s.batchedWorkflowProgress {
			if entry.active != 0 {
				continue
			}
			if oldestKey == "" || entry.updated.Before(oldest) {
				oldestKey = key
				oldest = entry.updated
			}
		}
		if oldestKey == "" {
			return
		}
		delete(s.batchedWorkflowProgress, oldestKey)
	}
}

func (s *Service) finishBatchedBranchQueryProgress(key string, progress *batchedBranchQueryProgress, keep bool) {
	s.batchedProgressMu.Lock()
	defer s.batchedProgressMu.Unlock()
	entry := s.batchedBranchProgress[key]
	if entry == nil || entry.progress != progress {
		return
	}
	if entry.active > 0 {
		entry.active--
	}
	if !keep {
		delete(s.batchedBranchProgress, key)
		return
	}
	entry.updated = time.Now()
}

func (s *Service) discardBatchedBranchQueryProgress(key string) {
	s.batchedProgressMu.Lock()
	delete(s.batchedBranchProgress, key)
	s.batchedProgressMu.Unlock()
}

func (s *Service) evictOldestBatchedBranchProgressLocked() {
	for len(s.batchedBranchProgress) >= batchedPRProgressMaxEntries {
		oldestKey := ""
		var oldest time.Time
		for key, entry := range s.batchedBranchProgress {
			if entry.active != 0 {
				continue
			}
			if oldestKey == "" || entry.updated.Before(oldest) {
				oldestKey = key
				oldest = entry.updated
			}
		}
		if oldestKey == "" {
			return
		}
		delete(s.batchedBranchProgress, oldestKey)
	}
}

func keepBatchedPRProgress(err error) bool {
	var deferred *AdmissionDeferredError
	return errors.As(err, &deferred)
}
