//go:build linux

package sshreconcile

import (
	"context"
	"errors"
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/unix"

	"github.com/kandev/kandev/internal/common/processidentity"
	"github.com/kandev/kandev/internal/common/sshlaunch"
)

// terminationWait bounds how long the helper observes the pidfd after
// SIGTERM. It is shorter than the SSH cleanup deadline so a still-live
// controller is reported unresolved instead of letting the channel time out.
const terminationWait = 5 * time.Second

// maxProcFileBytes bounds every procfs read. The values used here are a stat
// line, a command line, and an environment block.
const maxProcFileBytes = 256 * 1024

// beforeSignalRecheck is a test-only synchronization point between process
// admission and the final directory check. Production leaves it as a no-op.
var beforeSignalRecheck = func() {}

func reconcileProcess(ctx context.Context, receipt sshlaunch.Receipt, mode sshlaunch.ReconcileMode) sshlaunch.ReconcileResult {
	if receipt.UID != os.Geteuid() {
		return sshlaunch.Unresolved(sshlaunch.ReasonIdentityConflict, "receipt uid does not match the helper user")
	}
	handle, failure, ok := openProcessHandle(receipt.Process.PID)
	if !ok {
		if failure.Outcome == sshlaunch.OutcomeAbsent {
			return absentOutcome(receipt)
		}
		return failure
	}
	defer handle.Close()
	if failure, ok := handle.admit(receipt); !ok {
		if failure.Outcome == sshlaunch.OutcomeAbsent {
			return absentOutcome(receipt)
		}
		return failure
	}
	if mode == sshlaunch.ReconcileModeVerify {
		// Verify mode never signals or removes anything; the caller still owns
		// its guarded stop.
		return sshlaunch.ReconcileResult{Outcome: sshlaunch.OutcomeVerified}
	}
	beforeSignalRecheck()
	// Recheck the disposable path immediately before signalling: a directory
	// that appeared after the first inspection must survive untouched.
	if failure, ok := directoryAbsenceFailure(receipt.Owner.SessionDir, receipt.Owner.TaskDir); !ok {
		return failure
	}
	if err := unix.PidfdSendSignal(handle.pidfd, syscall.SIGTERM, nil, 0); err != nil {
		if errors.Is(err, unix.ESRCH) {
			return absentOutcome(receipt)
		}
		return sshlaunch.Unresolved(sshlaunch.ReasonProbeUnknown, "signal delivery failed")
	}
	if err := handle.waitExit(ctx); err != nil && !errors.Is(err, errProcessGone) {
		return sshlaunch.Unresolved(sshlaunch.ReasonTerminationUnconfirmed, "controller did not exit after SIGTERM")
	}
	return settledOutcome(receipt, sshlaunch.OutcomeStopped)
}

// absentOutcome completes only when the recorded controller and every session
// child it launched are gone. A surviving child keeps the owner unresolved so
// the cleanup inventory is not released under a live agent process.
func absentOutcome(receipt sshlaunch.Receipt) sshlaunch.ReconcileResult {
	// A recorded birth token from a previous boot cannot describe a process on
	// this host. The owner is unresolved rather than absent because the session
	// path may now belong to a replacement launched after the reboot.
	matches, err := processidentity.BirthBootMatches(receipt.Process)
	if err != nil {
		if errors.Is(err, processidentity.ErrUnverifiableIdentity) {
			return sshlaunch.Unresolved(sshlaunch.ReasonUnsupportedIdentity,
				"recorded launch boot identity is unavailable")
		}
		return sshlaunch.Unresolved(sshlaunch.ReasonTerminationUnconfirmed,
			"recorded launch boot identity could not be verified")
	}
	if !matches {
		return sshlaunch.Unresolved(sshlaunch.ReasonIdentityConflict,
			"recorded launch belongs to a previous boot")
	}
	return settledOutcome(receipt, sshlaunch.OutcomeAbsent)
}

func settledOutcome(receipt sshlaunch.Receipt, outcome sshlaunch.ReconcileOutcome) sshlaunch.ReconcileResult {
	remaining, err := processidentity.SessionMembersRemain(receipt.Process)
	if err != nil {
		if errors.Is(err, processidentity.ErrUnverifiableIdentity) {
			return sshlaunch.Unresolved(sshlaunch.ReasonUnsupportedIdentity,
				"session child identity cannot be verified")
		}
		return sshlaunch.Unresolved(sshlaunch.ReasonTerminationUnconfirmed,
			"controller is gone but its session children could not be inspected")
	}
	if remaining {
		return sshlaunch.Unresolved(sshlaunch.ReasonTerminationUnconfirmed,
			"a session child survived the controller")
	}
	return sshlaunch.ReconcileResult{Outcome: outcome}
}

var errProcessGone = errors.New("process exited")

// processHandle retains an opened process-directory reference and a pidfd for
// one PID. Signals travel only through the pidfd, so a recycled PID can never
// receive them.
type processHandle struct {
	procDir int
	pidfd   int
}

func openProcessHandle(pid int) (*processHandle, sshlaunch.ReconcileResult, bool) {
	// The pidfd must be opened before the process-directory handle. If the PID
	// is recycled in between, the directory describes the replacement while the
	// pidfd keeps the original process reference. The birth-identity check then
	// reads the replacement's token, finds a mismatch, and refuses. Opening the
	// directory first would allow the opposite pairing: verified evidence from
	// the original directory plus a pidfd for the replacement that would
	// receive the signal.
	pidfd, err := unix.PidfdOpen(pid, 0)
	if errors.Is(err, unix.ESRCH) {
		return nil, sshlaunch.ReconcileResult{Outcome: sshlaunch.OutcomeAbsent}, false
	}
	if errors.Is(err, unix.ENOSYS) || errors.Is(err, unix.EINVAL) || errors.Is(err, unix.EPERM) {
		return nil, sshlaunch.Unresolved(sshlaunch.ReasonUnsupportedIdentity, "pidfd is unavailable"), false
	}
	if err != nil {
		return nil, sshlaunch.Unresolved(sshlaunch.ReasonProbeUnknown, "process handle could not be opened"), false
	}
	procDir, err := unix.Open("/proc/"+strconv.Itoa(pid), unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if errors.Is(err, unix.ENOENT) || errors.Is(err, unix.ESRCH) {
		_ = unix.Close(pidfd)
		return nil, sshlaunch.ReconcileResult{Outcome: sshlaunch.OutcomeAbsent}, false
	}
	if err != nil {
		_ = unix.Close(pidfd)
		return nil, sshlaunch.Unresolved(sshlaunch.ReasonProbeUnknown, "process directory could not be opened"), false
	}
	return &processHandle{procDir: procDir, pidfd: pidfd}, sshlaunch.ReconcileResult{}, true
}

func (h *processHandle) Close() {
	if h.pidfd >= 0 {
		_ = unix.Close(h.pidfd)
		h.pidfd = -1
	}
	if h.procDir >= 0 {
		_ = unix.Close(h.procDir)
		h.procDir = -1
	}
}

// admit re-proves the recorded launch evidence through the retained handles.
// Only a live process with the exact birth identity, launch marker,
// command/workdir, and owner binding proceeds to the signal.
func (h *processHandle) admit(receipt sshlaunch.Receipt) (sshlaunch.ReconcileResult, bool) {
	statData, err := h.readProcFile("stat")
	if err != nil {
		return sshlaunch.Unresolved(sshlaunch.ReasonProbeUnknown, "process identity could not be read"), false
	}
	bootID, err := os.ReadFile("/proc/sys/kernel/random/boot_id")
	if err != nil {
		return sshlaunch.Unresolved(sshlaunch.ReasonProbeUnknown, "kernel boot identity could not be read"), false
	}
	state, err := processidentity.InspectProcStat(receipt.Process, string(bootID), statData)
	switch {
	case err != nil:
		if errors.Is(err, processidentity.ErrUnverifiableIdentity) {
			return sshlaunch.Unresolved(sshlaunch.ReasonUnsupportedIdentity, "process birth identity is unavailable"), false
		}
		return sshlaunch.Unresolved(sshlaunch.ReasonProbeUnknown, "process identity could not be verified"), false
	case state == processidentity.StateExited:
		return sshlaunch.ReconcileResult{Outcome: sshlaunch.OutcomeAbsent}, false
	case state != processidentity.StateAlive:
		return sshlaunch.Unresolved(sshlaunch.ReasonIdentityConflict, "recorded process identity changed"), false
	}

	cmdline, err := h.readProcFile("cmdline")
	if err != nil {
		return sshlaunch.Unresolved(sshlaunch.ReasonProbeUnknown, "process command line could not be read"), false
	}
	if !commandLineMatches(cmdline, receipt.Owner.TaskDir) {
		return sshlaunch.Unresolved(sshlaunch.ReasonIdentityConflict, "process command line does not match the recorded owner"), false
	}
	statusData, err := h.readProcFile("status")
	if err != nil {
		return sshlaunch.Unresolved(sshlaunch.ReasonProbeUnknown, "process credentials could not be read"), false
	}
	effectiveUID, err := parseEffectiveUID(statusData)
	if err != nil {
		return sshlaunch.Unresolved(sshlaunch.ReasonProbeUnknown, "process credentials could not be parsed"), false
	}
	if effectiveUID != receipt.UID {
		return sshlaunch.Unresolved(sshlaunch.ReasonIdentityConflict, "process effective uid does not match the receipt"), false
	}
	environ, err := h.readProcFile("environ")
	if err != nil {
		return sshlaunch.Unresolved(sshlaunch.ReasonProbeUnknown, "process environment could not be read"), false
	}
	if !launchMarkerMatches(environ, receipt.Generation) {
		return sshlaunch.Unresolved(sshlaunch.ReasonIdentityConflict, "process launch generation does not match the receipt"), false
	}
	return sshlaunch.ReconcileResult{}, true
}

// parseEffectiveUID reads the effective UID from a procfs status payload.
// Format: "Uid:	<real>	<effective>	<saved>	<filesystem>".
func parseEffectiveUID(data []byte) (int, error) {
	for _, line := range strings.Split(string(data), "\n") {
		value, ok := strings.CutPrefix(line, "Uid:")
		if !ok {
			continue
		}
		fields := strings.Fields(value)
		if len(fields) < 2 {
			return 0, errors.New("status Uid line is incomplete")
		}
		uid, err := strconv.Atoi(fields[1])
		if err != nil || uid < 0 {
			return 0, fmt.Errorf("status effective uid is invalid: %w", err)
		}
		return uid, nil
	}
	return 0, errors.New("status has no Uid line")
}

// readProcFile reads one entry through the opened process-directory handle, so
// every probe describes the same process the pidfd refers to.
func (h *processHandle) readProcFile(name string) ([]byte, error) {
	fd, err := unix.Openat(h.procDir, name, unix.O_RDONLY|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(fd), name)
	defer func() { _ = file.Close() }()
	buf := make([]byte, maxProcFileBytes)
	n, err := file.Read(buf)
	if err != nil && n == 0 {
		return nil, err
	}
	return buf[:n], nil
}

func (h *processHandle) waitExit(ctx context.Context) error {
	deadline := time.NewTimer(terminationWait)
	defer deadline.Stop()
	pollFds := []unix.PollFd{{Fd: int32(h.pidfd), Events: unix.POLLIN}}
	for {
		ready, err := unix.Poll(pollFds, 50)
		if err != nil && !errors.Is(err, unix.EINTR) {
			return fmt.Errorf("wait for controller exit: %w", err)
		}
		if ready > 0 {
			// Only a readable pidfd is exit evidence; a revoked signal state
			// (POLLERR) is not.
			if pollFds[0].Revents&unix.POLLIN != 0 {
				return errProcessGone
			}
			return errors.New("process handle reported an unexpected state")
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline.C:
			return errors.New("termination deadline expired")
		default:
		}
	}
}

// commandLineMatches requires an agentctl executable and this owner's exact
// workdir. A command line alone never grants authority; this check only
// prevents signalling a process whose retained evidence disagrees.
func commandLineMatches(data []byte, taskDir string) bool {
	args := splitNulList(data)
	if len(args) == 0 || !slices.ContainsFunc(args, isAgentctlExecutable) {
		return false
	}
	for index := 0; index < len(args); index++ {
		arg := args[index]
		if value, ok := strings.CutPrefix(arg, "--workdir="); ok && value == taskDir {
			return true
		}
		if arg == "--workdir" && index+1 < len(args) && args[index+1] == taskDir {
			return true
		}
	}
	return false
}

func isAgentctlExecutable(arg string) bool {
	base := arg
	if slash := strings.LastIndexByte(base, '/'); slash >= 0 {
		base = base[slash+1:]
	}
	return base == "agentctl"
}

func launchMarkerMatches(data []byte, generation string) bool {
	marker := sshlaunch.GenerationEnv + "=" + generation
	return slices.Contains(splitNulList(data), marker)
}

func splitNulList(data []byte) []string {
	raw := strings.Split(string(data), "\x00")
	values := raw[:0]
	for _, value := range raw {
		if value != "" {
			values = append(values, value)
		}
	}
	return values
}
