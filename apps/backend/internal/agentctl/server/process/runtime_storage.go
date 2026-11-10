package process

import (
	"context"
	"path/filepath"
	"time"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	systemmetrics "github.com/kandev/kandev/internal/system/metrics"
)

// diskCapacitySampler is shared by every instance in one agentctl process so
// concurrent tasks on the same proven filesystem reuse one metadata read.
func (m *Manager) SetDiskCapacitySampler(sampler *systemmetrics.DiskCapacitySampler) {
	if m == nil {
		return
	}
	m.diskSamplerMu.Lock()
	m.diskSampler = sampler
	m.diskSamplerMu.Unlock()
}

// RuntimeStorage reports the advisory capacity of the filesystem this
// executor actually uses. A configured journal wins because that is where
// delivery writes land; otherwise the workspace filesystem is measured.
// It also caches the result for ordinary status responses.
func (m *Manager) RuntimeStorage(ctx context.Context) streams.RuntimeStorageSnapshot {
	now := time.Now().UTC()
	path, kind, ok := m.runtimeStorageTarget()
	if !ok {
		return m.cacheRuntimeStorage(streams.RuntimeStorageSnapshot{
			Version:     streams.RuntimeStorageSnapshotVersion,
			StorageKind: streams.RuntimeStorageKindWorkspace,
			ObservedAt:  now,
			Unavailable: true,
		})
	}
	sample, err := m.sampleRuntimeStorage(ctx, path)
	if err != nil {
		return m.cacheRuntimeStorage(streams.RuntimeStorageSnapshot{
			Version:     streams.RuntimeStorageSnapshotVersion,
			StorageKind: kind,
			ObservedAt:  now,
			Unavailable: true,
		})
	}
	return m.cacheRuntimeStorage(streams.RuntimeStorageSnapshot{
		Version:     streams.RuntimeStorageSnapshotVersion,
		StorageKind: kind,
		ObservedAt:  sample.ObservedAt,
		Filesystem: &streams.RuntimeStorageFilesystem{
			TotalBytes:     int64(sample.Capacity.TotalBytes),
			AvailableBytes: int64(sample.Capacity.AvailableBytes),
			ObservedAt:     sample.ObservedAt,
			FilesystemID:   sample.FilesystemID,
		},
	})
}

func (m *Manager) cacheRuntimeStorage(
	snapshot streams.RuntimeStorageSnapshot,
) streams.RuntimeStorageSnapshot {
	if m == nil {
		return snapshot
	}
	copied := snapshot
	if snapshot.Filesystem != nil {
		filesystem := *snapshot.Filesystem
		copied.Filesystem = &filesystem
	}
	m.runtimeStorageCacheMu.Lock()
	m.runtimeStorageCache = &copied
	m.runtimeStorageCacheMu.Unlock()
	return snapshot
}

// CachedRuntimeStorage returns the last advisory snapshot without touching the
// filesystem. Ordinary status uses this so a slow advisor cannot consume its
// recovery budget.
func (m *Manager) CachedRuntimeStorage() *streams.RuntimeStorageSnapshot {
	if m == nil {
		return nil
	}
	m.runtimeStorageCacheMu.RLock()
	defer m.runtimeStorageCacheMu.RUnlock()
	if m.runtimeStorageCache == nil {
		return nil
	}
	copied := *m.runtimeStorageCache
	if m.runtimeStorageCache.Filesystem != nil {
		filesystem := *m.runtimeStorageCache.Filesystem
		copied.Filesystem = &filesystem
	}
	return &copied
}

func (m *Manager) runtimeStorageTarget() (string, string, bool) {
	if m == nil || m.cfg == nil {
		return "", "", false
	}
	if m.cfg.DurableJournalPath != "" {
		return filepath.Dir(m.cfg.DurableJournalPath), streams.RuntimeStorageKindJournal, true
	}
	if m.cfg.WorkDir != "" {
		return m.cfg.WorkDir, streams.RuntimeStorageKindWorkspace, true
	}
	return "", "", false
}

func (m *Manager) sampleRuntimeStorage(ctx context.Context, path string) (systemmetrics.DiskCapacitySample, error) {
	m.diskSamplerMu.RLock()
	sampler := m.diskSampler
	m.diskSamplerMu.RUnlock()
	if sampler != nil {
		return sampler.Sample(ctx, path)
	}
	capacity, err := systemmetrics.DiskUsage(ctx, path)
	if err != nil {
		return systemmetrics.DiskCapacitySample{}, err
	}
	identity, _ := systemmetrics.FilesystemIdentity(path)
	return systemmetrics.DiskCapacitySample{
		Capacity:     capacity,
		FilesystemID: identity,
		ObservedAt:   time.Now().UTC(),
	}, nil
}
