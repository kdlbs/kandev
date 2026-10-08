package lifecycle

import "errors"

// ErrExecutorInterrupted transfers settlement ownership to the task recovery owner.
var ErrExecutorInterrupted = errors.New("executor interrupted")
