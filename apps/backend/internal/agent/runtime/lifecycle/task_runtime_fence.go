package lifecycle

import "sync"

// taskRuntimeOwnershipFences coordinate runtime creation with operations that
// must make an ownership decision and act on the remote runtime as one step.
// A task uses a read lock for creation and an exclusive lock for cleanup.
type taskRuntimeOwnershipFences struct {
	byTask sync.Map // map[string]*sync.RWMutex
}

func (f *taskRuntimeOwnershipFences) lockForTask(taskID string) *sync.RWMutex {
	value, _ := f.byTask.LoadOrStore(taskID, &sync.RWMutex{})
	return value.(*sync.RWMutex)
}

func (f *taskRuntimeOwnershipFences) acquireCreation(taskID string) func() {
	lock := f.lockForTask(taskID)
	lock.RLock()
	return lock.RUnlock
}

func (f *taskRuntimeOwnershipFences) acquireSweep(taskID string) func() {
	lock := f.lockForTask(taskID)
	lock.Lock()
	return lock.Unlock
}

// AcquireSSHOrphanSweepFence prevents runtime creation for taskID while the
// orphan sweep performs its final state read and remote stop. Callers must
// release the returned function after that remote operation completes.
func (m *Manager) AcquireSSHOrphanSweepFence(taskID string) func() {
	return m.taskRuntimeFences.acquireSweep(taskID)
}
