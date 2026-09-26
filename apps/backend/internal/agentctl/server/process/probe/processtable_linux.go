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
// — see readLinuxStatFields.
const starttimeFieldIndex = 22 - 3

// kandevSessionIDEnvVar is the environment variable every descendant of an
// agentctl-launched process inherits, carrying the session it belongs to.
// It survives reparenting because environment inheritance is independent
// of the process tree.
const kandevSessionIDEnvVar = "KANDEV_SESSION_ID"

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
	return time.Now().Add(-time.Duration(uptimeSeconds * float64(time.Second))), nil
}

// readLinuxStatFields reads and tokenizes /proc/<pid>/stat, returning the
// fields starting from field 3 (state) — i.e. everything after pid+comm
// (fields 1-2), which readLinuxProcessStat and readLinuxStartTicks both
// index into via starttimeFieldIndex. ok is false only when pid does not
// exist; any other read or parse failure is an error.
func readLinuxStatFields(pid int) (fields []string, ok bool, err error) {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		if isLinuxProcessGone(err) {
			return nil, false, nil
		}
		return nil, false, err
	}

	// comm (field 2) is parenthesized and may itself contain spaces or
	// parens; the kernel guarantees the LAST ')' in the line closes it, so
	// everything after it is safe to split on whitespace.
	line := string(data)
	closeParen := strings.LastIndexByte(line, ')')
	if closeParen < 0 || closeParen+2 > len(line) {
		return nil, false, fmt.Errorf("malformed /proc/%d/stat", pid)
	}
	return strings.Fields(line[closeParen+2:]), true, nil
}

func readLinuxProcessStat(pid int, bootTime time.Time) (processInfo, bool, error) {
	fields, ok, err := readLinuxStatFields(pid)
	if err != nil || !ok {
		return processInfo{}, ok, err
	}
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
		PID:            pid,
		PPID:           ppid,
		StartTime:      startTime,
		Zombie:         fields[0] == linuxZombieState,
		StartTimeDatum: startTicks,
	}, true, nil
}

// readLinuxStartTicks re-reads pid's raw starttime ticks directly, without
// the boot-time anchoring readLinuxProcessStat applies to produce
// processInfo.StartTime. It backs environmentReader.StartTimeDatum, which
// match-only re-validation uses instead of a derived wall-clock value.
func readLinuxStartTicks(pid int) (int64, error) {
	fields, ok, err := readLinuxStatFields(pid)
	if err != nil {
		return 0, err
	}
	if !ok {
		return 0, fmt.Errorf("process %d not found", pid)
	}
	if len(fields) <= starttimeFieldIndex {
		return 0, fmt.Errorf("/proc/%d/stat has too few fields", pid)
	}
	return strconv.ParseInt(fields[starttimeFieldIndex], 10, 64)
}

// HasSessionID implements environmentReader by parsing /proc/<pid>/environ
// as discrete NUL-separated NAME=VALUE entries — never scanned as raw text
// — and comparing the first KANDEV_SESSION_ID entry's value for exact
// equality with sessionID.
func (linuxProcessTableReader) HasSessionID(pid int, sessionID string) (bool, error) {
	value, ok, err := readLinuxEnvVar(pid, kandevSessionIDEnvVar)
	if err != nil {
		return false, err
	}
	return ok && value == sessionID, nil
}

// StartTimeDatum implements environmentReader by re-reading pid's raw
// starttime ticks directly from /proc/<pid>/stat.
func (linuxProcessTableReader) StartTimeDatum(pid int) (int64, error) {
	return readLinuxStartTicks(pid)
}

// readLinuxEnvVar returns name's value from pid's environment and whether
// it was found.
func readLinuxEnvVar(pid int, name string) (string, bool, error) {
	data, err := readEnvironWithRetry(func() ([]byte, error) {
		return os.ReadFile(fmt.Sprintf("/proc/%d/environ", pid))
	})
	if err != nil {
		return "", false, err
	}
	value, ok := parseFirstEnvVar(data, name)
	return value, ok, nil
}

// environReadMaxAttempts bounds the retry below. A just-execve'd process's
// /proc/<pid>/environ can transiently read as empty, with no error, for a
// few microseconds before the kernel has that region ready — indistinguishable
// at read time from a genuinely empty environment. Retrying a *successful*
// empty read trades at most a few milliseconds of probe latency for
// correctness against that race; a read that returns an error is never
// retried, since an unreadable process is already a decided skip.
const environReadMaxAttempts = 4

// readEnvironWithRetry retries read while it keeps returning a successful
// but empty result, up to environReadMaxAttempts, backing off briefly
// between attempts.
func readEnvironWithRetry(read func() ([]byte, error)) ([]byte, error) {
	var data []byte
	var err error
	for attempt := 0; attempt < environReadMaxAttempts; attempt++ {
		data, err = read()
		if err != nil || len(data) > 0 {
			return data, err
		}
		if attempt < environReadMaxAttempts-1 {
			time.Sleep(time.Duration(1<<attempt) * time.Millisecond)
		}
	}
	return data, err
}

// parseFirstEnvVar reads a /proc/<pid>/environ-shaped blob — NUL-separated
// NAME=VALUE entries — as discrete entries, never as raw text, so a
// variable named e.g. "MY_KANDEV_SESSION_ID" never matches a lookup for
// "KANDEV_SESSION_ID". When name appears more than once, the first
// occurrence decides and the rest are ignored, so the answer never depends
// on how far the scan ran. A trailing empty field after the final NUL is
// not an entry.
func parseFirstEnvVar(data []byte, name string) (string, bool) {
	for _, entry := range strings.Split(string(data), "\x00") {
		if entry == "" {
			continue
		}
		eq := strings.IndexByte(entry, '=')
		if eq < 0 {
			continue
		}
		if entry[:eq] == name {
			return entry[eq+1:], true
		}
	}
	return "", false
}

// isLinuxProcessGone reports whether err indicates pid no longer exists: the
// ordinary os.IsNotExist race between listing and reading, plus ESRCH, which
// the kernel can also return for a pid that exited between the two.
func isLinuxProcessGone(err error) bool {
	return os.IsNotExist(err) || errors.Is(err, syscall.ESRCH)
}
