//go:build darwin

package probe

import (
	"fmt"
	"time"

	"golang.org/x/sys/unix"
)

// darwinZombieStat is kinfo_proc.kp_proc.p_stat's SZOMB value (BSD
// sys/proc.h) — the process finished exiting but has not yet been reaped by
// its parent. Excluded per D5.
const darwinZombieStat = 5

type darwinProcessTableReader struct{}

func platformProcessTableReader() processTableReader {
	return darwinProcessTableReader{}
}

func (darwinProcessTableReader) Resolution() time.Duration {
	// kinfo_proc.kp_proc.p_starttime is a struct timeval — already
	// microsecond-resolution wall-clock, the same clock domain as
	// time.Now(), so no boot-anchor translation is needed (unlike Linux).
	return time.Microsecond
}

func (darwinProcessTableReader) ReadProcessTable() ([]processInfo, error) {
	procs, err := unix.SysctlKinfoProcSlice("kern.proc.all")
	if err != nil {
		return nil, fmt.Errorf("probe: sysctl kern.proc.all: %w", err)
	}

	table := make([]processInfo, 0, len(procs))
	for _, p := range procs {
		table = append(table, processInfo{
			PID:       int(p.Proc.P_pid),
			PPID:      int(p.Eproc.Ppid),
			StartTime: time.Unix(p.Proc.P_starttime.Sec, int64(p.Proc.P_starttime.Usec)*1000),
			Zombie:    p.Proc.P_stat == darwinZombieStat,
		})
	}
	return table, nil
}
