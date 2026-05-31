package process

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// UserShellOptions contains optional parameters for starting a user shell.
type UserShellOptions struct {
	Label          string // Display name (e.g., "Terminal" or script name)
	InitialCommand string // Command to run after shell starts
	Closable       *bool  // Whether the terminal can be closed (nil = auto-determine)
}

// CreateUserShellResult contains the result of creating a new user shell.
type CreateUserShellResult struct {
	TerminalID string `json:"terminal_id"`
	Label      string `json:"label"`
	Closable   bool   `json:"closable"`
}

// UserShellInfo contains information about a running user shell.
type UserShellInfo struct {
	TerminalID     string    `json:"terminal_id"`
	ProcessID      string    `json:"process_id"`
	Running        bool      `json:"running"`
	Label          string    `json:"label"`           // Display name (e.g., "Terminal" or "Terminal 2")
	Closable       bool      `json:"closable"`        // Whether the terminal can be closed
	InitialCommand string    `json:"initial_command"` // Command that was run (empty for plain shells)
	CreatedAt      time.Time `json:"created_at"`      // When the shell was created (for stable ordering)
}

// CreateUserShell creates a new user shell terminal with auto-assigned ID and label.
// The first shell for a scope is labeled "Terminal" and is not closable.
// Subsequent shells are labeled "Terminal 2", "Terminal 3", etc. and are closable.
// The entry is registered atomically to prevent races with ListUserShells.
//
// `scopeID` groups shells. Callers should pass `taskEnvironmentID` so sessions
// in the same task share one shell list.
func (r *InteractiveRunner) CreateUserShell(scopeID string) CreateUserShellResult {
	r.userShellsMu.Lock()
	defer r.userShellsMu.Unlock()

	prefix := scopeID + ":"
	shellCount := 0
	for key, entry := range r.userShells {
		if strings.HasPrefix(key, prefix) {
			terminalID := key[len(prefix):]
			if strings.HasPrefix(terminalID, "shell-") && entry.InitialCommand == "" {
				shellCount++
			}
		}
	}

	terminalID := "shell-" + uuid.New().String()

	var label string
	var closable bool
	if shellCount == 0 {
		label = "Terminal"
		closable = false
	} else {
		label = fmt.Sprintf("Terminal %d", shellCount+1)
		closable = true
	}

	r.userShells[prefix+terminalID] = &userShellEntry{
		ProcessID:      "",
		Label:          label,
		InitialCommand: "",
		Closable:       closable,
		CreatedAt:      time.Now().UTC(),
	}

	return CreateUserShellResult{
		TerminalID: terminalID,
		Label:      label,
		Closable:   closable,
	}
}

// RegisterScriptShell registers a script terminal entry so ListUserShells returns it.
// The actual process is not started until the WebSocket connects (StartUserShell handles that).
func (r *InteractiveRunner) RegisterScriptShell(scopeID, terminalID, label, initialCommand string) {
	key := scopeID + ":" + terminalID

	r.userShellsMu.Lock()
	defer r.userShellsMu.Unlock()

	r.userShells[key] = &userShellEntry{
		ProcessID:      "", // No process yet - will be started when WebSocket connects
		Label:          label,
		InitialCommand: initialCommand,
		Closable:       true, // Script terminals are always closable
		CreatedAt:      time.Now().UTC(),
	}
}

// LookupShellInitialCommand returns the InitialCommand stored for a pre-registered
// shell entry, or "" when no entry exists. Used by remote-shell handlers to recover
// the script command across the WS-handshake boundary, since the per-terminal WS URL
// only carries terminalId — not script_id or command.
func (r *InteractiveRunner) LookupShellInitialCommand(scopeID, terminalID string) string {
	key := scopeID + ":" + terminalID
	r.userShellsMu.RLock()
	defer r.userShellsMu.RUnlock()
	if entry, ok := r.userShells[key]; ok {
		return entry.InitialCommand
	}
	return ""
}

// StartUserShell starts or returns an existing user shell for a terminal tab.
// Each terminal tab gets its own independent shell process. The shell is keyed
// in the userShells map by `scopeID` (taskEnvironmentID when known) so that
// sessions sharing an env share the same shell. The underlying process is
// still started under `processSessionID` for InteractiveStartRequest purposes
// (ring-buffer accounting, etc.) but `IsUserShell: true` excludes it from
// session-level routing.
func (r *InteractiveRunner) StartUserShell(ctx context.Context, scopeID, processSessionID, terminalID, workingDir, preferredShell string, opts *UserShellOptions) (*InteractiveProcessInfo, error) {
	key := scopeID + ":" + terminalID

	if opts == nil {
		opts = &UserShellOptions{}
	}
	if opts.Label == "" {
		opts.Label = "Terminal"
	}

	var existingEntry *userShellEntry
	r.userShellsMu.RLock()
	entry, exists := r.userShells[key]
	if exists {
		if entry.ProcessID != "" {
			if info, ok := r.Get(entry.ProcessID, false); ok {
				r.userShellsMu.RUnlock()
				r.logger.Info("reusing user shell",
					zap.String("scope_id", scopeID),
					zap.String("session_id", processSessionID),
					zap.String("terminal_id", terminalID),
					zap.String("process_id", info.ID),
					zap.Int("os_pid", info.OSPID),
					zap.Strings("command", info.Command),
					zap.String("working_dir", info.WorkingDir),
					zap.String("label", entry.Label),
					zap.String("initial_command", entry.InitialCommand))
				return info, nil
			}
		}
		existingEntry = entry
	}
	r.userShellsMu.RUnlock()

	initialCommand := opts.InitialCommand
	if initialCommand == "" && existingEntry != nil {
		initialCommand = existingEntry.InitialCommand
	}

	label := opts.Label
	if existingEntry != nil && existingEntry.Label != "" {
		label = existingEntry.Label
	}

	req := InteractiveStartRequest{
		SessionID:            processSessionID,
		Command:              defaultShellCommand(preferredShell),
		WorkingDir:           workingDir,
		ScopeID:              scopeID,
		TerminalID:           terminalID,
		Label:                label,
		InitialCommand:       initialCommand,
		DisableTurnDetection: true,
		IsUserShell:          true,
	}

	info, err := r.Start(ctx, req)
	if err != nil {
		return nil, err
	}

	r.userShellsMu.Lock()
	if existingEntry != nil {
		existingEntry.ProcessID = info.ID
		r.userShells[key] = existingEntry
	} else {
		closable := true
		if opts.Closable != nil {
			closable = *opts.Closable
		}
		r.userShells[key] = &userShellEntry{
			ProcessID:      info.ID,
			Label:          opts.Label,
			InitialCommand: opts.InitialCommand,
			Closable:       closable,
			CreatedAt:      time.Now().UTC(),
		}
	}
	r.userShellsMu.Unlock()

	r.logger.Info("started user shell",
		zap.String("scope_id", scopeID),
		zap.String("session_id", processSessionID),
		zap.String("terminal_id", terminalID),
		zap.String("process_id", info.ID),
		zap.Int("os_pid", info.OSPID),
		zap.String("shell", req.Command[0]),
		zap.String("working_dir", workingDir),
		zap.String("label", label),
		zap.String("initial_command", initialCommand),
		zap.Bool("deferred_start", info.OSPID == 0))

	return info, nil
}

// ListUserShells returns every user shell registered for the scope, sorted
// by creation time. The list includes script terminals, the hardcoded
// bottom-panel entry (if it exists), and any orphan ordinary shells the
// agent has registered.
//
// Auto-creating the first "Terminal" entry is no longer this function's
// job — the terminal service (internal/terminal/service) owns that policy
// now and inserts a DB row alongside the agentctl-side entry. This keeps
// "what's in the panel" derivable from a single source of truth.
func (r *InteractiveRunner) ListUserShells(scopeID string) []UserShellInfo {
	r.userShellsMu.RLock()
	defer r.userShellsMu.RUnlock()

	prefix := scopeID + ":"
	var shells []UserShellInfo
	for key, entry := range r.userShells {
		if !strings.HasPrefix(key, prefix) {
			continue
		}
		terminalID := key[len(prefix):]
		_, running := r.Get(entry.ProcessID, false)
		shells = append(shells, UserShellInfo{
			TerminalID:     terminalID,
			ProcessID:      entry.ProcessID,
			Running:        running,
			Label:          entry.Label,
			Closable:       entry.Closable,
			InitialCommand: entry.InitialCommand,
			CreatedAt:      entry.CreatedAt,
		})
	}

	sort.Slice(shells, func(i, j int) bool {
		return shells[i].CreatedAt.Before(shells[j].CreatedAt)
	})
	return shells
}

// StopUserShell stops a user shell for a terminal tab.
func (r *InteractiveRunner) StopUserShell(ctx context.Context, scopeID, terminalID string) error {
	key := scopeID + ":" + terminalID

	r.userShellsMu.Lock()
	entry, exists := r.userShells[key]
	if exists {
		delete(r.userShells, key)
	}
	r.userShellsMu.Unlock()

	if !exists {
		return nil
	}

	r.logger.Info("stopping user shell",
		zap.String("scope_id", scopeID),
		zap.String("terminal_id", terminalID),
		zap.String("process_id", entry.ProcessID),
		zap.Int("os_pid", r.osPIDForProcess(entry.ProcessID)),
		zap.String("label", entry.Label),
		zap.String("initial_command", entry.InitialCommand))

	return r.Stop(ctx, entry.ProcessID)
}

// StopUserShellsForScope stops and unregisters every user shell in a scope.
// This is used by task archive/delete cleanup where bottom-panel/script shells
// may not have DB rows but still need to be torn down with the task environment.
func (r *InteractiveRunner) StopUserShellsForScope(ctx context.Context, scopeID string) (int, error) {
	if scopeID == "" {
		return 0, nil
	}

	prefix := scopeID + ":"
	type stopTarget struct {
		terminalID string
		entry      *userShellEntry
	}
	targets := []stopTarget{}

	r.userShellsMu.Lock()
	for key, entry := range r.userShells {
		if !strings.HasPrefix(key, prefix) {
			continue
		}
		delete(r.userShells, key)
		targets = append(targets, stopTarget{
			terminalID: key[len(prefix):],
			entry:      entry,
		})
	}
	r.userShellsMu.Unlock()

	var lastErr error
	stoppedCount := 0
	for _, target := range targets {
		processID := ""
		label := ""
		initialCommand := ""
		if target.entry != nil {
			processID = target.entry.ProcessID
			label = target.entry.Label
			initialCommand = target.entry.InitialCommand
		}
		r.logger.Info("stopping user shell for scope cleanup",
			zap.String("scope_id", scopeID),
			zap.String("terminal_id", target.terminalID),
			zap.String("process_id", processID),
			zap.Int("os_pid", r.osPIDForProcess(processID)),
			zap.String("label", label),
			zap.String("initial_command", initialCommand))
		if processID == "" {
			continue
		}
		if err := r.Stop(ctx, processID); err != nil {
			r.logger.Warn("failed to stop user shell for scope cleanup",
				zap.String("scope_id", scopeID),
				zap.String("terminal_id", target.terminalID),
				zap.String("process_id", processID),
				zap.Error(err))
			lastErr = err
			continue
		}
		stoppedCount++
	}
	return stoppedCount, lastErr
}

// IsUserShellAlive reports whether the PTY behind (scopeID, terminalID) is
// currently running. Returns false when the entry doesn't exist, the entry
// has no process id (lazy-start hasn't happened yet), or the process has
// since exited.
//
// ProcessID is read under the read lock so the subsequent r.Get call
// doesn't race with a concurrent shell-start path that mutates the entry
// pointer in place.
func (r *InteractiveRunner) IsUserShellAlive(scopeID, terminalID string) bool {
	key := scopeID + ":" + terminalID

	r.userShellsMu.RLock()
	var processID string
	if entry, exists := r.userShells[key]; exists {
		processID = entry.ProcessID
	}
	r.userShellsMu.RUnlock()

	if processID == "" {
		return false
	}
	_, running := r.Get(processID, false)
	return running
}

// ResizeUserShell resizes the PTY for a user shell.
func (r *InteractiveRunner) ResizeUserShell(scopeID, terminalID string, cols, rows uint16) error {
	key := scopeID + ":" + terminalID

	r.userShellsMu.RLock()
	entry, exists := r.userShells[key]
	r.userShellsMu.RUnlock()

	if !exists {
		return fmt.Errorf("no user shell found for scope %s terminal %s", scopeID, terminalID)
	}

	proc, ok := r.get(entry.ProcessID)
	if !ok {
		return fmt.Errorf("process not found: %s", entry.ProcessID)
	}

	return r.lazyStartAndResize(proc, cols, rows,
		zap.String("scope_id", scopeID),
		zap.String("terminal_id", terminalID),
	)
}

// GetUserShellPtyWriter returns the PTY writer for a user shell.
func (r *InteractiveRunner) GetUserShellPtyWriter(scopeID, terminalID string) (io.Writer, string, error) {
	key := scopeID + ":" + terminalID

	r.userShellsMu.RLock()
	entry, exists := r.userShells[key]
	r.userShellsMu.RUnlock()

	if !exists {
		return nil, "", fmt.Errorf("no user shell found for scope %s terminal %s", scopeID, terminalID)
	}

	writer, err := r.GetPtyWriter(entry.ProcessID)
	if err != nil {
		return nil, entry.ProcessID, err
	}

	return writer, entry.ProcessID, nil
}

// ClearUserShellDirectOutput clears the direct output for a user shell.
func (r *InteractiveRunner) ClearUserShellDirectOutput(scopeID, terminalID string) {
	key := scopeID + ":" + terminalID

	r.userShellsMu.RLock()
	entry, exists := r.userShells[key]
	r.userShellsMu.RUnlock()

	if !exists {
		return
	}

	proc, ok := r.get(entry.ProcessID)
	if !ok {
		return
	}

	proc.directOutputMu.Lock()
	proc.directOutput = nil
	proc.hasActiveWebSocket = false
	proc.directOutputMu.Unlock()

	r.logger.Info("direct output cleared for user shell",
		zap.String("scope_id", scopeID),
		zap.String("terminal_id", terminalID),
		zap.String("process_id", entry.ProcessID),
		zap.Int("os_pid", proc.osPID()))
}

func (r *InteractiveRunner) osPIDForProcess(processID string) int {
	if processID == "" {
		return 0
	}
	pid, ok := r.GetOSPID(processID)
	if !ok {
		return 0
	}
	return pid
}
