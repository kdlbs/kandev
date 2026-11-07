//go:build unix && !linux

package process

import (
	"context"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// footprintReadTimeout bounds the ps probe so a stalled host cannot hold an
// instance's footprint read open.
const footprintReadTimeout = 5 * time.Second

// footprintFromProcessTree measures every live descendant of rootPID using one
// bounded `ps` snapshot of the process table. Commit charge is not exposed per
// process on this platform, so CommittedBytes stays zero and the resident total
// is the only byte figure reported.
func footprintFromProcessTree(rootPID int) ProcessFootprint {
	var footprint ProcessFootprint
	if rootPID <= 0 {
		return footprint
	}
	parentOf, residentOf, ok := psProcessTable()
	if !ok {
		footprint.unreadable()
		return footprint
	}
	for pid := range parentOf {
		if !descendsFromProcess(pid, rootPID, parentOf) {
			continue
		}
		resident, readable := residentOf[pid]
		if !readable {
			footprint.unreadable()
			continue
		}
		footprint.add(0, resident)
	}
	return footprint
}

// psProcessTable returns pid -> ppid and pid -> resident bytes from one snapshot.
// A snapshot that cannot be taken yields ok=false so the caller reports nothing
// rather than an empty measurement that reads as "no processes".
func psProcessTable() (parentOf map[int]int, residentOf map[int]uint64, ok bool) {
	ctx, cancel := context.WithTimeout(context.Background(), footprintReadTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, "ps", "-A", "-o", "pid=,ppid=,rss=").Output()
	if err != nil {
		return nil, nil, false
	}
	parentOf = make(map[int]int)
	residentOf = make(map[int]uint64)
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 3 {
			continue
		}
		pid, pidErr := strconv.Atoi(fields[0])
		parent, parentErr := strconv.Atoi(fields[1])
		if pidErr != nil || parentErr != nil {
			continue
		}
		resident, residentErr := strconv.ParseUint(fields[2], 10, 64)
		if residentErr != nil {
			continue
		}
		parentOf[pid] = parent
		residentOf[pid] = resident * 1024
	}
	if len(parentOf) == 0 {
		return nil, nil, false
	}
	return parentOf, residentOf, true
}
