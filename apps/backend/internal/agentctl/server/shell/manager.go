package shell

import (
	"errors"
	"fmt"
	"sync"

	"github.com/kandev/kandev/internal/common/logger"
	"go.uber.org/zap"
)

// Manager manages multiple named shell sessions, one per terminal ID.
type Manager struct {
	workDir   string
	logger    *logger.Logger
	terminals map[string]*Session
	mu        sync.RWMutex
	stopping  bool
}

// BeginStop closes terminal admission after any start already holding the
// manager lock has either committed or failed.
func (m *Manager) BeginStop() {
	m.mu.Lock()
	m.stopping = true
	m.mu.Unlock()
}

// NewManager creates a new shell session manager.
func NewManager(workDir string, log *logger.Logger) *Manager {
	return &Manager{
		workDir:   workDir,
		logger:    log.WithFields(zap.String("component", "shell-manager")),
		terminals: make(map[string]*Session),
	}
}

// Start creates and starts a new shell session for the given terminal ID.
// If a session already exists for this terminal, it is returned as-is.
func (m *Manager) Start(terminalID string, cfg Config) (*Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.stopping {
		return nil, fmt.Errorf("shell manager is stopping")
	}

	if s, ok := m.terminals[terminalID]; ok {
		return s, nil
	}

	if cfg.WorkDir == "" {
		cfg.WorkDir = m.workDir
	}

	s, err := NewSession(cfg, m.logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create shell for terminal %s: %w", terminalID, err)
	}

	m.terminals[terminalID] = s
	m.logger.Info("terminal shell started", zap.String("terminal_id", terminalID))
	return s, nil
}

// Get returns the shell session for the given terminal ID.
func (m *Manager) Get(terminalID string) (*Session, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	s, ok := m.terminals[terminalID]
	return s, ok
}

// Stop stops and removes the shell session for the given terminal ID.
func (m *Manager) Stop(terminalID string) error {
	m.mu.RLock()
	s, ok := m.terminals[terminalID]
	m.mu.RUnlock()
	if !ok {
		return nil
	}

	m.logger.Info("stopping terminal shell", zap.String("terminal_id", terminalID))
	if err := s.Stop(); err != nil {
		return err
	}
	m.mu.Lock()
	if m.terminals[terminalID] == s {
		delete(m.terminals, terminalID)
	}
	m.mu.Unlock()
	return nil
}

// Buffer returns the buffered output for the given terminal ID.
func (m *Manager) Buffer(terminalID string) ([]byte, error) {
	m.mu.RLock()
	s, ok := m.terminals[terminalID]
	m.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("terminal %s not found", terminalID)
	}
	return s.GetBufferedOutput(), nil
}

// StopAll stops all terminal shell sessions.
func (m *Manager) StopAll() error {
	m.mu.Lock()
	terminals := make(map[string]*Session, len(m.terminals))
	for id, s := range m.terminals {
		terminals[id] = s
	}
	m.mu.Unlock()

	var errs []error
	for id, s := range terminals {
		m.logger.Info("stopping terminal shell (shutdown)", zap.String("terminal_id", id))
		if err := s.Stop(); err != nil {
			errs = append(errs, fmt.Errorf("stop terminal %s: %w", id, err))
			continue
		}
		m.mu.Lock()
		if m.terminals[id] == s {
			delete(m.terminals, id)
		}
		m.mu.Unlock()
	}
	return errors.Join(errs...)
}
