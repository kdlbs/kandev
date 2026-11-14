//go:build linux

package process

import (
	"os"
	"strconv"
	"strings"
)

// procPageSize is the page size this build reads /proc/<pid>/statm against.
// Linux reports memory in pages, so the resident total is only correct against
// the size the kernel actually used; a wrong constant would silently understate
// or overstate every session's footprint.
var procPageSize = uint64(os.Getpagesize())

// footprintFromProcessTree measures every live descendant of rootPID by
// following the ppid links out of one /proc snapshot. Commit charge is not
// exposed per process on Linux, so CommittedBytes stays zero here and the
// resident total is the only byte figure reported.
func footprintFromProcessTree(rootPID int) ProcessFootprint {
	var footprint ProcessFootprint
	if rootPID <= 0 {
		return footprint
	}
	parentOf, ok := procParents()
	if !ok {
		footprint.unreadable()
		return footprint
	}
	for pid := range parentOf {
		if !descendsFromProcess(pid, rootPID, parentOf) {
			continue
		}
		resident, readable := procResidentBytes(pid)
		if !readable {
			footprint.unreadable()
			continue
		}
		footprint.add(0, resident)
	}
	return footprint
}

func procParents() (map[int]int, bool) {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil, false
	}
	parentOf := make(map[int]int, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		pid, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue
		}
		parent, ok := procParentPID(pid)
		if !ok {
			continue
		}
		parentOf[pid] = parent
	}
	if len(parentOf) == 0 {
		return nil, false
	}
	return parentOf, true
}

// procParentPID reads a process's ppid out of /proc/<pid>/stat. The comm field
// is parenthesised and may itself contain spaces and parentheses, so the fields
// after the final ") " are the only safe source of ppid.
func procParentPID(pid int) (int, bool) {
	stat, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return 0, false
	}
	commandEnd := strings.LastIndex(string(stat), ") ")
	if commandEnd < 0 {
		return 0, false
	}
	fields := strings.Fields(string(stat)[commandEnd+2:])
	if len(fields) < 2 || fields[0] == "Z" || fields[0] == "X" {
		return 0, false
	}
	parent, err := strconv.Atoi(fields[1])
	if err != nil {
		return 0, false
	}
	return parent, true
}

func procResidentBytes(pid int) (uint64, bool) {
	statm, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/statm")
	if err != nil {
		return 0, false
	}
	fields := strings.Fields(string(statm))
	if len(fields) < 2 {
		return 0, false
	}
	residentPages, err := strconv.ParseUint(fields[1], 10, 64)
	if err != nil {
		return 0, false
	}
	return residentPages * procPageSize, true
}
