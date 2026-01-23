// Package lifecycle manages agent execution lifecycles including tracking,
// state transitions, and cleanup.
package lifecycle

import (
	"errors"
	"sync"

	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// ErrExecutionNotFound is returned when an execution doesn't exist in the store.
var ErrExecutionNotFound = errors.New("execution not found")

// ExecutionStore provides thread-safe storage and retrieval of agent executions.
// It maintains three indexes for efficient lookup by execution ID, session ID, and container ID.
type ExecutionStore struct {
	executions  map[string]*AgentExecution
	bySession   map[string]string // sessionID -> executionID
	byContainer map[string]string // containerID -> executionID
	mu          sync.RWMutex
}

// NewExecutionStore creates a new ExecutionStore with initialized maps.
func NewExecutionStore() *ExecutionStore {
	return &ExecutionStore{
		executions:  make(map[string]*AgentExecution),
		bySession:   make(map[string]string),
		byContainer: make(map[string]string),
	}
}

// Add adds an agent execution to all tracking maps.
// The execution must have a valid ID. SessionID and ContainerID are optional
// but will be indexed if present.
func (s *ExecutionStore) Add(execution *AgentExecution) {
	if execution == nil || execution.ID == "" {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.executions[execution.ID] = execution

	if execution.SessionID != "" {
		s.bySession[execution.SessionID] = execution.ID
	}

	if execution.ContainerID != "" {
		s.byContainer[execution.ContainerID] = execution.ID
	}
}

// Remove removes an agent execution from all tracking maps.
func (s *ExecutionStore) Remove(executionID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	execution, exists := s.executions[executionID]
	if !exists {
		return
	}

	// Remove from secondary indexes
	if execution.SessionID != "" {
		delete(s.bySession, execution.SessionID)
	}
	if execution.ContainerID != "" {
		delete(s.byContainer, execution.ContainerID)
	}

	// Remove from primary map
	delete(s.executions, executionID)
}

// Get returns an agent execution by its ID.
func (s *ExecutionStore) Get(executionID string) (*AgentExecution, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	execution, exists := s.executions[executionID]
	return execution, exists
}

// GetBySessionID returns the agent execution associated with a session ID.
func (s *ExecutionStore) GetBySessionID(sessionID string) (*AgentExecution, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	executionID, exists := s.bySession[sessionID]
	if !exists {
		return nil, false
	}

	execution, exists := s.executions[executionID]
	return execution, exists
}

// GetByContainerID returns the agent execution associated with a container ID.
func (s *ExecutionStore) GetByContainerID(containerID string) (*AgentExecution, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	executionID, exists := s.byContainer[containerID]
	if !exists {
		return nil, false
	}

	execution, exists := s.executions[executionID]
	return execution, exists
}

// List returns all tracked agent executions.
func (s *ExecutionStore) List() []*AgentExecution {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]*AgentExecution, 0, len(s.executions))
	for _, execution := range s.executions {
		result = append(result, execution)
	}
	return result
}

// UpdateStatus updates the status of an agent execution.
func (s *ExecutionStore) UpdateStatus(executionID string, status v1.AgentStatus) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if execution, exists := s.executions[executionID]; exists {
		execution.Status = status
	}
}

// UpdateError updates the error message of an agent execution and sets its status to failed.
func (s *ExecutionStore) UpdateError(executionID string, errorMsg string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if execution, exists := s.executions[executionID]; exists {
		execution.ErrorMessage = errorMsg
		execution.Status = v1.AgentStatusFailed
	}
}

// UpdateMetadata updates the metadata of an agent execution using the provided function.
// The function is called outside the lock to prevent deadlocks - it receives a copy of
// the current metadata and returns the new metadata to set.
func (s *ExecutionStore) UpdateMetadata(executionID string, updateFn func(metadata map[string]interface{}) map[string]interface{}) {
	// First, get a copy of current metadata
	s.mu.RLock()
	execution, exists := s.executions[executionID]
	if !exists {
		s.mu.RUnlock()
		return
	}

	// Copy metadata to avoid races
	currentMetadata := make(map[string]interface{})
	if execution.Metadata != nil {
		for k, v := range execution.Metadata {
			currentMetadata[k] = v
		}
	}
	s.mu.RUnlock()

	// Call update function outside the lock
	newMetadata := updateFn(currentMetadata)

	// Apply the result
	s.mu.Lock()
	defer s.mu.Unlock()

	// Re-check existence (could have been removed)
	if execution, exists = s.executions[executionID]; exists {
		execution.Metadata = newMetadata
	}
}

// WithLock executes a function with the store lock held, providing access to the execution.
// Returns ErrExecutionNotFound if the execution doesn't exist.
// The function should be fast to avoid blocking other operations.
func (s *ExecutionStore) WithLock(executionID string, fn func(execution *AgentExecution)) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	execution, exists := s.executions[executionID]
	if !exists {
		return ErrExecutionNotFound
	}
	fn(execution)
	return nil
}

// WithRLock executes a function with the store read lock held, providing access to the execution.
// Returns ErrExecutionNotFound if the execution doesn't exist.
func (s *ExecutionStore) WithRLock(executionID string, fn func(execution *AgentExecution)) error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	execution, exists := s.executions[executionID]
	if !exists {
		return ErrExecutionNotFound
	}
	fn(execution)
	return nil
}
