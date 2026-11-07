package service

import "errors"

// ErrRunSessionAdmissionDeferred leaves the existing Office run/attempt as the
// owner of continuation. It is neither a failed provider attempt nor the task
// ceiling's separately-owned replay handoff (ErrLaunchDeferredByCapacity).
var ErrRunSessionAdmissionDeferred = errors.New("office run-session admission deferred")
