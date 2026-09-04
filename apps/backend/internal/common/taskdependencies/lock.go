package taskdependencies

import "sync"

var edgeMutationMu sync.Mutex

// AcquireMutationLock serializes dependency graph validation and mutation
// across task and Office services in this backend process.
func AcquireMutationLock() func() {
	edgeMutationMu.Lock()
	return edgeMutationMu.Unlock
}
