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

// captureRootIdentity reads pid's current kinfo_proc p_starttime, to be
// compared later by walkProcessTree against a fresh read of whatever process
// currently holds that PID (D5). p_starttime is a raw kernel timeval fixed
// for the lifetime of the process, so re-reading it later for a still-alive
// process returns the identical value with no jitter.
func captureRootIdentity(pid int) (rootIdentity, bool) {
	kp, err := unix.SysctlKinfoProc("kern.proc.pid", pid)
	if err != nil {
		return rootIdentity{}, false
	}
	tv := kp.Proc.P_starttime
	return rootIdentity{pid: pid, startTime: time.Unix(tv.Sec, int64(tv.Usec)*1000)}, true
}

// newTurnStartMarker records the turn-start wall-clock stamp directly:
// Darwin's p_starttime is already wall-clock (no boot-tick domain to
// bridge), so no anchor conversion is needed — see turnStartMarker's doc
// comment.
func newTurnStartMarker(t time.Time) turnStartMarker {
	return turnStartMarker{wallTime: t}
}

// walkProcessTree walks all non-zombie descendants of root and reports
// whether any was born at or after the turn start, truncated down to
// microsecond resolution before an inclusive comparison (D5, AC-80).
func walkProcessTree(ctx context.Context, root rootIdentity, marker turnStartMarker) string {
	if root.pid <= 0 {
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

	// D5: identify the root by (pid, start time), never bare pid — reject a
	// reused PID whose current occupant's start time does not match what the
	// caller recorded, rather than trusting bare existence.
	rootInfo, ok := procs[root.pid]
	if !ok || !rootInfo.startTime.Equal(root.startTime) {
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
	threshold := marker.wallTime.Truncate(darwinStartTimeResolution)
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
		if !info.zombie && !info.startTime.Before(threshold) {
			return probeResultLive
		}
		queue = append(queue, children[cur]...)
	}

	return probeResultSettled
}
