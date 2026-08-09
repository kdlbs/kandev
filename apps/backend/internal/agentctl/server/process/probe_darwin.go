//go:build darwin

package process

import (
	"context"
	"time"

	"golang.org/x/sys/unix"
)

// darwinZombieState is BSD's SZOMB process-state constant (sys/proc.h:
// SIDL=1, SRUN=2, SSLEEP=3, SSTOP=4, SZOMB=5). Not exported by x/sys/unix.
const darwinZombieState = 5

// darwinStartTimeResolution is the microsecond resolution of kinfo_proc's
// p_starttime timeval (spec: "Start-time source and resolution").
const darwinStartTimeResolution = time.Microsecond

// walkProcessTree walks all non-zombie descendants of rootPID and reports
// whether any was born at or after turnRefTime, truncated down to
// microsecond resolution before an inclusive comparison (D5, AC-80).
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
		zombie    bool
	}

	procs := make(map[int]procInfo, len(allProcs))
	for i := range allProcs {
		kp := &allProcs[i]
		pid := int(kp.Proc.P_pid)
		ppid := int(kp.Eproc.Ppid)
		tv := kp.Proc.P_starttime
		st := time.Unix(tv.Sec, int64(tv.Usec)*1000)
		procs[pid] = procInfo{
			ppid:      ppid,
			startTime: st,
			zombie:    int(kp.Proc.P_stat) == darwinZombieState,
		}
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

	// D5/AC-80: truncate the turn start down to source resolution before an
	// inclusive comparison, so a process born in the same microsecond tick
	// counts as in-turn — the error always falls toward "live".
	threshold := turnRefTime.Truncate(darwinStartTimeResolution)
	// visited guards against a ppid cycle in the snapshot (e.g. a pid recycled
	// mid-enumeration into its own descendant's slot) sending the walk into an
	// unbounded loop instead of terminating on its own.
	visited := map[int]bool{rootPID: true}
	queue := append([]int(nil), children[rootPID]...)
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
		if !info.zombie && !info.startTime.Before(threshold) {
			return probeResultLive
		}
		queue = append(queue, children[cur]...)
	}

	return probeResultSettled
}
