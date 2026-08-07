package acpdbg

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// RunConfig is passed to Run. Command is the argv to spawn; the env is
// inherited from the parent process.
type RunConfig struct {
	// AgentID is a short identifier for the run, used in the JSONL meta
	// start entry. For --exec runs, the caller supplies a synthetic id
	// derived from the executable basename.
	AgentID string
	// Command is the argv to spawn. First element is the executable, rest
	// are arguments. No shell expansion.
	Command []string
	// Workdir is the child process cwd. If empty, a fresh kandev-acpdbg-*
	// temp directory is created and cleaned up on exit.
	Workdir string
	// CaptureStderr controls whether child stderr lines are recorded into
	// the JSONL. Default: false (written to our stderr as plain lines).
	CaptureStderr bool
}

// Runner owns a recorder and a spawned child process. Use NewRunner to
// create one, then call Do with a callback that drives the protocol on the
// framer. Runner.Close always finalizes the recorder and kills any
// still-running child.
type Runner struct {
	rec    *Recorder
	cfg    RunConfig
	tmpDir string // non-empty if we allocated the workdir ourselves

	cmd    *exec.Cmd
	tree   processTree
	stdin  io.WriteCloser
	stdout io.ReadCloser
	framer *Framer

	stderrWG sync.WaitGroup

	// Frames received that aren't responses to our outstanding request:
	// notifications and agent-initiated requests. The queue is unbounded
	// because nothing drains it while Request waits for a response, and a
	// session/load replay emits one notification per history entry. With a
	// bounded queue the read loop blocks on the first frame past the limit
	// and never reaches the response that follows the replay, so the request
	// hangs until its deadline. Memory is bounded in practice by the size of
	// the session being replayed.
	oobMu     sync.Mutex
	oobBuf    []Frame
	oobClosed bool
	oobWake   chan struct{}
	// responses[id] is closed by the read loop when a response with that
	// id is recorded; the mu-protected map stores the frame itself.
	mu         sync.Mutex
	pending    map[int]chan Frame
	readLoopWG sync.WaitGroup
	readLoopEr error

	closeOnce    sync.Once
	shutdownOnce sync.Once
	shutdownCh   chan struct{}
	watchStopCh  chan struct{}
	cancelWG     sync.WaitGroup
}

// NewRunner spawns the configured child process, wires its stdio through a
// recorder into the given JSONL file, and starts background goroutines to
// read stdout (framed JSON-RPC) and stderr (plain text).
func NewRunner(ctx context.Context, jsonlPath string, cfg RunConfig) (*Runner, error) {
	if len(cfg.Command) == 0 {
		return nil, errors.New("command is empty")
	}

	rec, err := NewRecorder(jsonlPath)
	if err != nil {
		return nil, err
	}

	// Allocate a tmp workdir if the caller didn't provide one. Use a
	// pid-scoped prefix so we never collide with the host utility manager
	// or with other concurrent acpdbg runs.
	workdir := cfg.Workdir
	var tmpDir string
	if workdir == "" {
		wd, err := os.MkdirTemp("", fmt.Sprintf("kandev-acpdbg-%d-*", os.Getpid()))
		if err != nil {
			_ = rec.Close()
			return nil, fmt.Errorf("mktemp workdir: %w", err)
		}
		workdir = wd
		tmpDir = wd
	}
	// Keep the resolved directory in the runner configuration as well as on the
	// child command. ACP session/new and session/load use r.cfg.Workdir, so
	// leaving this empty would make wire-level probes differ from the child cwd.
	cfg.Workdir = workdir

	// Record start meta before spawning so the file always has something
	// useful even if exec fails.
	_ = rec.Meta("start", map[string]any{
		"agent":   cfg.AgentID,
		"command": cfg.Command,
		"workdir": workdir,
	})

	cmd, tree, stdin, stdout, stderr, err := startChild(cfg, workdir)
	if err != nil {
		_ = rec.Meta("close", map[string]any{"reason": err.Error()})
		_ = rec.Close()
		if tmpDir != "" {
			_ = os.RemoveAll(tmpDir)
		}
		return nil, err
	}

	r := &Runner{
		rec:         rec,
		cfg:         cfg,
		tmpDir:      tmpDir,
		cmd:         cmd,
		tree:        tree,
		stdin:       stdin,
		stdout:      stdout,
		framer:      NewFramer(stdin, stdout),
		oobWake:     make(chan struct{}, 1),
		pending:     map[int]chan Frame{},
		shutdownCh:  make(chan struct{}),
		watchStopCh: make(chan struct{}),
	}
	r.cancelWG.Add(1)
	go r.watchContext(ctx)

	// Stderr goroutine. Always drain it to avoid blocking the child; only
	// record to JSONL when CaptureStderr is set.
	r.stderrWG.Add(1)
	go r.drainStderr(stderr)

	// Stdout read loop: read frames, record each, route to the matching
	// pending request or drop into the oob channel.
	r.readLoopWG.Add(1)
	go r.readLoop()

	return r, nil
}

// startChild spawns the subprocess and returns its stdio pipes.
func startChild(cfg RunConfig, workdir string) (*exec.Cmd, processTree, io.WriteCloser, io.ReadCloser, io.ReadCloser, error) {
	//nolint:gosec // the command comes from the agent registry or the user's --exec flag; both are trusted inputs
	cmd := exec.Command(cfg.Command[0], cfg.Command[1:]...)
	cmd.Dir = workdir
	configureProcessTree(cmd)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, processTree{}, nil, nil, nil, fmt.Errorf("stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, processTree{}, nil, nil, nil, fmt.Errorf("stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, processTree{}, nil, nil, nil, fmt.Errorf("stderr pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, processTree{}, nil, nil, nil, fmt.Errorf("start %s: %w", cfg.Command[0], err)
	}
	tree, err := captureProcessTree(cmd)
	if err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return nil, processTree{}, nil, nil, nil, fmt.Errorf("contain %s: %w", cfg.Command[0], err)
	}
	return cmd, tree, stdin, stdout, stderr, nil
}

func (r *Runner) watchContext(ctx context.Context) {
	defer r.cancelWG.Done()
	select {
	case <-ctx.Done():
		r.signalShutdown()
		r.tree.kill(r.cmd)
	case <-r.watchStopCh:
	}
}

func (r *Runner) signalShutdown() {
	r.shutdownOnce.Do(func() { close(r.shutdownCh) })
}

// Path returns the JSONL file path the runner is writing to.
func (r *Runner) Path() string { return r.rec.Path() }

// Framer returns the framer so callers can build requests with fresh ids.
func (r *Runner) Framer() *Framer { return r.framer }

// Request writes a request frame, records it, and waits for the matching
// response frame (or a timeout). Notifications and agent-initiated requests
// received while we wait land on the OOB channel and are handled by the
// caller via HandleOOB.
func (r *Runner) Request(ctx context.Context, frame Frame) (Frame, error) {
	idNum, ok := frame["id"].(int)
	if !ok {
		return nil, fmt.Errorf("request frame has no integer id")
	}
	ch, err := r.registerPending(idNum)
	if err != nil {
		return nil, err
	}

	if err := r.rec.Sent(frame); err != nil {
		return nil, err
	}
	if err := r.framer.Write(frame); err != nil {
		return nil, fmt.Errorf("write request: %w", err)
	}

	select {
	case resp, ok := <-ch:
		if !ok {
			// Channel closed by readLoop on fatal error.
			r.mu.Lock()
			err := r.readLoopEr
			r.mu.Unlock()
			if err == nil {
				err = io.EOF
			}
			return nil, fmt.Errorf("read loop exited: %w", err)
		}
		return resp, nil
	case <-ctx.Done():
		r.mu.Lock()
		delete(r.pending, idNum)
		r.mu.Unlock()
		return nil, ctx.Err()
	}
}

// DrainOOBUntil consumes out-of-band frames (notifications, agent-initiated
// requests) until the given condition returns true or the context is
// cancelled. The handler is called for every oob frame it receives; if the
// frame is an agent-initiated request we automatically reply with
// method-not-found so the session doesn't hang.
func (r *Runner) DrainOOBUntil(ctx context.Context, handler func(Frame) bool) error {
	for {
		// NextOOB already auto-replies to agent-initiated requests so the
		// session does not hang waiting on us.
		frame, err := r.NextOOB(ctx)
		if err != nil {
			return err
		}
		if handler != nil && handler(frame) {
			return nil
		}
	}
}

// pushOOB appends a frame to the unbounded out-of-band queue. It never blocks,
// so the read loop always stays free to reach the response that follows a
// replay burst.
func (r *Runner) pushOOB(frame Frame) {
	r.oobMu.Lock()
	if r.oobClosed {
		r.oobMu.Unlock()
		return
	}
	r.oobBuf = append(r.oobBuf, frame)
	r.oobMu.Unlock()
	signal(r.oobWake)
}

// popOOB removes the oldest queued frame. ok is false when the queue is empty;
// closed reports that no further frames will arrive.
func (r *Runner) popOOB() (frame Frame, ok bool, closed bool) {
	r.oobMu.Lock()
	defer r.oobMu.Unlock()
	if len(r.oobBuf) == 0 {
		return nil, false, r.oobClosed
	}
	frame = r.oobBuf[0]
	// Release the popped slot. Re-slicing alone keeps the whole backing array
	// reachable, so a drained replay burst would stay pinned for the Runner's
	// lifetime; a channel receive used to zero the slot for us.
	r.oobBuf[0] = nil
	r.oobBuf = r.oobBuf[1:]
	if len(r.oobBuf) == 0 {
		r.oobBuf = nil
	}
	return frame, true, false
}

// closeOOB marks the queue drained-and-finished and wakes any waiter. Frames
// already queued stay readable, matching what a closed channel used to offer.
func (r *Runner) closeOOB() {
	r.oobMu.Lock()
	r.oobClosed = true
	r.oobMu.Unlock()
	signal(r.oobWake)
}

// oobFinished reports that the child is gone and every queued frame has been
// consumed, so a caller waiting for more will wait forever.
func (r *Runner) oobFinished() bool {
	r.oobMu.Lock()
	defer r.oobMu.Unlock()
	return r.oobClosed && len(r.oobBuf) == 0
}

// wakeNextOOBWaiter passes the wake token on when work remains. oobWake holds a
// single coalesced token, so a consumer that takes it has to re-arm or a second
// waiter sleeps with frames still queued (or misses the close entirely).
func (r *Runner) wakeNextOOBWaiter() {
	r.oobMu.Lock()
	pending := len(r.oobBuf) > 0 || r.oobClosed
	r.oobMu.Unlock()
	if pending {
		signal(r.oobWake)
	}
}

// signal delivers a non-blocking wake token, dropping it when one is pending.
func signal(ch chan struct{}) {
	select {
	case ch <- struct{}{}:
	default:
	}
}

// NextOOB returns the next out-of-band frame, blocking until one is
// available or the context is cancelled.
func (r *Runner) NextOOB(ctx context.Context) (Frame, error) {
	for {
		frame, ok, closed := r.popOOB()
		if ok {
			r.wakeNextOOBWaiter()
			if frame.Method() != "" && frame.ID() != nil {
				reply := NewMethodNotFound(frame.ID(), frame.Method())
				_ = r.rec.Sent(reply)
				_ = r.framer.Write(reply)
			}
			return frame, nil
		}
		if closed {
			r.wakeNextOOBWaiter()
			return nil, io.EOF
		}
		select {
		case <-r.oobWake:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}

// Close closes stdin (signalling EOF to the child), waits briefly for
// graceful exit, kills the child if needed, drains goroutines, and closes
// the recorder. Idempotent.
func (r *Runner) Close(reason string) {
	r.closeOnce.Do(func() {
		r.signalShutdown()
		_ = r.stdin.Close()
		done := make(chan error, 1)
		go func() { done <- r.cmd.Wait() }()

		var exitCode int
		var waitErr error
		select {
		case err := <-done:
			waitErr = err
		case <-time.After(5 * time.Second):
			// Kill the whole tree, not just the direct child. Agents spawned
			// through `npx` leave grandchildren holding the inherited stdout
			// handle; with those alive the read loop never sees EOF and the
			// readLoopWG.Wait below blocks forever, so --timeout never lands.
			r.tree.kill(r.cmd)
			waitErr = <-done
			if reason == "" {
				reason = "killed after timeout"
			}
		}
		// The child may have exited before the grace-period branch. Closing
		// the owned tree in every terminal path also handles shims that leave
		// descendants behind after the leader exits.
		r.tree.kill(r.cmd)
		if exitErr, ok := waitErr.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else if waitErr != nil {
			exitCode = -1
		}

		r.readLoopWG.Wait()
		r.stderrWG.Wait()
		r.closeOOB()

		meta := map[string]any{"exit_code": exitCode}
		if reason != "" {
			meta["reason"] = reason
		}
		if waitErr != nil {
			meta["wait_err"] = waitErr.Error()
		}
		if r.readLoopEr != nil && !errors.Is(r.readLoopEr, io.EOF) {
			meta["read_err"] = r.readLoopEr.Error()
		}
		_ = r.rec.Meta("close", meta)
		_ = r.rec.Close()
		close(r.watchStopCh)
		r.cancelWG.Wait()
		if r.tmpDir != "" {
			_ = os.RemoveAll(r.tmpDir)
		}
	})
}

// routeResponse delivers the frame to a pending Request waiter if one
// exists. Returns true if consumed.
func (r *Runner) routeResponse(frame Frame) bool {
	idf, ok := frame["id"].(float64)
	if !ok {
		return false
	}
	id := int(idf)
	r.mu.Lock()
	ch, found := r.pending[id]
	if found {
		delete(r.pending, id)
	}
	r.mu.Unlock()
	if !found {
		return false
	}
	ch <- frame
	return true
}

func (r *Runner) readLoop() {
	defer r.readLoopWG.Done()
	for {
		frame, err := r.framer.Read()
		if err != nil {
			// Fail any outstanding request waiters so Request() returns
			// rather than blocking forever when the child dies.
			r.failPending(err)
			return
		}
		_ = r.rec.Received(frame)

		// Responses route to any pending Request waiter.
		if frame.IsResponse() && r.routeResponse(frame) {
			continue
		}

		// Anything else (notifications, agent-initiated requests,
		// orphaned responses) goes out of band. The queue is unbounded and
		// never drops, so session/update chunks and agent-initiated requests
		// survive a replay burst without stalling this loop.
		//
		// Stop once shutdown starts, though: nobody will read what we queue
		// from here on, and a wedged child that floods stdout through the
		// grace period would otherwise make us allocate everything it writes.
		select {
		case <-r.shutdownCh:
			r.failPending(errShutdown)
			return
		default:
		}
		r.pushOOB(frame)
	}
}

// errShutdown is what outstanding requests see when the read loop stops because
// the runner is shutting down rather than because the child died.
var errShutdown = errors.New("runner shutting down")

// registerPending claims the response channel for a request id, refusing once
// the read loop is terminal. failPending only closes the waiters that exist
// when it runs, so a registration racing past it would never be woken: writing
// the request can still succeed against a child holding stdin open, and the
// caller would then block until its deadline. The check shares r.mu with
// failPending so the two cannot interleave.
func (r *Runner) registerPending(id int) (chan Frame, error) {
	ch := make(chan Frame, 1)
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.readLoopEr != nil {
		return nil, fmt.Errorf("read loop exited: %w", r.readLoopEr)
	}
	r.pending[id] = ch
	return ch, nil
}

// failPending closes every outstanding request waiter so callers return instead
// of blocking on a child nobody is reading any more.
func (r *Runner) failPending(err error) {
	r.mu.Lock()
	if r.readLoopEr == nil {
		r.readLoopEr = err
	}
	for id, ch := range r.pending {
		close(ch)
		delete(r.pending, id)
	}
	r.mu.Unlock()
}

func (r *Runner) drainStderr(rc io.ReadCloser) {
	defer r.stderrWG.Done()
	sc := bufio.NewScanner(rc)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Text()
		if r.cfg.CaptureStderr {
			_ = r.rec.Stderr(line)
		} else {
			// Print to our stderr with a prefix so the user can tell it
			// came from the child.
			fmt.Fprintf(os.Stderr, "[%s stderr] %s\n", r.cfg.AgentID, line)
		}
	}
}

// SanitizeAgentID produces a filesystem-safe token for use in a JSONL
// filename. Non-[A-Za-z0-9._-] characters become underscores.
func SanitizeAgentID(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_', r == '.':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	out := b.String()
	if out == "" {
		return "unknown"
	}
	return out
}

// SuggestJSONLPath returns a default JSONL filename inside outDir based on
// the agent id, operation name, and a timestamp.
func SuggestJSONLPath(outDir, agentID, op string) string {
	ts := time.Now().UTC().Format("20060102-150405")
	name := fmt.Sprintf("%s-%s-%s.jsonl", SanitizeAgentID(agentID), op, ts)
	return filepath.Join(outDir, name)
}
