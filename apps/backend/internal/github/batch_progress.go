package github

import (
	"errors"
	"time"
)

const (
	batchedPRProgressTTL        = 24 * time.Hour
	batchedPRProgressMaxEntries = 256
)

type batchedPRProgressEntry struct {
	progress *batchedPRQueryProgress
	updated  time.Time
	active   int
}

type batchedBranchProgressEntry struct {
	progress *batchedBranchQueryProgress
	updated  time.Time
	active   int
}

func (s *Service) batchedPRQueryProgress(key string) *batchedPRQueryProgress {
	s.batchedProgressMu.Lock()
	defer s.batchedProgressMu.Unlock()
	now := time.Now()
	s.expireBatchedProgressLocked(now)
	if s.batchedPRProgress == nil {
		s.batchedPRProgress = make(map[string]*batchedPRProgressEntry)
	}
	entry := s.batchedPRProgress[key]
	if entry == nil {
		s.evictOldestBatchedPRProgressLocked()
		entry = &batchedPRProgressEntry{progress: &batchedPRQueryProgress{}, updated: now}
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
	s.batchedProgressMu.Unlock()
}

func (s *Service) batchedBranchQueryProgress(key string) *batchedBranchQueryProgress {
	s.batchedProgressMu.Lock()
	defer s.batchedProgressMu.Unlock()
	now := time.Now()
	s.expireBatchedProgressLocked(now)
	if s.batchedBranchProgress == nil {
		s.batchedBranchProgress = make(map[string]*batchedBranchProgressEntry)
	}
	entry := s.batchedBranchProgress[key]
	if entry == nil {
		s.evictOldestBatchedBranchProgressLocked()
		entry = &batchedBranchProgressEntry{progress: &batchedBranchQueryProgress{}, updated: now}
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
