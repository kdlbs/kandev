package service

// PreAllocationTaskCreateError marks a task-create failure returned before a
// task row was allocated. HTTP callers may let a user correct the request and
// submit it with a new idempotency identity.
type PreAllocationTaskCreateError struct {
	Cause error
}

func (e *PreAllocationTaskCreateError) Error() string { return e.Cause.Error() }

func (e *PreAllocationTaskCreateError) Unwrap() error { return e.Cause }

func newPreAllocationTaskCreateError(err error) error {
	if err == nil {
		return nil
	}
	return &PreAllocationTaskCreateError{Cause: err}
}
