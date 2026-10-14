package routingerr

import (
	"errors"
	"fmt"
)

// BuiltCommandValidationError identifies invalid executable arguments before
// process startup. It remains recognizable through launch and recovery wrappers.
type BuiltCommandValidationError struct {
	Command string
	Cause   error
}

func (e *BuiltCommandValidationError) Error() string {
	return fmt.Sprintf("validate %s command: %v", e.Command, e.Cause)
}

func (e *BuiltCommandValidationError) Unwrap() error { return e.Cause }

// IsBuiltCommandValidationError recognizes the command producer's typed
// failure, independently of display text from providers or error wrappers.
func IsBuiltCommandValidationError(err error) bool {
	var validation *BuiltCommandValidationError
	return errors.As(err, &validation)
}
