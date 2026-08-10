//go:build linux

package process

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// linuxClockTicksPerSecond is CLK_TCK / USER_HZ, the unit /proc/<pid>/stat's
// starttime field is expressed in. The Linux kernel ABI has fixed this at
// 100 since the 2.6 timer-frequency decoupling specifically so userspace
// tools do not need sysconf(_SC_CLK_TCK) — ps(1), top(1) and procps all
// hardcode the same value.
const linuxClockTicksPerSecond = 100

// linuxProcStatResolution is the tick resolution start times are compared at
// (D5/AC-80's "source resolution").
var linuxProcStatResolution = time.Second / linuxClockTicksPerSecond

// procEntry is one /proc/<pid>/stat snapshot, holding the raw kernel
// ticks-since-boot start time. Comparisons stay entirely in this tick
// domain (see turnStartMarker) rather than converting through a wall-clock
// boot-time anchor, so no per-walk anchor read is needed at all.
type procEntry struct {
	ppid       int
	startTicks int64
	zombie     bool
}

// linuxCollectProcs snapshots every currently-visible /proc/<pid> entry into
// a (ppid, ticks-since-boot, zombie) map. Returns ok=false on a hard read
// failure or context cancellation; entries this process can no longer read
// (raced exit, permission) are skipped rather than failing the whole
// snapshot.
func linuxCollectProcs(ctx context.Context) (map[int]procEntry, bool) {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil, false
	}

	procs := make(map[int]procEntry, len(entries))
	for _, e := range entries {
		if ctx.Err() != nil {
			return nil, false
		}
		if !e.IsDir() {
			continue
		}
		pid, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		stat, err := linuxReadStat(pid)
		if err != nil {
			continue
		}
		procs[pid] = procEntry{
			ppid:       stat.ppid,
			startTicks: stat.startTicks,
			zombie:     stat.state == "Z",
		}
	}
	return procs, true
}

// newTurnStartMarker converts a wall-clock turn-start stamp into
// boot-relative ticks ONCE, using the boot anchor read at this same moment
// (D5, AC-80's clock-domain rule) — RecordTurnStart calls this immediately
// at stamp time, so no later probe ever re-reads a boot anchor or re-derives
// the comparison. Integer division truncates toward zero, which for the
// always-positive stamp-since-boot duration here is truncation DOWN to tick
// resolution — the same "error always falls toward live" direction AC-80
// requires. If the anchor can't be read, hasBootTicks stays false and the
// probe answers unknown (never settled) per the spec's failure rule.
func newTurnStartMarker(t time.Time) turnStartMarker {
	bootTime, ok := linuxBootTime()
	if !ok {
		return turnStartMarker{wallTime: t}
	}
	ticks := int64(t.Sub(bootTime) / linuxProcStatResolution)
	return turnStartMarker{wallTime: t, bootTicks: ticks, hasBootTicks: true}
}

// captureRootIdentity reads pid's current /proc/<pid>/stat startTicks, to be
// compared later by walkProcessTree against a fresh read of whatever process
// currently holds that PID (D5). Ticks-since-boot is a raw kernel value
// fixed for the lifetime of the process, so re-reading it later for a
// still-alive process returns the identical value with no jitter.
func captureRootIdentity(pid int) (rootIdentity, bool) {
	stat, err := linuxReadStat(pid)
	if err != nil {
		return rootIdentity{}, false
	}
	return rootIdentity{pid: pid, startTicks: stat.startTicks}, true
}

// walkProcessTree walks all non-zombie descendants of root and reports
// whether any was born at or after the turn start, truncated down to tick
// resolution before an inclusive comparison (D5, AC-80). The comparison is
// performed entirely in the boot-tick domain: marker.bootTicks was computed
// once at stamp time (newTurnStartMarker) and every descendant's raw
// /proc/<pid>/stat ticks-since-boot is compared directly against it — no
// boot-time anchor is read here, so a wall-clock adjustment between stamp
// and probe cannot shift the comparison (see turnStartMarker's doc comment).
func walkProcessTree(ctx context.Context, root rootIdentity, marker turnStartMarker) string {
	if root.pid <= 0 {
		return probeResultUnknown
	}
	if ctx.Err() != nil {
		return probeResultUnknown
	}
	if !marker.hasBootTicks {
		// The boot anchor could not be read at stamp time (newTurnStartMarker).
		// Per the spec's failure rule, an unreadable anchor answers unknown,
		// never settled.
		return probeResultUnknown
	}

	// D5: identify the root by (pid, start time), never bare pid — reject a
	// reused PID whose current occupant's startTicks does not match what the
	// caller recorded, rather than trusting bare existence.
	rootStat, err := linuxReadStat(root.pid)
	if err != nil || rootStat.startTicks != root.startTicks {
		return probeResultUnknown
	}

	procs, ok := linuxCollectProcs(ctx)
	if !ok {
		return probeResultUnknown
	}

	// Build parent→children map.
	children := make(map[int][]int, len(procs))
	for pid, info := range procs {
		children[info.ppid] = append(children[info.ppid], pid)
	}

	// D5/AC-80: the turn start was already truncated down to tick resolution
	// when it was converted into ticks (newTurnStartMarker's integer
	// division), so a process born in the same tick counts as in-turn — the
	// error always falls toward "live".
	threshold := marker.bootTicks
	// visited guards against a ppid cycle in the snapshot (e.g. a pid recycled
	// mid-enumeration into its own descendant's slot) sending the walk into an
	// unbounded loop instead of terminating on its own.
	visited := map[int]bool{root.pid: true}
	queue := append([]int(nil), children[root.pid]...)
	for len(queue) > 0 {
		if ctx.Err() != nil {
			return probeResultUnknown
		}
		cur := queue[0]
		queue = queue[1:]
		if visited[cur] {
			continue
		}
		visited[cur] = true

		info, ok := procs[cur]
		if !ok {
			continue
		}
		if !info.zombie && info.startTicks >= threshold {
			return probeResultLive
		}
		queue = append(queue, children[cur]...)
	}

	return probeResultSettled
}

// linuxBootTime returns a wall-clock estimate of system boot time, derived
// from /proc/uptime read at this instant. Used as the single anchor every
// descendant's tick-domain start time in one walk is converted against, so
// all comparisons in the walk are internally consistent even though the
// anchor itself is a fresh read rather than one frozen at turn-start (a
// documented scope boundary — see the Linux clock-domain note in
// docs/specs/disambiguate-waiting/spec.md, "Start-time source and
// resolution").
func linuxBootTime() (time.Time, bool) {
	data, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return time.Time{}, false
	}
	fields := strings.Fields(string(data))
	if len(fields) == 0 {
		return time.Time{}, false
	}
	uptimeSeconds, err := strconv.ParseFloat(fields[0], 64)
	if err != nil || uptimeSeconds < 0 {
		return time.Time{}, false
	}
	return time.Now().Add(-time.Duration(uptimeSeconds * float64(time.Second))), true
}

type linuxProcStat struct {
	state      string
	ppid       int
	startTicks int64
}

// linuxReadStat reads the process state (field 3), parent PID (field 4), and
// start time in clock ticks since boot (field 22) from /proc/<pid>/stat.
func linuxReadStat(pid int) (linuxProcStat, error) {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return linuxProcStat{}, err
	}
	// /proc/<pid>/stat: "pid (comm) state ppid ... starttime ..."
	// The comm field may contain spaces and parentheses; skip past the last ')'.
	s := string(data)
	lastParen := strings.LastIndex(s, ")")
	if lastParen < 0 {
		return linuxProcStat{}, errors.New("malformed /proc/stat: no closing paren")
	}
	// Fields after comm, 1-indexed in the man page as fields 3+: [0]=state
	// [1]=ppid [2..18]=... [19]=starttime (field 22 overall = index 19 here).
	fields := strings.Fields(s[lastParen+1:])
	const starttimeIndex = 19 // field 22 - 3 (state is field 3, index 0 here)
	if len(fields) <= starttimeIndex {
		return linuxProcStat{}, fmt.Errorf("not enough fields in /proc/%d/stat", pid)
	}
	ppid, err := strconv.Atoi(fields[1])
	if err != nil {
		return linuxProcStat{}, err
	}
	startTicks, err := strconv.ParseInt(fields[starttimeIndex], 10, 64)
	if err != nil {
		return linuxProcStat{}, err
	}
	return linuxProcStat{state: fields[0], ppid: ppid, startTicks: startTicks}, nil
}
