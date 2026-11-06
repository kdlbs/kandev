package hostmemory

import (
	"errors"
	"strconv"
	"strings"
)

// overcommitStrict is the vm.overcommit_memory mode under which the kernel
// enforces CommitLimit. In the heuristic (0) and always (1) modes CommitLimit is
// only advisory and Committed_AS routinely exceeds it, so the commit pair says
// nothing about allocation failure there.
const overcommitStrict = "2"

// readingFromProc builds a Reading from the contents of /proc/meminfo and
// /proc/sys/vm/overcommit_memory. It carries no build tag so the parsing is
// exercised on every platform. overcommit is empty when the mode could not be
// read; the commit pair is then not judged.
func readingFromProc(meminfo, overcommit string) (Reading, error) {
	values := parseMeminfo(meminfo)
	total, ok := values["MemTotal"]
	if !ok {
		return Reading{Complete: false}, errors.New("/proc/meminfo has no MemTotal")
	}
	reading := Reading{TotalBytes: kib(total), Complete: true}
	if available, ok := values["MemAvailable"]; ok {
		reading.AvailableBytes = kib(available)
	} else {
		reading.AvailableBytes = kib(values["MemFree"] + values["Buffers"] + values["Cached"])
	}
	if strings.TrimSpace(overcommit) != overcommitStrict {
		return reading, nil
	}
	if limit, ok := values["CommitLimit"]; ok {
		if committed, ok := values["Committed_AS"]; ok {
			reading.CommitLimitBytes = kib(limit)
			reading.CommitUsedBytes = kib(committed)
			reading.CommitKnown = true
		}
	}
	return reading, nil
}

func parseMeminfo(content string) map[string]uint64 {
	values := make(map[string]uint64)
	for _, line := range strings.Split(content, "\n") {
		key, rest, found := strings.Cut(line, ":")
		if !found {
			continue
		}
		fields := strings.Fields(rest)
		if len(fields) == 0 {
			continue
		}
		value, err := strconv.ParseUint(fields[0], 10, 64)
		if err != nil {
			continue
		}
		values[strings.TrimSpace(key)] = value
	}
	return values
}

func kib(v uint64) uint64 { return v << 10 }
