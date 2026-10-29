package toolretention

import "context"

const (
	historyCompactionBatch = 200
	// historyArchiveFailureLimit bounds durable owned-ref cleanup retries so a
	// permanently failing intent cannot keep the shared operation running.
	historyArchiveFailureLimit = 5
)

// stepHistoryAnalysis adds the bounded history inventory once per analysis.
func (s *Service) stepHistoryAnalysis(ctx context.Context, r *record) error {
	if r == nil || r.Operation == nil || r.Progress.HistoryDone {
		return nil
	}
	if s.opts.History == nil {
		r.Progress.HistoryDone = true
		return nil
	}
	stats, err := s.opts.History.Analyze(ctx, r.Operation.Cutoff)
	if err != nil {
		return err
	}
	applyHistoryStats(r.Operation, stats, true)
	if !stats.Complete {
		r.Operation.HistoryTruncated = true
	}
	r.Progress.HistoryDone = true
	return nil
}

// stepHistoryCleanup advances one bounded history batch under the shared
// maintenance lease. It reports Done only when no candidate remains.
func (s *Service) stepHistoryCleanup(ctx context.Context, r *record) (bool, error) {
	if r == nil || r.Operation == nil || r.Progress.HistoryDone {
		return false, nil
	}
	if s.opts.History == nil {
		r.Progress.HistoryDone = true
		return true, nil
	}
	stats, err := s.opts.History.CompactOnce(ctx, r.Operation.Cutoff, historyCompactionBatch)
	applyHistoryStats(r.Operation, stats, false)
	if err != nil {
		if s.opts.Log != nil {
			s.opts.Log(ctx, "history compaction failed", err)
		}
		return true, err
	}
	if stats.ArchiveFailures > 0 {
		r.Progress.HistoryArchiveFailures++
		if r.Progress.HistoryArchiveFailures >= historyArchiveFailureLimit {
			r.Progress.HistoryDone = true
		}
	}
	if stats.Remaining == 0 && stats.ArchiveFailures == 0 {
		r.Progress.HistoryDone = true
	}
	if s.opts.Log != nil && stats.ArchiveFailures > 0 {
		s.opts.Log(ctx, "history compaction retained owned refs for retry", nil)
	}
	return true, nil
}

func applyHistoryStats(operation *Operation, stats HistoryStats, analysis bool) {
	if operation == nil {
		return
	}
	if analysis {
		operation.HistoryTasks = stats.Tasks
		operation.HistorySnapshots = stats.Snapshots
		operation.HistorySQLBytes = stats.LegacySQLBytes
		operation.HistoryGitBytes = stats.ArchiveBytes
		return
	}
	operation.ExpiredSets += stats.ExpiredChangeSets
	operation.DeletedArchiveRefs += stats.DeletedArchiveRefs
	operation.ReclaimedBytes += stats.ReclaimedBytes
	operation.ArchiveFailures += stats.ArchiveFailures
}
