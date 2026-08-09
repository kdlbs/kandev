//go:build windows

package process

import (
	"context"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
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

	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return probeResultUnknown
	}
	defer windows.CloseHandle(snapshot)

	type procInfo struct {
		ppid uint32
	}

	procs := make(map[uint32]procInfo)
	var pe windows.ProcessEntry32
	pe.Size = uint32(unsafe.Sizeof(pe))
	if err := windows.Process32First(snapshot, &pe); err != nil {
		return probeResultUnknown
	}
	for {
		procs[pe.ProcessID] = procInfo{ppid: pe.ParentProcessID}
		if err := windows.Process32Next(snapshot, &pe); err != nil {
			break
		}
	}

	// Verify rootPID is alive.
	if _, ok := procs[uint32(rootPID)]; !ok {
		return probeResultUnknown
	}

	// Build parent→children map.
	children := make(map[uint32][]uint32, len(procs))
	for pid, info := range procs {
		children[info.ppid] = append(children[info.ppid], pid)
	}

	// BFS over descendants of rootPID.
	threshold := turnRefTime.Add(-probeStartTimeSlack)
	queue := append([]uint32(nil), children[uint32(rootPID)]...)
	for len(queue) > 0 {
		if ctx.Err() != nil {
			return probeResultUnknown
		}
		cur := queue[0]
		queue = queue[1:]

		startTime, err := windowsProcessStartTime(cur)
		if err != nil {
			// Process may have exited; skip but continue walking siblings.
			continue
		}
		if !startTime.Before(threshold) {
			return probeResultLive
		}
		queue = append(queue, children[cur]...)
	}

	return probeResultSettled
}

func windowsProcessStartTime(pid uint32) (time.Time, error) {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return time.Time{}, err
	}
	defer windows.CloseHandle(h)

	var creation, exit, kernel, user windows.Filetime
	if err := windows.GetProcessTimes(h, &creation, &exit, &kernel, &user); err != nil {
		return time.Time{}, err
	}
	return time.Unix(0, creation.Nanoseconds()), nil
}
