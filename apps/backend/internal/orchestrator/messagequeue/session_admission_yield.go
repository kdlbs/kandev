package messagequeue

import (
	"context"
	"sync/atomic"
)

type sessionAdmissionLease struct {
	released atomic.Bool
	release  func()
}

// YieldSessionAdmission releases only the current gate. The callback must
// keep running synchronously and its context can no longer reuse ownership.
// A provider refresh calls this only after its durable dispatch barrier commits.
func (s *Service) YieldSessionAdmission(ctx context.Context) bool {
	token, ok := ctx.Value(sessionAdmissionContextKey{}).(sessionAdmissionToken)
	if !ok || token.service != s || token.lease == nil || !token.lease.released.CompareAndSwap(false, true) {
		return false
	}
	token.lease.release()
	return true
}
