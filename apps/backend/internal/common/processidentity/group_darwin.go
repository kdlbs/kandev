//go:build darwin

package processidentity

import "golang.org/x/sys/unix"

// GroupMembers lists live processes whose process group ID is pgid. It only
// reads the process table; a read error is returned, never read as an empty group.
func GroupMembers(pgid int) ([]int, error) {
	if pgid <= 0 {
		return nil, ErrUnverifiableIdentity
	}
	procs, err := unix.SysctlKinfoProcSlice("kern.proc.all")
	if err != nil {
		return nil, err
	}
	var members []int
	for _, proc := range procs {
		if proc.Proc.P_pid <= 0 || proc.Proc.P_stat == 5 || int(proc.Eproc.Pgid) != pgid {
			continue
		}
		members = append(members, int(proc.Proc.P_pid))
	}
	return members, nil
}
