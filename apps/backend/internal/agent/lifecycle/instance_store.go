// Package lifecycle manages agent instance lifecycles including tracking,
// state transitions, and cleanup.
package lifecycle

import (
	"sync"

	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// InstanceStore provides thread-safe storage and retrieval of agent instances.
// It maintains three indexes for efficient lookup by instance ID, task ID, and container ID.
type InstanceStore struct {
	instances   map[string]*AgentInstance
	byTask      map[string]string // taskID -> instanceID
	byContainer map[string]string // containerID -> instanceID
	mu          sync.RWMutex
}

// NewInstanceStore creates a new InstanceStore with initialized maps.
func NewInstanceStore() *InstanceStore {
	return &InstanceStore{
		instances:   make(map[string]*AgentInstance),
		byTask:      make(map[string]string),
		byContainer: make(map[string]string),
	}
}

// Add adds an agent instance to all tracking maps.
// The instance must have a valid ID. TaskID and ContainerID are optional
// but will be indexed if present.
func (s *InstanceStore) Add(instance *AgentInstance) {
	if instance == nil || instance.ID == "" {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.instances[instance.ID] = instance

	if instance.TaskID != "" {
		s.byTask[instance.TaskID] = instance.ID
	}

	if instance.ContainerID != "" {
		s.byContainer[instance.ContainerID] = instance.ID
	}
}

// Remove removes an agent instance from all tracking maps.
func (s *InstanceStore) Remove(instanceID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	instance, exists := s.instances[instanceID]
	if !exists {
		return
	}

	// Remove from secondary indexes
	if instance.TaskID != "" {
		delete(s.byTask, instance.TaskID)
	}
	if instance.ContainerID != "" {
		delete(s.byContainer, instance.ContainerID)
	}

	// Remove from primary map
	delete(s.instances, instanceID)
}

// Get returns an agent instance by its ID.
func (s *InstanceStore) Get(instanceID string) (*AgentInstance, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	instance, exists := s.instances[instanceID]
	return instance, exists
}

// GetByTaskID returns the agent instance associated with a task ID.
func (s *InstanceStore) GetByTaskID(taskID string) (*AgentInstance, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	instanceID, exists := s.byTask[taskID]
	if !exists {
		return nil, false
	}

	instance, exists := s.instances[instanceID]
	return instance, exists
}

// GetByContainerID returns the agent instance associated with a container ID.
func (s *InstanceStore) GetByContainerID(containerID string) (*AgentInstance, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	instanceID, exists := s.byContainer[containerID]
	if !exists {
		return nil, false
	}

	instance, exists := s.instances[instanceID]
	return instance, exists
}

// List returns all tracked agent instances.
func (s *InstanceStore) List() []*AgentInstance {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]*AgentInstance, 0, len(s.instances))
	for _, instance := range s.instances {
		result = append(result, instance)
	}
	return result
}

// UpdateStatus updates the status of an agent instance.
func (s *InstanceStore) UpdateStatus(instanceID string, status v1.AgentStatus) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if instance, exists := s.instances[instanceID]; exists {
		instance.Status = status
	}
}

// UpdateProgress updates the progress of an agent instance.
func (s *InstanceStore) UpdateProgress(instanceID string, progress int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if instance, exists := s.instances[instanceID]; exists {
		instance.Progress = progress
	}
}

// UpdateError updates the error message of an agent instance and sets its status to failed.
func (s *InstanceStore) UpdateError(instanceID string, errorMsg string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if instance, exists := s.instances[instanceID]; exists {
		instance.ErrorMessage = errorMsg
		instance.Status = v1.AgentStatusFailed
	}
}

// UpdateMetadata updates the metadata of an agent instance using the provided function.
func (s *InstanceStore) UpdateMetadata(instanceID string, updateFn func(metadata map[string]interface{}) map[string]interface{}) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if instance, exists := s.instances[instanceID]; exists {
		if instance.Metadata == nil {
			instance.Metadata = make(map[string]interface{})
		}
		instance.Metadata = updateFn(instance.Metadata)
	}
}

// WithLock executes a function with the store lock held, providing access to the instance.
// Returns false if the instance doesn't exist.
func (s *InstanceStore) WithLock(instanceID string, fn func(instance *AgentInstance)) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	instance, exists := s.instances[instanceID]
	if !exists {
		return false
	}
	fn(instance)
	return true
}

// WithRLock executes a function with the store read lock held, providing access to the instance.
// Returns false if the instance doesn't exist.
func (s *InstanceStore) WithRLock(instanceID string, fn func(instance *AgentInstance)) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	instance, exists := s.instances[instanceID]
	if !exists {
		return false
	}
	fn(instance)
	return true
}
