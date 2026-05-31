//go:build linux

package oslimits

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
)

const maxTopConsumers = 5

// InotifyProbe reads inotify resource usage from /proc on Linux/WSL.
type InotifyProbe struct {
	procRoot string
}

// NewInotifyProbe creates an InotifyProbe that reads from /proc.
func NewInotifyProbe() *InotifyProbe {
	return &InotifyProbe{procRoot: "/proc"}
}

// Name returns the probe name.
func (p *InotifyProbe) Name() string { return "Inotify" }

// Category returns the probe category.
func (p *InotifyProbe) Category() string { return categoryID }

// Samples returns two samples: one for inotify instances and one for watches.
func (p *InotifyProbe) Samples(_ context.Context) ([]Sample, error) {
	maxInstances, err := p.readUintFile(filepath.Join(p.procRoot, "sys/fs/inotify/max_user_instances"))
	if err != nil {
		return nil, fmt.Errorf("read max_user_instances: %w", err)
	}
	maxWatches, err := p.readUintFile(filepath.Join(p.procRoot, "sys/fs/inotify/max_user_watches"))
	if err != nil {
		return nil, fmt.Errorf("read max_user_watches: %w", err)
	}

	uid := uint32(os.Getuid())
	stats, err := p.scanProc(uid)
	if err != nil {
		// Best-effort: return stats with zero usage rather than failing
		stats = procStats{}
	}

	instSample := buildInstanceSample(stats, maxInstances)
	watchSample := buildWatchSample(stats, maxWatches)

	return []Sample{instSample, watchSample}, nil
}

func buildInstanceSample(stats procStats, maxInstances uint64) Sample {
	s := Sample{
		ID:           sampleIDInotifyInstances,
		Name:         "Inotify instances",
		Unit:         unitInstances,
		Used:         stats.totalInstances,
		Limit:        maxInstances,
		Supported:    true,
		TopConsumers: stats.topConsumers,
	}
	if maxInstances > 0 {
		s.UsageRatio = float64(stats.totalInstances) / float64(maxInstances)
	}
	return s
}

func buildWatchSample(stats procStats, maxWatches uint64) Sample {
	s := Sample{
		ID:           sampleIDInotifyWatches,
		Name:         "Inotify watches",
		Unit:         unitWatches,
		Used:         stats.totalWatches,
		Limit:        maxWatches,
		Supported:    true,
		TopConsumers: stats.topWatchConsumers,
	}
	if maxWatches > 0 {
		s.UsageRatio = float64(stats.totalWatches) / float64(maxWatches)
	}
	return s
}

type procStats struct {
	totalInstances    uint64
	totalWatches      uint64
	topConsumers      []Consumer // sorted by fdCount descending
	topWatchConsumers []Consumer // sorted by watchCount descending
}

type pidStats struct {
	pid        int
	command    string
	fdCount    uint64
	watchCount uint64
}

func (p *InotifyProbe) scanProc(uid uint32) (procStats, error) {
	entries, err := os.ReadDir(p.procRoot)
	if err != nil {
		return procStats{}, fmt.Errorf("read proc: %w", err)
	}

	var perPID []pidStats
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		pid, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		if !p.ownedByUID(filepath.Join(p.procRoot, e.Name()), uid) {
			continue
		}
		stats := p.scanPID(pid)
		if stats.fdCount == 0 {
			continue
		}
		perPID = append(perPID, stats)
	}

	return buildProcStats(perPID), nil
}

func buildProcStats(perPID []pidStats) procStats {
	var total procStats
	for _, s := range perPID {
		total.totalInstances += s.fdCount
		total.totalWatches += s.watchCount
	}
	total.topConsumers = topN(perPID, func(a, b pidStats) bool { return a.fdCount > b.fdCount })
	total.topWatchConsumers = topN(perPID, func(a, b pidStats) bool { return a.watchCount > b.watchCount })
	return total
}

func topN(perPID []pidStats, less func(a, b pidStats) bool) []Consumer {
	sorted := make([]pidStats, len(perPID))
	copy(sorted, perPID)
	sort.Slice(sorted, func(i, j int) bool { return less(sorted[i], sorted[j]) })
	n := maxTopConsumers
	if len(sorted) < n {
		n = len(sorted)
	}
	consumers := make([]Consumer, n)
	for i := range n {
		consumers[i] = Consumer{
			PID:        sorted[i].pid,
			Command:    sorted[i].command,
			FDCount:    sorted[i].fdCount,
			WatchCount: sorted[i].watchCount,
		}
	}
	return consumers
}

func (p *InotifyProbe) ownedByUID(pidDir string, uid uint32) bool {
	info, err := os.Stat(pidDir)
	if err != nil {
		return false
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return false
	}
	return stat.Uid == uid
}

func (p *InotifyProbe) scanPID(pid int) pidStats {
	pidDir := filepath.Join(p.procRoot, strconv.Itoa(pid))
	fdDir := filepath.Join(pidDir, "fd")
	fds, err := os.ReadDir(fdDir)
	if err != nil {
		return pidStats{pid: pid}
	}

	var fdCount, watchCount uint64
	for _, fd := range fds {
		target, err := os.Readlink(filepath.Join(fdDir, fd.Name()))
		if err != nil {
			continue
		}
		if target != "anon_inode:inotify" {
			continue
		}
		fdCount++
		watchCount += p.countWatches(pidDir, fd.Name())
	}

	if fdCount == 0 {
		return pidStats{pid: pid}
	}

	return pidStats{
		pid:        pid,
		command:    p.readComm(pidDir),
		fdCount:    fdCount,
		watchCount: watchCount,
	}
}

// countWatches parses /proc/<pid>/fdinfo/<fd> and counts "inotify " lines.
func (p *InotifyProbe) countWatches(pidDir, fdName string) uint64 {
	f, err := os.Open(filepath.Join(pidDir, "fdinfo", fdName))
	if err != nil {
		return 0
	}
	defer func() { _ = f.Close() }()
	var count uint64
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		if strings.HasPrefix(scanner.Text(), "inotify ") {
			count++
		}
	}
	return count
}

func (p *InotifyProbe) readComm(pidDir string) string {
	data, err := os.ReadFile(filepath.Join(pidDir, "comm"))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func (p *InotifyProbe) readUintFile(path string) (uint64, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	v, err := strconv.ParseUint(strings.TrimSpace(string(data)), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse %q: %w", path, err)
	}
	return v, nil
}
