// Package lifecycle manages agent execution lifecycles including tracking,
// state transitions, and cleanup.
package lifecycle

import (
	"errors"
	"fmt"
	"sync"

	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// ErrExecutionNotFound is returned when an execution doesn't exist in the store.
var ErrExecutionNotFound = errors.New("execution not found")

// ErrPromptActivityNotOwned means a cancellation snapshot no longer belongs
// to the execution and prompt that were captured.
var ErrPromptActivityNotOwned = errors.New("prompt activity no longer owned")

// ErrExecutionAlreadyExistsForSession is returned by Add when the session
// already maps to a different execution. The previous behavior was to silently
// overwrite the bySession index, which orphaned the prior execution: the
// agentctl/subprocess kept running but was unreachable via GetBySessionID, so
// nothing ever cleaned it up. Callers must Remove the prior execution before
// adding a replacement.
var ErrExecutionAlreadyExistsForSession = errors.New("execution already exists for session")

// ErrExecutionAlreadyExistsForTaskHost is returned by Add when a task
// environment already owns a dedicated task-host execution.
var ErrExecutionAlreadyExistsForTaskHost = errors.New("execution already exists for task host")

// ErrAgentAlreadyRunning is returned by LaunchAgent when an agent execution is
// already tracked for the requested session. The error fires both when the
// execution is live (a concurrent caller raced us) and when it is stale (a
// prior registration whose process never started or exited without cleanup);
// callers must probe IsAgentRunningForSession to decide. Wrapped at producer
// sites with %w so callers can use errors.Is rather than string-match the
// surrounding context.
var ErrAgentAlreadyRunning = errors.New("session already has an agent running")

// ErrAgentReported is returned when the agent subprocess publishes an error
// event during a prompt turn. Distinguished from infrastructure errors
// (network, timeout) so callers can suppress duplicate error-message creation
// — the agent failure path already records the error in the session.
var ErrAgentReported = errors.New("agent error")

// ExecutionStore provides thread-safe storage and retrieval of agent executions.
// It maintains indexes for execution, session, task-host environment, and container.
type ExecutionStore struct {
	executions            map[string]*AgentExecution
	bySession             map[string]string // sessionID -> executionID
	byTaskHostEnvironment map[string]string // taskEnvironmentID -> dedicated task-host executionID
	byContainer           map[string]string // containerID -> executionID
	mu                    sync.RWMutex
}

// NewExecutionStore creates a new ExecutionStore with initialized maps.
func NewExecutionStore() *ExecutionStore {
	return &ExecutionStore{
		executions:            make(map[string]*AgentExecution),
		bySession:             make(map[string]string),
		byTaskHostEnvironment: make(map[string]string),
		byContainer:           make(map[string]string),
	}
}

// Add adds an agent execution to all tracking maps.
// The execution must have a valid ID. SessionID and ContainerID are optional
// but will be indexed if present.
//
// Returns ErrExecutionAlreadyExistsForSession if the session is already mapped
// to a different execution. Callers must explicitly Remove the prior execution
// before adding a replacement — otherwise the prior subprocess is orphaned.
func (s *ExecutionStore) Add(execution *AgentExecution) error {
	if execution == nil || execution.ID == "" {
		return fmt.Errorf("execution and execution.ID are required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if execution.IsTaskHost && execution.TaskEnvironmentID == "" {
		return fmt.Errorf("task-host execution requires task_environment_id")
	}
	if execution.IsTaskHost {
		if existingID, exists := s.byTaskHostEnvironment[execution.TaskEnvironmentID]; exists && existingID != execution.ID {
			return fmt.Errorf("%w: task_environment=%s existing=%s new=%s",
				ErrExecutionAlreadyExistsForTaskHost,
				execution.TaskEnvironmentID, existingID, execution.ID)
		}
	} else if execution.SessionID != "" {
		if existingID, exists := s.bySession[execution.SessionID]; exists && existingID != execution.ID {
			return fmt.Errorf("%w: session=%s existing=%s new=%s",
				ErrExecutionAlreadyExistsForSession,
				execution.SessionID, existingID, execution.ID)
		}
	}

	s.executions[execution.ID] = execution

	if execution.IsTaskHost {
		s.byTaskHostEnvironment[execution.TaskEnvironmentID] = execution.ID
	} else if execution.SessionID != "" {
		s.bySession[execution.SessionID] = execution.ID
	}

	if execution.ContainerID != "" {
		s.byContainer[execution.ContainerID] = execution.ID
	}
	return nil
}

// Remove removes an agent execution from all tracking maps.
func (s *ExecutionStore) Remove(executionID string) {
	s.RemoveIfSame(executionID, nil)
}

// RemoveIfSame removes executionID only when it still points at expected.
// A nil expected preserves Remove's unconditional behavior. Pointer identity
// prevents a delayed health probe from evicting a replacement registered under
// the same stable task-host ID.
func (s *ExecutionStore) RemoveIfSame(executionID string, expected *AgentExecution) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	execution, exists := s.executions[executionID]
	if !exists || (expected != nil && execution != expected) {
		return false
	}

	// Remove from secondary indexes
	if execution.IsTaskHost {
		delete(s.byTaskHostEnvironment, execution.TaskEnvironmentID)
	} else if execution.SessionID != "" {
		delete(s.bySession, execution.SessionID)
	}
	if execution.ContainerID != "" {
		delete(s.byContainer, execution.ContainerID)
	}
	execution.clearRuntimeEnvironment()

	// Remove from primary map
	delete(s.executions, executionID)
	return true
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

// OwnsPromptGeneration reports whether executionID and generation still own
// sessionID's active lifecycle prompt.
func (s *ExecutionStore) OwnsPromptGeneration(sessionID, executionID string, generation uint64) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	currentExecutionID, exists := s.bySession[sessionID]
	if !exists || currentExecutionID != executionID {
		return false
	}
	execution, exists := s.executions[currentExecutionID]
	return exists && generation != 0 && execution.promptGeneration == generation
}

// OwnsPromptActivity reports whether the execution still owns the prompt and
// its activity has not changed since the watchdog captured its snapshot.
func (s *ExecutionStore) OwnsPromptActivity(
	sessionID, executionID string,
	generation, activityEpoch uint64,
) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	currentExecutionID, exists := s.bySession[sessionID]
	if !exists || currentExecutionID != executionID {
		return false
	}
	execution, exists := s.executions[currentExecutionID]
	if !exists || generation == 0 || activityEpoch == 0 || execution.promptGeneration != generation {
		return false
	}
	return execution.promptActivityEpochSnapshot() == activityEpoch
}

// ClaimPromptActivity validates a captured prompt identity while the store
// lock is held and returns that exact execution for a lifecycle operation.
// Callers must use the returned execution instead of looking it up by session
// again, because a session can acquire a replacement execution while the
// operation waits for the agent to settle.
func (s *ExecutionStore) ClaimPromptActivity(
	sessionID, executionID string,
	generation, activityEpoch uint64,
) (*AgentExecution, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	currentExecutionID, exists := s.bySession[sessionID]
	if !exists {
		return nil, ErrExecutionNotFound
	}
	execution, exists := s.executions[currentExecutionID]
	if !exists {
		return nil, ErrExecutionNotFound
	}
	if currentExecutionID != executionID || generation == 0 || activityEpoch == 0 || execution.promptGeneration != generation || execution.promptActivityEpochSnapshot() != activityEpoch {
		return nil, ErrPromptActivityNotOwned
	}
	return execution, nil
}

// ActivePromptGeneration returns the generation of the prompt that is dispatched
// and still in flight for executionID, or 0 if there is none. A steer reuses this
// exact generation so the folded completion is attributed to the predecessor's
// still-open waiter rather than discarded as a mismatch. It deliberately returns
// 0 for a generation that is only admitted (not yet dispatched — its buffers are
// mid-reset) or already completed, so a steer never overtakes an about-to-start
// turn or attaches to a finished one; those cases fall back to an ordinary prompt.
func (s *ExecutionStore) ActivePromptGeneration(executionID string) uint64 {
	s.mu.RLock()
	defer s.mu.RUnlock()

	execution, exists := s.executions[executionID]
	if !exists {
		return 0
	}
	gen := execution.promptGeneration
	if gen == 0 || execution.dispatchedPromptGeneration != gen || execution.promptCompletionGeneration == gen {
		return 0
	}
	return gen
}

// GetByTaskEnvironmentID returns any execution associated with a task environment ID.
func (s *ExecutionStore) GetByTaskEnvironmentID(taskEnvironmentID string) (*AgentExecution, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, execution := range s.executions {
		if !execution.IsTaskHost && execution.TaskEnvironmentID == taskEnvironmentID {
			return execution, true
		}
	}
	return nil, false
}

// GetTaskHostByEnvironmentID returns the dedicated task-host execution for a
// task environment. It never falls back to a session-owned execution.
func (s *ExecutionStore) GetTaskHostByEnvironmentID(taskEnvironmentID string) (*AgentExecution, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	executionID, exists := s.byTaskHostEnvironment[taskEnvironmentID]
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

// BeginPrompt assigns a new immutable identity to the next prompt dispatch.
func (s *ExecutionStore) BeginPrompt(executionID string) (uint64, error) {
	execution, exists := s.Get(executionID)
	if !exists {
		return 0, ErrExecutionNotFound
	}
	execution.promptLifecycleMu.Lock()
	defer execution.promptLifecycleMu.Unlock()

	s.mu.Lock()
	defer s.mu.Unlock()

	current, exists := s.executions[executionID]
	if !exists || current != execution {
		return 0, ErrExecutionNotFound
	}
	return beginExecutionPrompt(current), nil
}

func beginExecutionPrompt(execution *AgentExecution) uint64 {
	execution.promptGeneration++
	execution.promptCompletionGeneration = 0
	// The new generation is not dispatched until its triggerPrompt succeeds; a
	// steer must not reuse it until then (its buffers are still being reset).
	execution.dispatchedPromptGeneration = 0
	execution.ProviderError = nil
	execution.Status = v1.AgentStatusRunning
	execution.resetActiveTool()
	return execution.promptGeneration
}

// MarkPromptDispatched records that generation reached agentctl and is in
// flight. Ignored if a newer generation has since begun or this one already
// completed, so a steer can only ever reuse a live dispatched turn.
func (s *ExecutionStore) MarkPromptDispatched(executionID string, generation uint64) {
	if generation == 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	execution, exists := s.executions[executionID]
	if !exists {
		return
	}
	if execution.promptGeneration != generation || execution.promptCompletionGeneration == generation {
		return
	}
	execution.dispatchedPromptGeneration = generation
	execution.armPromptActivity()
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
