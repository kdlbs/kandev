package procstat

import (
	"strconv"
	"strings"
)

// StatMatchesLiveGroup recognizes a live group member in a Linux /proc stat record.
func StatMatchesLiveGroup(stat string, pgid int) bool {
	commandEnd := strings.LastIndex(stat, ") ")
	if commandEnd < 0 {
		return false
	}
	fields := strings.Fields(stat[commandEnd+2:])
	if len(fields) < 3 || fields[0] == "Z" || fields[0] == "X" {
		return false
	}
	group, err := strconv.Atoi(fields[2])
	return err == nil && group == pgid
}
