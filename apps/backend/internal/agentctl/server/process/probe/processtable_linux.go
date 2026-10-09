//go:build linux

package probe

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// clockTicksPerSecond is Linux's USER_HZ, the tick rate /proc/<pid>/stat's
// starttime field (22) is expressed in. golang.org/x/sys/unix does not wrap
// sysconf(_SC_CLK_TCK), and there is no other cgo-free syscall to read it in
// this repo's Go toolchain (round-5 F8 asked Build to verify this during
// implementation). USER_HZ is a stable kernel ABI value fixed at 100 on
// every architecture the glibc-based Linux Kandev ships on actually runs.
const clockTicksPerSecond = 100

// linuxZombieState is /proc/<pid>/stat field 3's zombie state character.
const linuxZombieState = "Z"

// starttimeFieldIndex is /proc/<pid>/stat field 22 (starttime), re-indexed
// into the fields slice produced after splitting off pid+comm (fields 1-2)
// — see readLinuxProcessStat.
const starttimeFieldIndex = 22 - 3

type linuxProcessTableReader struct{}

func platformProcessTableReader() processTableReader {
	return linuxProcessTableReader{}
}

func (linuxProcessTableReader) Resolution() time.Duration {
	return time.Second / time.Duration(clockTicksPerSecond)
}

func (linuxProcessTableReader) ReadProcessTable() ([]processInfo, error) {
	bootTime, err := linuxBootTime()
	if err != nil {
		return nil, fmt.Errorf("probe: read boot time: %w", err)
	}

	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil, fmt.Errorf("probe: read /proc: %w", err)
	}

	table := make([]processInfo, 0, len(entries))
	for _, entry := range entries {
		pid, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue // not a pid directory (self, thread-self, ...)
		}

		info, ok, err := readLinuxProcessStat(pid, bootTime)
		if err != nil {
			// An unreadable stat mid-walk makes the whole snapshot
			// incomplete — surfaced as unknown by the caller, never a
			// shortened set (D5).
			return nil, err
		}
		if !ok {
			continue // exited between ReadDir and stat — absent, one snapshot
		}
		table = append(table, info)
	}
	return table, nil
}

// linuxBootTime anchors /proc/<pid>/stat's ticks-since-boot starttime field
// to agentctl's own wall clock via /proc/uptime (system uptime in seconds),
// NOT /proc/stat's btime (whole-second resolution, which would reproduce
// the banned `ps -eo lstart` failure mode — round-5 F8).
func linuxBootTime() (time.Time, error) {
	data, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return time.Time{}, err
	}
	fields := strings.Fields(string(data))
	if len(fields) == 0 {
		return time.Time{}, fmt.Errorf("unexpected /proc/uptime contents %q", data)
	}
	uptimeSeconds, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse /proc/uptime: %w", err)
	}
	now := time.Now()
	uptime := time.Duration(uptimeSeconds * float64(time.Second))
	bootTime := linuxBootTimeForClock(now, uptime, linuxStatBootTime(), linuxPID1StartTime())
	return bootTime, nil
}

// linuxBootTimeForClock normally anchors process start ticks to /proc/uptime.
// Some container time namespaces virtualize /proc/uptime while
// /proc/<pid>/stat's starttime remains measured from the host boot. In that
// case, /proc/stat's whole-second btime is used only to detect the namespace
// offset; adding PID 1's start tick restores the sub-second precision of the
// uptime and process start tick sources.
func linuxBootTimeForClock(now time.Time, uptime time.Duration, wallBootTime time.Time, pid1StartTime time.Duration) time.Time {
	bootTime := now.Add(-uptime)
	if wallBootTime.IsZero() || pid1StartTime <= 5*time.Second {
		return bootTime
	}
	wallUptime := now.Sub(wallBootTime)
	namespaceOffset := wallUptime - uptime
	if absoluteDuration(namespaceOffset-pid1StartTime) > 2*time.Second {
		return bootTime
	}
	return now.Add(-(uptime + pid1StartTime))
}

func absoluteDuration(value time.Duration) time.Duration {
	if value < 0 {
		return -value
	}
	return value
}

func linuxStatBootTime() time.Time {
	data, err := os.ReadFile("/proc/stat")
	if err != nil {
		return time.Time{}
	}
	for _, line := range strings.Split(string(data), "\n") {
		if !strings.HasPrefix(line, "btime ") {
			continue
		}
		seconds, err := strconv.ParseInt(strings.TrimPrefix(line, "btime "), 10, 64)
		if err != nil {
			return time.Time{}
		}
		return time.Unix(seconds, 0)
	}
	return time.Time{}
}

func linuxPID1StartTime() time.Duration {
	info, ok, err := readLinuxProcessStat(1, time.Time{})
	if err != nil || !ok {
		return 0
	}
	return info.StartTime.Sub(time.Time{})
}

func readLinuxProcessStat(pid int, bootTime time.Time) (processInfo, bool, error) {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		if isLinuxProcessGone(err) {
			return processInfo{}, false, nil
		}
		return processInfo{}, false, err
	}

	// comm (field 2) is parenthesized and may itself contain spaces or
	// parens; the kernel guarantees the LAST ')' in the line closes it, so
	// everything after it is safe to split on whitespace.
	line := string(data)
	closeParen := strings.LastIndexByte(line, ')')
	if closeParen < 0 || closeParen+2 > len(line) {
		return processInfo{}, false, fmt.Errorf("malformed /proc/%d/stat", pid)
	}
	fields := strings.Fields(line[closeParen+2:])
	if len(fields) <= starttimeFieldIndex {
		return processInfo{}, false, fmt.Errorf("/proc/%d/stat has too few fields", pid)
	}

	ppid, err := strconv.Atoi(fields[1])
	if err != nil {
		return processInfo{}, false, fmt.Errorf("parse ppid for pid %d: %w", pid, err)
	}
	startTicks, err := strconv.ParseInt(fields[starttimeFieldIndex], 10, 64)
	if err != nil {
		return processInfo{}, false, fmt.Errorf("parse starttime for pid %d: %w", pid, err)
	}

	startTime := bootTime.Add(time.Duration(startTicks) * (time.Second / time.Duration(clockTicksPerSecond)))

	return processInfo{
		PID:       pid,
		PPID:      ppid,
		StartTime: startTime,
		Zombie:    fields[0] == linuxZombieState,
	}, true, nil
}

func isLinuxProcessGone(err error) bool {
	return os.IsNotExist(err) || errors.Is(err, syscall.ESRCH)
}
