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

// walkProcessTree walks all descendants of rootPID and reports whether any
// process born at or after (turnRefTime - probeStartTimeSlack) is still alive.
// Returns "live", "settled", or "unknown".
//
// Start times are read from the mtime of /proc/<pid> which the Linux kernel
// sets to the process's start time (task->start_time).
func walkProcessTree(ctx context.Context, rootPID int, turnRefTime time.Time) string {
	if rootPID <= 0 {
		return probeResultUnknown
	}
	if ctx.Err() != nil {
		return probeResultUnknown
	}

	// Verify rootPID exists.
	if _, err := os.Stat(fmt.Sprintf("/proc/%d", rootPID)); err != nil {
		return probeResultUnknown
	}

	// Snapshot all processes once to build a parent→children map.
	// procEntry holds only what we need for the BFS.
	type procEntry struct {
		ppid      int
		startTime time.Time
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
		info, err := os.Stat(fmt.Sprintf("/proc/%d", pid))
		if err != nil {
			continue
		}
		ppid, err := linuxReadPPID(pid)
		if err != nil {
			continue
		}
		procs[pid] = procEntry{ppid: ppid, startTime: info.ModTime()}
	}

	// Build parent→children map.
	children := make(map[int][]int, len(procs))
	for pid, info := range procs {
		children[info.ppid] = append(children[info.ppid], pid)
	}

	// BFS over descendants of rootPID.
	threshold := turnRefTime.Add(-probeStartTimeSlack)
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
		if !info.startTime.Before(threshold) {
			return probeResultLive
		}
		queue = append(queue, children[cur]...)
	}

	return probeResultSettled
}

// linuxReadPPID reads the parent PID for pid from /proc/<pid>/stat.
func linuxReadPPID(pid int) (int, error) {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return 0, err
	}
	// /proc/<pid>/stat: "pid (comm) state ppid ..."
	// The comm field may contain spaces and parentheses; skip past the last ')'.
	s := string(data)
	lastParen := strings.LastIndex(s, ")")
	if lastParen < 0 {
		return 0, errors.New("malformed /proc/stat: no closing paren")
	}
	fields := strings.Fields(s[lastParen+1:])
	// After comm: [0]=state [1]=ppid ...
	if len(fields) < 2 {
		return 0, fmt.Errorf("not enough fields in /proc/%d/stat", pid)
	}
	return strconv.Atoi(fields[1])
}
