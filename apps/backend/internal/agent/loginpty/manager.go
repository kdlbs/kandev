// Package loginpty runs an agent's interactive login command under a PTY on
// the kandev host. The UI surfaces this as a terminal so users can complete
// browser-based or token-prompt auth flows without shelling into the
// container.
//
// Sessions are keyed by agent ID — at most one login PTY per agent runs at a
// time, since concurrent logins for the same agent would race on the same
// HOME dotfile. The session terminates when the command exits, on idle
// timeout, or on explicit Stop.
package loginpty

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/common/logger"
	"go.uber.org/zap"
)

const (
	// IdleTimeout terminates a session that hasn't produced output for this long.
	// Picked to fit typical browser-OAuth flows (open browser → grant → callback).
	IdleTimeout = 10 * time.Minute

	// HardTimeout is the wall-clock upper bound; covers stuck sessions.
	HardTimeout = 30 * time.Minute

	// outputBufferSize is the rolling buffer of recent PTY output kept for
	// late subscribers. Login flows are usually short so 16KB is plenty.
	outputBufferSize = 16 * 1024
)

// Errors surfaced to callers.
var (
	ErrSessionAlreadyRunning = errors.New("login session already running for this agent")
	ErrSessionNotFound       = errors.New("login session not found")
	ErrSessionNotRunning     = errors.New("login session is not running")
)

// Manager owns active login sessions and dispatches lifecycle callbacks.
type Manager struct {
	mu       sync.Mutex
	sessions map[string]*Session // key: agentID
	byID     map[string]*Session // key: sessionID
	log      *logger.Logger

	// onExit is invoked from a goroutine when a session terminates (with the
	// agent ID and exit code). Used by callers to invalidate discovery cache
	// and broadcast updated availability. Optional.
	onExit func(agentID string, exitCode int, exitErr error)
}

// NewManager constructs a Manager. onExit may be nil.
func NewManager(log *logger.Logger, onExit func(agentID string, exitCode int, exitErr error)) *Manager {
	return &Manager{
		sessions: map[string]*Session{},
		byID:     map[string]*Session{},
		log:      log.WithFields(zap.String("component", "login-pty")),
		onExit:   onExit,
	}
}

// Start spawns a PTY-backed process for the agent. Returns the session
// snapshot. Fails with ErrSessionAlreadyRunning if a session for this agent
// is already active.
func (m *Manager) Start(agentID string, cmd []string, cols, rows uint16) (*Session, error) {
	if len(cmd) == 0 {
		return nil, errors.New("empty command")
	}
	m.mu.Lock()
	if existing, ok := m.sessions[agentID]; ok {
		m.mu.Unlock()
		_ = existing.Status() // touch
		return existing, ErrSessionAlreadyRunning
	}

	if cols == 0 {
		cols = 80
	}
	if rows == 0 {
		rows = 24
	}

	sess := &Session{
		ID:          uuid.NewString(),
		AgentID:     agentID,
		Cmd:         append([]string(nil), cmd...),
		subscribers: map[chan<- []byte]struct{}{},
		log:         m.log.WithFields(zap.String("agent", agentID)),
		startedAt:   time.Now(),
	}

	if err := sess.start(cmd, cols, rows); err != nil {
		m.mu.Unlock()
		return nil, err
	}

	m.sessions[agentID] = sess
	m.byID[sess.ID] = sess
	m.mu.Unlock()

	go m.supervise(sess)
	return sess, nil
}

// Get returns a session by agent ID (or nil).
func (m *Manager) Get(agentID string) *Session {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.sessions[agentID]
}

// GetByID returns a session by session ID (or nil).
func (m *Manager) GetByID(sessionID string) *Session {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.byID[sessionID]
}

// Stop terminates the agent's session if one is running.
func (m *Manager) Stop(agentID string) error {
	m.mu.Lock()
	sess, ok := m.sessions[agentID]
	m.mu.Unlock()
	if !ok {
		return ErrSessionNotFound
	}
	sess.stop()
	return nil
}

// supervise watches the session's lifetime: waits on the process, enforces
// timeouts, then deletes the session from the manager and notifies onExit.
func (m *Manager) supervise(sess *Session) {
	ctx, cancel := context.WithTimeout(context.Background(), HardTimeout)
	defer cancel()

	exited := make(chan exitInfo, 1)
	go func() {
		err := sess.waitForCommand()
		exited <- exitInfo{
			code: exitCodeFromError(err),
			err:  err,
		}
	}()

	idle := time.NewTimer(IdleTimeout)
	defer idle.Stop()

	var info exitInfo
loop:
	for {
		select {
		case info = <-exited:
			// Natural child exit. On Windows ConPTY, the underlying handle does
			// NOT return EOF when the child process terminates, so the readLoop
			// would block on Read until something closes the PTY. Closing here
			// unblocks it so the readDone wait below resolves promptly. On Unix
			// the master already saw EOF from the slave close — this is just an
			// extra (idempotent) Close that the windowsPTY sync.Once also guards.
			sess.stop()
			break loop
		case <-idle.C:
			sess.log.Info("login session idle timeout — terminating")
			sess.stop()
		case <-ctx.Done():
			sess.log.Info("login session hard timeout — terminating")
			sess.stop()
		case <-sess.activityCh:
			// Reset idle timer on output activity.
			if !idle.Stop() {
				select {
				case <-idle.C:
				default:
				}
			}
			idle.Reset(IdleTimeout)
		}
	}

	// Wait for readLoop to drain remaining PTY output before flipping running=false
	// or closing subscribers. Otherwise late Subscribe callers (or BufferedOutput
	// readers) could observe an empty session even though the process produced
	// output. The pty is closed by stop() in timeout paths and by the child
	// exit in the natural path; either way readLoop returns shortly after.
	// Cap the wait so a stuck PTY can't deadlock cleanup.
	select {
	case <-sess.readDone:
	case <-time.After(2 * time.Second):
		sess.log.Warn("readLoop did not exit in time during supervise cleanup")
	}

	m.mu.Lock()
	delete(m.sessions, sess.AgentID)
	delete(m.byID, sess.ID)
	m.mu.Unlock()

	sess.mu.Lock()
	now := time.Now()
	sess.finishedAt = &now
	sess.exitCode = &info.code
	sess.running = false
	durationMs := now.Sub(sess.startedAt).Milliseconds()
	sess.mu.Unlock()

	// Log the exit so we can tell apart "agent exited 0 silently" (e.g.
	// already-authed) from "agent crashed". Suppress the noisy EOF error,
	// which is just normal PTY closure.
	exitErr := ""
	if info.err != nil && info.err != io.EOF {
		exitErr = info.err.Error()
	}
	sess.log.Info("login session ended",
		zap.String("session_id", sess.ID),
		zap.Int("exit_code", info.code),
		zap.String("err", exitErr),
		zap.Int64("duration_ms", durationMs),
	)

	sess.broadcastClose()

	if m.onExit != nil {
		m.onExit(sess.AgentID, info.code, info.err)
	}
}

type exitInfo struct {
	code int
	err  error
}

func exitCodeFromError(err error) int {
	if err == nil {
		return 0
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode()
	}
	return -1
}

// Session is a single login PTY process plus its subscriber set.
// Exported fields are stable; everything else is internal.
type Session struct {
	ID      string
	AgentID string
	Cmd     []string

	mu         sync.Mutex
	cmd        *exec.Cmd
	pty        ptyHandle
	running    bool
	startedAt  time.Time
	finishedAt *time.Time
	exitCode   *int

	subMu       sync.Mutex
	subscribers map[chan<- []byte]struct{}
	subClosed   bool // set under subMu by broadcastClose; once true, Subscribe closes the channel immediately

	bufMu  sync.Mutex
	buffer []byte

	activityCh chan struct{} // notifications to supervise loop on output
	readDone   chan struct{} // closed when readLoop exits (all PTY output drained)

	log *logger.Logger
}

func (s *Session) start(cmd []string, cols, rows uint16) error {
	s.activityCh = make(chan struct{}, 1)
	s.readDone = make(chan struct{})
	c := exec.Command(cmd[0], cmd[1:]...) // #nosec G204 — command is hard-coded per agent
	c.Env = buildLoginEnv()

	handle, err := startPTYWithSize(c, cols, rows)
	if err != nil {
		return fmt.Errorf("start pty: %w", err)
	}

	s.cmd = c
	s.pty = handle
	s.running = true
	s.log.Info("login session started",
		zap.String("session_id", s.ID),
		zap.Strings("cmd", cmd),
		zap.Int("pid", c.Process.Pid),
	)
	go s.readLoop()
	return nil
}

func (s *Session) readLoop() {
	defer close(s.readDone)
	buf := make([]byte, 4096)
	for {
		n, err := s.pty.Read(buf)
		if n > 0 {
			data := make([]byte, n)
			copy(data, buf[:n])
			s.appendBuffer(data)
			s.broadcast(data)
			select {
			case s.activityCh <- struct{}{}:
			default:
			}
		}
		if err != nil {
			if err != io.EOF {
				s.log.Debug("pty read ended", zap.Error(err))
			}
			return
		}
	}
}

func (s *Session) appendBuffer(data []byte) {
	s.bufMu.Lock()
	defer s.bufMu.Unlock()
	s.buffer = append(s.buffer, data...)
	if len(s.buffer) > outputBufferSize {
		s.buffer = s.buffer[len(s.buffer)-outputBufferSize:]
	}
}

// BufferedOutput returns a copy of the rolling output buffer for catching up
// new WS subscribers.
func (s *Session) BufferedOutput() []byte {
	s.bufMu.Lock()
	defer s.bufMu.Unlock()
	if len(s.buffer) == 0 {
		return nil
	}
	out := make([]byte, len(s.buffer))
	copy(out, s.buffer)
	return out
}

// Subscribe registers a channel for PTY output. The channel is non-blocking:
// if it fills up, chunks are dropped (the subscriber falls behind).
//
// If the session has already ended (broadcastClose ran), the channel is
// closed immediately so the caller's reader returns rather than blocking
// forever on a channel that nothing will ever send to or close.
//
// Both the close-check and the map insertion happen under subMu so a
// concurrent broadcastClose cannot race in between (which would otherwise
// leave the late subscriber registered in a fresh empty map with nothing
// to ever close it).
func (s *Session) Subscribe(ch chan<- []byte) {
	s.subMu.Lock()
	defer s.subMu.Unlock()
	if s.subClosed {
		close(ch)
		return
	}
	s.subscribers[ch] = struct{}{}
}

// Unsubscribe removes a channel from the subscriber set.
func (s *Session) Unsubscribe(ch chan<- []byte) {
	s.subMu.Lock()
	defer s.subMu.Unlock()
	delete(s.subscribers, ch)
}

func (s *Session) broadcast(data []byte) {
	s.subMu.Lock()
	defer s.subMu.Unlock()
	for ch := range s.subscribers {
		select {
		case ch <- data:
		default:
		}
	}
}

// broadcastClose closes any remaining subscriber channels — used to wake WS
// loops on session exit. subClosed is set under subMu so any concurrent
// Subscribe taking subMu after this either observes subClosed and closes
// its own channel, or completed before us and is already in the snapshot.
func (s *Session) broadcastClose() {
	s.subMu.Lock()
	subs := s.subscribers
	s.subscribers = map[chan<- []byte]struct{}{}
	s.subClosed = true
	s.subMu.Unlock()
	for ch := range subs {
		close(ch)
	}
}

// Write sends user input to the underlying process via the PTY.
func (s *Session) Write(p []byte) (int, error) {
	s.mu.Lock()
	pf := s.pty
	running := s.running
	s.mu.Unlock()
	if !running || pf == nil {
		return 0, ErrSessionNotRunning
	}
	return pf.Write(p)
}

// Resize updates the PTY window size.
func (s *Session) Resize(cols, rows uint16) error {
	s.mu.Lock()
	pf := s.pty
	running := s.running
	s.mu.Unlock()
	if !running || pf == nil {
		return ErrSessionNotRunning
	}
	return pf.Resize(cols, rows)
}

// Status snapshot for HTTP responses.
type Status struct {
	ID         string     `json:"session_id"`
	AgentID    string     `json:"agent_id"`
	Cmd        []string   `json:"cmd"`
	Running    bool       `json:"running"`
	StartedAt  time.Time  `json:"started_at"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
	ExitCode   *int       `json:"exit_code,omitempty"`
}

// Status returns a value snapshot of the session.
func (s *Session) Status() Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := Status{
		ID:        s.ID,
		AgentID:   s.AgentID,
		Cmd:       append([]string(nil), s.Cmd...),
		Running:   s.running,
		StartedAt: s.startedAt,
	}
	if s.finishedAt != nil {
		f := *s.finishedAt
		st.FinishedAt = &f
	}
	if s.exitCode != nil {
		e := *s.exitCode
		st.ExitCode = &e
	}
	return st
}

// stop tears the PTY down idempotently. The `!s.running` guard is flipped
// *under the same lock* that we use to read pty/cmd, so concurrent callers
// (timeout case in supervise + external Manager.Stop, etc.) don't double-
// close the underlying handle on the way out. The Windows ConPTY backing
// also has its own sync.Once on Close because its upstream library has no
// internal synchronization and a double-close triggers STATUS_HEAP_CORRUPTION.
func (s *Session) stop() {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return
	}
	s.running = false
	pf := s.pty
	cmd := s.cmd
	s.mu.Unlock()

	if pf != nil {
		_ = pf.Close()
	}
	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
}

func (s *Session) waitForCommand() error {
	if s.cmd == nil {
		return nil
	}
	return s.cmd.Wait()
}

// buildLoginEnv composes the environment for the login subprocess. The user's
// HOME (where agents write credentials) is inherited from the kandev process.
func buildLoginEnv() []string {
	env := os.Environ()
	env = append(env, "TERM=xterm-256color", "LANG=C.UTF-8", "LC_ALL=C.UTF-8")
	return env
}
