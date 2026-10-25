package sandbox

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"unicode"
)

// AgentStateScope separates native agent history across execution authorities.
type AgentStateScope struct {
	OrgID          string `json:"org_id"`
	WorkspaceID    string `json:"workspace_id"`
	SessionID      string `json:"session_id"`
	ExecutionID    string `json:"execution_id,omitempty"`
	AgentProfileID string `json:"agent_profile_id"`
	InstallationID string `json:"installation_id"`
	ProviderKey    string `json:"provider_key"`
}

type AgentStateSnapshot struct {
	Scope AgentStateScope `json:"scope"`
	Home  string          `json:"home"`
}

// AgentState holds a private ownership lock outside the child-visible directory.
// Closing the lease preserves session files for a later attempt.
type AgentState struct {
	mu       sync.Mutex
	snapshot AgentStateSnapshot
	root     string
	identity os.FileInfo
	lock     *os.File
}

func OpenAgentState(base string, scope AgentStateScope) (*AgentState, error) {
	if !validRuntimeDirectory(base) || !validAgentStateScope(scope) {
		return nil, ErrInvalidPolicy
	}
	raw, err := json.Marshal(scope)
	if err != nil {
		return nil, ErrInvalidPolicy
	}
	root := filepath.Join(base, "agent-state-"+runtimeDigest(raw))
	created := false
	if err = os.Mkdir(root, 0700); err == nil {
		created = true
	} else if !errors.Is(err, os.ErrExist) {
		return nil, ErrInvalidPolicy
	}
	if !validRuntimeDirectory(root) {
		return nil, ErrInvalidPolicy
	}
	identity, err := os.Stat(root)
	if err != nil {
		return nil, ErrInvalidPolicy
	}
	marker := filepath.Join(root, "owner.json")
	if created {
		if err = initializeAgentState(root, marker, raw); err != nil {
			return nil, ErrInvalidPolicy
		}
	}
	file, err := lockAgentState(marker)
	if err != nil {
		return nil, ErrInvalidPolicy
	}
	content, err := io.ReadAll(io.LimitReader(file, 4097))
	if err != nil || string(content) != string(raw) {
		_ = file.Close()
		return nil, ErrInvalidPolicy
	}
	state := &AgentState{root: root, identity: identity, lock: file, snapshot: AgentStateSnapshot{Scope: scope, Home: filepath.Join(root, "home")}}
	if !state.valid() {
		_ = state.Close()
		return nil, ErrInvalidPolicy
	}
	return state, nil
}

func initializeAgentState(root, marker string, raw []byte) error {
	home := filepath.Join(root, "home")
	for _, dir := range []string{home, filepath.Join(home, ".config"), filepath.Join(home, ".local-state"), filepath.Join(home, ".tmp")} {
		if err := os.Mkdir(dir, 0700); err != nil {
			return ErrInvalidPolicy
		}
	}
	return writeRuntimeFile(marker, raw, 0600)
}

func validAgentStateScope(scope AgentStateScope) bool {
	if (scope.SessionID == "") == (scope.ExecutionID == "") {
		return false
	}
	for _, value := range []string{scope.OrgID, scope.AgentProfileID, scope.InstallationID, scope.ProviderKey} {
		if value == "" {
			return false
		}
	}
	for _, value := range []string{scope.OrgID, scope.WorkspaceID, scope.SessionID, scope.ExecutionID, scope.AgentProfileID, scope.InstallationID, scope.ProviderKey} {
		if len(value) > 128 || strings.ContainsFunc(value, unicode.IsControl) {
			return false
		}
	}
	return true
}

func (s *AgentState) Directories() RuntimeDirectories {
	if s == nil {
		return RuntimeDirectories{}
	}
	return s.snapshot.Directories()
}

// Directories derives child paths from captured state without opening or allocating files.
func (s *AgentStateSnapshot) Directories() RuntimeDirectories {
	if s == nil {
		return RuntimeDirectories{}
	}
	home := s.Home
	return RuntimeDirectories{Home: home, Config: filepath.Join(home, ".config"), State: filepath.Join(home, ".local-state"), Temp: filepath.Join(home, ".tmp")}
}

func (s *AgentState) Snapshot() *AgentStateSnapshot {
	if s == nil {
		return nil
	}
	result := s.snapshot
	return &result
}

func (s *AgentState) valid() bool {
	if s == nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.lock == nil {
		return false
	}
	current, err := os.Lstat(s.root)
	if err != nil || !os.SameFile(current, s.identity) || !validRuntimeDirectory(s.root) {
		return false
	}
	dirs := s.Directories()
	for _, dir := range []string{dirs.Home, dirs.Config, dirs.State, dirs.Temp} {
		if !validRuntimeDirectory(dir) {
			return false
		}
	}
	return true
}

func (s *AgentState) Close() error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.lock == nil {
		return nil
	}
	err := s.lock.Close()
	s.lock = nil
	return err
}

// Remove deletes only this lease's allocation. The lifecycle owner must first
// prove that every process using the allocation has stopped.
func (s *AgentState) Remove() error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.lock == nil {
		return ErrInvalidPolicy
	}
	current, err := os.Lstat(s.root)
	if err != nil || !os.SameFile(current, s.identity) || !validRuntimeDirectory(s.root) {
		return ErrInvalidPolicy
	}
	if err = os.RemoveAll(s.root); err != nil {
		return ErrInvalidPolicy
	}
	err = s.lock.Close()
	s.lock = nil
	return err
}
