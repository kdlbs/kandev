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
// The first shell for a session is labeled "Terminal" and is not closable.
// Subsequent shells are labeled "Terminal 2", "Terminal 3", etc. and are closable.
// The entry is registered atomically to prevent races with ListUserShells.
func (r *InteractiveRunner) CreateUserShell(sessionID string) CreateUserShellResult {
	r.userShellsMu.Lock()
	defer r.userShellsMu.Unlock()

	// Count existing plain shell terminals for this session
	prefix := sessionID + ":"
	shellCount := 0
	for key, entry := range r.userShells {
		if strings.HasPrefix(key, prefix) {
			terminalID := key[len(prefix):]
			// Only count plain shells (not script terminals)
			if strings.HasPrefix(terminalID, "shell-") && entry.InitialCommand == "" {
				shellCount++
			}
		}
	}

	// Generate terminal ID and label
	terminalID := "shell-" + uuid.New().String()

	var label string
	var closable bool
	if shellCount == 0 {
		label = "Terminal"
		closable = false // First terminal is not closable
	} else {
		label = fmt.Sprintf("Terminal %d", shellCount+1)
		closable = true
	}

	// Register the entry so ListUserShells includes it immediately
	r.userShells[prefix+terminalID] = &userShellEntry{
		ProcessID:      "", // No process yet - will be started when WebSocket connects
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
func (r *InteractiveRunner) RegisterScriptShell(sessionID, terminalID, label, initialCommand string) {
	key := sessionID + ":" + terminalID

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

// StartUserShell starts or returns an existing user shell for a terminal tab.
// Each terminal tab gets its own independent shell process.
// If opts.InitialCommand is provided, it will be written to stdin after the shell starts.
func (r *InteractiveRunner) StartUserShell(ctx context.Context, sessionID, terminalID, workingDir, preferredShell string, opts *UserShellOptions) (*InteractiveProcessInfo, error) {
	key := sessionID + ":" + terminalID

	// Normalize options
	if opts == nil {
		opts = &UserShellOptions{}
	}
	if opts.Label == "" {
		opts.Label = "Terminal"
	}

	// Check if shell entry already exists (auto-created by ListUserShells or RegisterScriptShell)
	var existingEntry *userShellEntry
	r.userShellsMu.RLock()
	entry, exists := r.userShells[key]
	if exists {
		// If entry has a process, check if it's still alive
		if entry.ProcessID != "" {
			if info, ok := r.Get(entry.ProcessID, false); ok {
				r.userShellsMu.RUnlock()
				return info, nil
			}
			// Process died - we'll start a new one below
		}
		// Entry exists but no process (pre-registered) or process died
		// Keep the existing metadata (label, closable, createdAt, initialCommand)
		existingEntry = entry
	}
	r.userShellsMu.RUnlock()

	// Use initial command from pre-registered entry if not provided in opts
	initialCommand := opts.InitialCommand
	if initialCommand == "" && existingEntry != nil {
		initialCommand = existingEntry.InitialCommand
	}

	req := InteractiveStartRequest{
		SessionID:            sessionID,
		Command:              defaultShellCommand(preferredShell),
		WorkingDir:           workingDir,
		InitialCommand:       initialCommand,
		DisableTurnDetection: true, // User shells must not trigger turn complete / MarkReady
		IsUserShell:          true, // Exclude from session-level lookups (ResizeBySession, GetPtyWriterBySession)
	}

	info, err := r.Start(ctx, req)
	if err != nil {
		return nil, err
	}

	// Track the user shell with metadata
	// If entry already exists (auto-created by ListUserShells), preserve its metadata
	r.userShellsMu.Lock()
	if existingEntry != nil {
		// Update existing entry with the new process ID
		existingEntry.ProcessID = info.ID
		r.userShells[key] = existingEntry
	} else {
		// Create new entry
		closable := true // Default: closable
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
		zap.String("session_id", sessionID),
		zap.String("terminal_id", terminalID),
		zap.String("process_id", info.ID),
		zap.String("shell", req.Command[0]),
		zap.String("working_dir", workingDir),
		zap.String("label", opts.Label),
		zap.String("initial_command", opts.InitialCommand))

	return info, nil
}

// ListUserShells returns all user shells for a session, sorted by creation time.
// If no plain shell terminals exist, automatically creates the first "Terminal" entry.
func (r *InteractiveRunner) ListUserShells(sessionID string) []UserShellInfo {
	r.userShellsMu.Lock()
	defer r.userShellsMu.Unlock()

	prefix := sessionID + ":"
	var shells []UserShellInfo
	hasPlainShell := false

	for key, entry := range r.userShells {
		if strings.HasPrefix(key, prefix) {
			terminalID := key[len(prefix):]
			// Check if process is still alive
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
			// Check if this is a plain shell (not a script terminal)
			if entry.InitialCommand == "" {
				hasPlainShell = true
			}
		}
	}

	// Auto-create the first "Terminal" if no plain shells exist
	if !hasPlainShell {
		terminalID := "shell-" + uuid.New().String()
		now := time.Now().UTC()
		entry := &userShellEntry{
			ProcessID:      "", // No process yet - will be started when WebSocket connects
			Label:          "Terminal",
			InitialCommand: "",
			Closable:       false, // First terminal is not closable
			CreatedAt:      now,
		}
		r.userShells[prefix+terminalID] = entry

		shells = append(shells, UserShellInfo{
			TerminalID:     terminalID,
			ProcessID:      "",
			Running:        false,
			Label:          "Terminal",
			Closable:       false,
			InitialCommand: "",
			CreatedAt:      now,
		})
	}

	// Sort by creation time for stable ordering
	sort.Slice(shells, func(i, j int) bool {
		return shells[i].CreatedAt.Before(shells[j].CreatedAt)
	})

	return shells
}

// StopUserShell stops a user shell for a terminal tab.
func (r *InteractiveRunner) StopUserShell(ctx context.Context, sessionID, terminalID string) error {
	key := sessionID + ":" + terminalID

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
		zap.String("session_id", sessionID),
		zap.String("terminal_id", terminalID),
		zap.String("process_id", entry.ProcessID))

	return r.Stop(ctx, entry.ProcessID)
}

// ResizeUserShell resizes the PTY for a user shell.
func (r *InteractiveRunner) ResizeUserShell(sessionID, terminalID string, cols, rows uint16) error {
	key := sessionID + ":" + terminalID

	r.userShellsMu.RLock()
	entry, exists := r.userShells[key]
	r.userShellsMu.RUnlock()

	if !exists {
		return fmt.Errorf("no user shell found for session %s terminal %s", sessionID, terminalID)
	}

	proc, ok := r.get(entry.ProcessID)
	if !ok {
		return fmt.Errorf("process not found: %s", entry.ProcessID)
	}

	return r.lazyStartAndResize(proc, cols, rows,
		zap.String("session_id", sessionID),
		zap.String("terminal_id", terminalID),
	)
}

// GetUserShellPtyWriter returns the PTY writer for a user shell.
func (r *InteractiveRunner) GetUserShellPtyWriter(sessionID, terminalID string) (io.Writer, string, error) {
	key := sessionID + ":" + terminalID

	r.userShellsMu.RLock()
	entry, exists := r.userShells[key]
	r.userShellsMu.RUnlock()

	if !exists {
		return nil, "", fmt.Errorf("no user shell found for session %s terminal %s", sessionID, terminalID)
	}

	writer, err := r.GetPtyWriter(entry.ProcessID)
	if err != nil {
		return nil, entry.ProcessID, err
	}

	return writer, entry.ProcessID, nil
}

// ClearUserShellDirectOutput clears the direct output for a user shell.
func (r *InteractiveRunner) ClearUserShellDirectOutput(sessionID, terminalID string) {
	key := sessionID + ":" + terminalID

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
		zap.String("session_id", sessionID),
		zap.String("terminal_id", terminalID))
}
