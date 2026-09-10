package lifecycle

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"
	"golang.org/x/crypto/ssh"
)

// docs/specs/executors/requirements/ssh-transport-liveness.md
// AC-EXECUTORS-SSH-TRANSPORT-LIVENESS-001.10
const (
	defaultSSHKeepaliveInterval = 15 * time.Second
	defaultSSHKeepaliveDeadline = 45 * time.Second
)

// Tuning values for SSH session transport liveness. Package-level variables,
// not constants, so tests can shorten them (see goleak_test.go's TestMain,
// which zeroes the keepalive pair for this package's test binary — a
// non-positive value disables the watchdog per sshKeepaliveTuningValid).
var (
	sshKeepaliveInterval = defaultSSHKeepaliveInterval
	sshKeepaliveDeadline = defaultSSHKeepaliveDeadline
)

// sshBrokerPreflightTimeout bounds the credential-broker reachability
// preflight a broker-backed resume reset issues before it stops the stale
// remote controller. The preflight runs on the raw caller context today and
// is otherwise unbounded; 20s mirrors sshAgentctlCleanupTimeout, since both
// are reachability-scale checks, not long-running work.
var sshBrokerPreflightTimeout = 20 * time.Second

// ErrSSHTransportLost identifies a disposal or reuse decision made because a
// session's SSH transport was declared lost (deadline/probe-error teardown)
// or has become unresponsive, distinguishing that cause from an ordinary
// remote failure or a host that refused the call.
var ErrSSHTransportLost = errors.New("ssh: session transport lost")

const sshKeepaliveRequestType = "keepalive@openssh.com"

const (
	sshTransportLostReasonProbeError = "probe error"
	sshTransportLostReasonDeadline   = "liveness deadline exceeded"
)

// sshTransportClassification is the outcome of the single disposal reading
// AC-EXECUTORS-SSH-TRANSPORT-LIVENESS-002.8 requires: it governs every
// remote command a disposal path issues.
type sshTransportClassification int

const (
	sshTransportAnswering sshTransportClassification = iota
	sshTransportUnresponsive
	sshTransportLost
)

// sshKeepaliveTuningValid reports whether interval/deadline are usable
// (AC-EXECUTORS-SSH-TRANSPORT-LIVENESS-002.9): both positive, and the
// deadline strictly greater than twice the interval.
func sshKeepaliveTuningValid(interval, deadline time.Duration) bool {
	return interval > 0 && deadline > 0 && deadline > 2*interval
}

// sshKeepaliveWatchdog runs the two goroutines described in the system
// design's Control flow section for exactly one session SSH client: a
// prober that sends transport liveness probes one at a time, and a loop that
// decides when the transport is lost. It exposes start (via
// startSSHKeepaliveWatchdog), stop-deciding (stopAndAwaitLoop) and
// await-probe-exit (awaitProbeExit) — the first and last steps of a
// disposal, always in that order.
type sshKeepaliveWatchdog struct {
	interval time.Duration
	deadline time.Duration
	client   *ssh.Client

	stopCh   chan struct{}
	stopOnce sync.Once

	loopDone   chan struct{}
	proberDone chan struct{}

	recordReply func(time.Time)
	onLost      func(reason string, silence time.Duration)
}

// startSSHKeepaliveWatchdog starts a watchdog for client. Callers must have
// already validated interval/deadline with sshKeepaliveTuningValid.
// startedAt is the watchdog's start time, used as the silence interval's
// baseline until the first probe reply completes.
func startSSHKeepaliveWatchdog(
	client *ssh.Client,
	interval, deadline time.Duration,
	startedAt time.Time,
	recordReply func(time.Time),
	onLost func(reason string, silence time.Duration),
) *sshKeepaliveWatchdog {
	w := &sshKeepaliveWatchdog{
		interval:    interval,
		deadline:    deadline,
		client:      client,
		stopCh:      make(chan struct{}),
		loopDone:    make(chan struct{}),
		proberDone:  make(chan struct{}),
		recordReply: recordReply,
		onLost:      onLost,
	}
	// replies is capacity-1 and lossy by design: a coalesced reply is
	// harmless because another follows within one interval. errs is also
	// capacity-1 but is written at most once ever (the prober returns
	// immediately after), so the buffered send always succeeds instantly and
	// is never dropped — unlike a reply, the loop must always observe it.
	replies := make(chan time.Time, 1)
	errs := make(chan error, 1)
	go w.runProber(replies, errs)
	go w.runLoop(replies, errs, startedAt)
	return w
}

// runProber sends one transport liveness probe at a time, spaced by the
// interval (AC-EXECUTORS-SSH-TRANSPORT-LIVENESS-001.2). It has exactly two
// exits, both unconditional: a probe error (published on the guaranteed-
// delivery errs channel before the goroutine returns, so the loop always
// observes it) and the stop signal observed while idle between probes. It
// always terminates once the client is closed, since SendRequest then errors
// immediately.
func (w *sshKeepaliveWatchdog) runProber(replies chan<- time.Time, errs chan<- error) {
	defer close(w.proberDone)
	for {
		_, _, err := w.client.SendRequest(sshKeepaliveRequestType, true, nil)
		now := time.Now()
		if err != nil {
			errs <- err
			return
		}
		select {
		case replies <- now:
		default:
		}
		select {
		case <-w.stopCh:
			return
		case <-time.After(w.interval):
		}
	}
}

// runLoop decides when the transport is lost. Two precedence rules apply,
// because a select with several ready cases picks at random: a pending
// completed reply or probe error beats an expired deadline timer (handled by
// the non-blocking re-check inside the timer.C case below), and the stop
// signal beats both. The top-of-loop non-blocking check only catches a stop
// already signaled before that iteration's blocking select begins — it
// cannot catch a stop delivered concurrently with a probe error or an
// expired deadline, since the blocking select itself picks at random among
// whichever cases are ready together. So every path that is about to
// declare loss re-checks the stop signal immediately first and yields to it.
func (w *sshKeepaliveWatchdog) runLoop(replies <-chan time.Time, errs <-chan error, startedAt time.Time) {
	defer close(w.loopDone)
	lastReply := startedAt
	timer := time.NewTimer(w.deadline)
	defer timer.Stop()
	for {
		select {
		case <-w.stopCh:
			return
		default:
		}
		select {
		case <-w.stopCh:
			return
		case at := <-replies:
			w.handleReply(at, &lastReply, timer)
		case <-errs:
			if w.stopRequested() {
				return
			}
			w.declareLost(sshTransportLostReasonProbeError, time.Since(lastReply))
			return
		case <-timer.C:
			// A completed reply or a probe error may have arrived in the
			// same instant the deadline fired. Either wins over the timer
			// alone.
			select {
			case at := <-replies:
				w.handleReply(at, &lastReply, timer)
			case err := <-errs:
				_ = err
				if w.stopRequested() {
					return
				}
				w.declareLost(sshTransportLostReasonProbeError, time.Since(lastReply))
				return
			default:
				if w.stopRequested() {
					return
				}
				if w.handleDeadline(lastReply, timer) {
					return
				}
			}
		}
	}
}

// stopRequested reports whether the stop signal has been raised, without
// blocking. Used immediately before every action that would declare
// transport loss, so a stop delivered in the same instant as a probe error
// or an expired deadline always wins.
func (w *sshKeepaliveWatchdog) stopRequested() bool {
	select {
	case <-w.stopCh:
		return true
	default:
		return false
	}
}

func (w *sshKeepaliveWatchdog) handleReply(at time.Time, lastReply *time.Time, timer *time.Timer) {
	*lastReply = at
	if w.recordReply != nil {
		w.recordReply(*lastReply)
	}
	if !timer.Stop() {
		select {
		case <-timer.C:
		default:
		}
	}
	timer.Reset(w.deadline)
}

func (w *sshKeepaliveWatchdog) handleDeadline(lastReply time.Time, timer *time.Timer) bool {
	silence := time.Since(lastReply)
	if silence > w.deadline {
		w.declareLost(sshTransportLostReasonDeadline, silence)
		return true
	}
	// Spurious wake (the timer fired just as the deadline's boundary was
	// reached but the strictly-greater check above didn't trip): reset for
	// the remaining time rather than looping busily.
	remaining := w.deadline - silence
	if remaining <= 0 {
		remaining = time.Millisecond
	}
	timer.Reset(remaining)
	return false
}

func (w *sshKeepaliveWatchdog) declareLost(reason string, silence time.Duration) {
	if w.onLost != nil {
		w.onLost(reason, silence)
	}
}

// stopAndAwaitLoop signals the watchdog to stop and waits for the watchdog
// loop — not the prober — to exit. Bounded: the loop is either parked in its
// select, already returned, or performing a teardown (two local handle
// closes and one log line), so this never touches the network. Idempotent
// and safe on a nil-free zero watchdog that never started.
func (w *sshKeepaliveWatchdog) stopAndAwaitLoop() {
	w.stopOnce.Do(func() { close(w.stopCh) })
	<-w.loopDone
}

// awaitProbeExit waits for the prober goroutine to exit. Call after
// stopAndAwaitLoop and after the session SSH client has been closed: the
// close is what unblocks a prober mid-SendRequest, whether idle or wedged.
func (w *sshKeepaliveWatchdog) awaitProbeExit() {
	<-w.proberDone
}

// classifySSHTransportLocked reads state's transport-lost marker and, when a
// watchdog is running, its silence interval, and classifies the transport
// (AC-EXECUTORS-SSH-TRANSPORT-LIVENESS-002.8). Callers must hold the
// executor mutex — the marker and the last-probe-reply timestamp it reads
// are guarded by it.
func classifySSHTransportLocked(state *sshSessionState) sshTransportClassification {
	if state.transportLost {
		return sshTransportLost
	}
	if state.watchdog == nil {
		return sshTransportAnswering
	}
	silence := time.Since(state.lastProbeReply)
	if silence > 2*state.watchdog.interval {
		return sshTransportUnresponsive
	}
	return sshTransportAnswering
}

// closeForwarderOnce closes state's local port forward through its
// once-guard (AC-EXECUTORS-SSH-TRANSPORT-LIVENESS-002.7): whichever of
// transport teardown or a disposal path reaches it first performs the
// close, and the other observes the same result.
func (r *SSHExecutor) closeForwarderOnce(state *sshSessionState) error {
	state.forwarderCloseOnce.Do(func() {
		if state.forwarder != nil {
			state.forwarderCloseErr = state.forwarder.Close()
		}
	})
	return state.forwarderCloseErr
}

// closeClientOnce closes state's session SSH client through its own
// once-guard, independent of the forwarder's.
func (r *SSHExecutor) closeClientOnce(state *sshSessionState) error {
	state.clientCloseOnce.Do(func() {
		if state.client != nil {
			state.clientCloseErr = r.closeSSHClient(state.client)
		}
	})
	return state.clientCloseErr
}

// markTransportLost sets state's transport-lost marker without performing a
// full transport teardown. Used by the disposal backstop
// (AC-EXECUTORS-SSH-TRANSPORT-LIVENESS-002.8): it closes only the client
// through its once-guard and must not log the teardown warning, so it is not
// "transport teardown" under the spec's Terminology.
func (r *SSHExecutor) markTransportLost(state *sshSessionState) {
	r.mu.Lock()
	state.transportLost = true
	r.mu.Unlock()
}

// isTransportLost reports state's transport-lost marker under the executor
// mutex.
func (r *SSHExecutor) isTransportLost(state *sshSessionState) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return state.transportLost
}

// transportTeardown performs transport teardown for state
// (AC-EXECUTORS-SSH-TRANSPORT-LIVENESS-001.5): set the marker, close the
// forward then the client (each through its once-guard), and log one warning
// naming the executor instance, remote host, and silence interval. Runs at
// most once per session (AC-EXECUTORS-SSH-TRANSPORT-LIVENESS-002.5); it does
// not remove the session from the tracked-sessions map
// (AC-EXECUTORS-SSH-TRANSPORT-LIVENESS-002.6).
func (r *SSHExecutor) transportTeardown(instanceID string, state *sshSessionState, reason string, silence time.Duration) {
	state.teardownOnce.Do(func() {
		r.mu.Lock()
		state.transportLost = true
		r.mu.Unlock()
		forwarderErr := r.closeForwarderOnce(state)
		clientErr := r.closeClientOnce(state)
		r.logTransportLoss(instanceID, state, reason, silence, forwarderErr, clientErr)
	})
}

func (r *SSHExecutor) logTransportLoss(instanceID string, state *sshSessionState, reason string, silence time.Duration, forwarderErr, clientErr error) {
	host := ""
	if state.target != nil {
		host = state.target.Host
	}
	fields := []zap.Field{
		zap.String("instance_id", instanceID),
		zap.String("host", host),
		zap.Duration("silence_interval", silence),
		zap.String("reason", reason),
	}
	if forwarderErr != nil {
		fields = append(fields, zap.NamedError("forwarder_close_error", forwarderErr))
	}
	if clientErr != nil {
		fields = append(fields, zap.NamedError("client_close_error", clientErr))
	}
	r.logger.Warn("ssh session transport lost", fields...)
}

// startWatchdogLocked starts a watchdog for state's session SSH client, in
// the same critical section that recorded state in r.sessions
// (AC-EXECUTORS-SSH-TRANSPORT-LIVENESS-001.1). Callers must hold r.mu. A
// non-positive interval/deadline, or a deadline not greater than twice the
// interval, leaves state.watchdog nil and every existing disposal path
// unchanged (AC-EXECUTORS-SSH-TRANSPORT-LIVENESS-002.9).
func (r *SSHExecutor) startWatchdogLocked(instanceID string, state *sshSessionState) {
	interval, deadline := sshKeepaliveInterval, sshKeepaliveDeadline
	if !sshKeepaliveTuningValid(interval, deadline) {
		r.logger.Debug("ssh: not starting transport watchdog", zap.String("instance_id", instanceID))
		return
	}
	startedAt := time.Now()
	state.lastProbeReply = startedAt
	state.watchdog = startSSHKeepaliveWatchdog(state.client, interval, deadline, startedAt,
		func(at time.Time) {
			r.mu.Lock()
			state.lastProbeReply = at
			r.mu.Unlock()
		},
		func(reason string, silence time.Duration) {
			r.transportTeardown(instanceID, state, reason, silence)
		},
	)
}

// stopWatchdogAndWait signals state's watchdog to stop and waits for its
// loop to exit. A no-op on a session that never started one.
func (r *SSHExecutor) stopWatchdogAndWait(state *sshSessionState) {
	if state.watchdog != nil {
		state.watchdog.stopAndAwaitLoop()
	}
}

// awaitProbeExit waits for state's watchdog's prober to exit. A no-op on a
// session that never started one.
func (r *SSHExecutor) awaitProbeExit(state *sshSessionState) {
	if state.watchdog != nil {
		state.watchdog.awaitProbeExit()
	}
}

// runWithTransportBackstop runs fn — a remote command issued over state's
// session SSH client — on its own goroutine and races it against timeout
// (AC-EXECUTORS-SSH-TRANSPORT-LIVENESS-002.8). fn is expected to carry its
// own bound (typically a context.WithTimeout of the same duration) for the
// ordinary case where it is already executing; the backstop exists for the
// harder case where fn is blocked opening its SSH channel, which no context
// bounds. On timeout it closes the client through its once-guard — the only
// thing that releases a call blocked in Client.NewSession — marks the client
// transport-lost, and waits for fn's goroutine to actually exit before
// returning, so the abandoned command never outlives this call.
func (r *SSHExecutor) runWithTransportBackstop(state *sshSessionState, timeout time.Duration, fn func() error) error {
	done := make(chan error, 1)
	go func() { done <- fn() }()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case err := <-done:
		return err
	case <-timer.C:
		// fn may have completed in the same instant the timer fired — a
		// select with both cases ready picks at random. Recheck done
		// before treating this as a timeout, so a command that actually
		// finished is never reported as abandoned.
		select {
		case err := <-done:
			return err
		default:
		}
		r.markTransportLost(state)
		_ = r.closeClientOnce(state)
		if err := <-done; err != nil {
			return err
		}
		return fmt.Errorf("ssh: remote command exceeded %s and was abandoned", timeout)
	}
}
