package logbundle

import (
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
)

type backendCandidate struct {
	Name     string
	Path     string
	Day      string
	Sequence int
	Active   bool
	Legacy   bool
}

func (s *Service) backendCandidates() []backendCandidate {
	logDir := filepath.Join(s.config.HomeDir, "logs")
	entries, err := os.ReadDir(logDir)
	if err != nil {
		return nil
	}
	today := s.config.Now().UTC()
	todayDay := today.Format(time.DateOnly)
	candidates := make([]backendCandidate, 0, len(entries))
	for _, entry := range entries {
		parsed, ok := logger.ParseBackendLogFilename(entry.Name())
		if !ok {
			continue
		}
		path := filepath.Join(logDir, entry.Name())
		info, err := os.Lstat(path)
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		day := parsed.Day
		if parsed.Active {
			day = todayDay
		}
		if !logger.BackendLogDayRetained(day, today) {
			continue
		}
		candidates = append(candidates, backendCandidate{
			Name: entry.Name(), Path: path, Day: day, Sequence: parsed.Sequence,
			Active: parsed.Active, Legacy: parsed.Legacy,
		})
	}
	slices.SortFunc(candidates, compareBackendCandidates)
	return candidates
}

func compareBackendCandidates(left, right backendCandidate) int {
	if left.Day != right.Day {
		if left.Day > right.Day {
			return -1
		}
		return 1
	}
	leftRank, rightRank := backendCandidateRank(left), backendCandidateRank(right)
	if leftRank != rightRank {
		if leftRank > rightRank {
			return -1
		}
		return 1
	}
	if left.Name < right.Name {
		return -1
	}
	if left.Name > right.Name {
		return 1
	}
	return 0
}

func backendCandidateRank(candidate backendCandidate) int {
	if candidate.Active {
		return int(^uint(0) >> 1)
	}
	if candidate.Legacy {
		return -1
	}
	return candidate.Sequence
}
