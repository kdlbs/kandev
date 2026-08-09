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

// walkProcessTree walks all non-zombie descendants of rootPID and reports
// whether any was born at or after turnRefTime, truncated down to tick
// resolution before an inclusive comparison (D5, AC-80). Start times are
// read from /proc/<pid>/stat field 22 (ticks since boot) and converted to
// wall-clock using a boot-time anchor read once per probe call, so every
// descendant in one walk is compared against the same anchor instant.
func walkProcessTree(ctx context.Context, rootPID int, turnRefTime time.Time) string {
	if rootPID <= 0 {
		return probeResultUnknown
	}
	if ctx.Err() != nil {
		return probeResultUnknown
	}

	bootTime, ok := linuxBootTime()
	if !ok {
		return probeResultUnknown
	}

	// Verify rootPID exists.
	if _, err := os.Stat(fmt.Sprintf("/proc/%d", rootPID)); err != nil {
		return probeResultUnknown
	}

	type procEntry struct {
		ppid      int
		startTime time.Time
		zombie    bool
	}

	entries, err := os.ReadDir("/proc")
	if err != nil {
		return probeResultUnknown
	}

	procs := make(map[int]procEntry, len(entries))
	for _, e := range entries {
		if ctx.Err() != nil {
			return probeResultUnknown
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
			ppid:      stat.ppid,
			startTime: bootTime.Add(time.Duration(stat.startTicks) * time.Second / linuxClockTicksPerSecond),
			zombie:    stat.state == "Z",
		}
	}

	// Build parent→children map.
	children := make(map[int][]int, len(procs))
	for pid, info := range procs {
		children[info.ppid] = append(children[info.ppid], pid)
	}

	// D5/AC-80: truncate the turn start down to source resolution before an
	// inclusive comparison, so a process born in the same tick counts as
	// in-turn — the error always falls toward "live".
	threshold := turnRefTime.Truncate(linuxProcStatResolution)
	queue := append([]int(nil), children[rootPID]...)
	for len(queue) > 0 {
		if ctx.Err() != nil {
			return probeResultUnknown
		}
		cur := queue[0]
		queue = queue[1:]

		info, ok := procs[cur]
		if !ok {
			continue
		}
		if !info.zombie && !info.startTime.Before(threshold) {
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
	// [1]=ppid [2..17]=... [18]=starttime (field 22 overall = index 19 here).
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
