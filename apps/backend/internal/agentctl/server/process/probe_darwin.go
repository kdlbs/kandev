//go:build darwin

package process

import (
	"context"
	"time"

	"golang.org/x/sys/unix"
)

// walkProcessTree walks all descendants of rootPID and reports whether any
// process born at or after (turnRefTime - probeStartTimeSlack) is still alive.
// Returns "live", "settled", or "unknown".
func walkProcessTree(ctx context.Context, rootPID int, turnRefTime time.Time) string {
	if rootPID <= 0 {
		return probeResultUnknown
	}
	if ctx.Err() != nil {
		return probeResultUnknown
	}

	allProcs, err := unix.SysctlKinfoProcSlice("kern.proc.all")
	if err != nil {
		return probeResultUnknown
	}

	type procInfo struct {
		ppid      int
		startTime time.Time
	}

	procs := make(map[int]procInfo, len(allProcs))
	for i := range allProcs {
		kp := &allProcs[i]
		pid := int(kp.Proc.P_pid)
		ppid := int(kp.Eproc.Ppid)
		tv := kp.Proc.P_starttime
		st := time.Unix(tv.Sec, int64(tv.Usec)*1000)
		procs[pid] = procInfo{ppid: ppid, startTime: st}
	}

	// Verify rootPID is alive.
	if _, ok := procs[rootPID]; !ok {
		return probeResultUnknown
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
