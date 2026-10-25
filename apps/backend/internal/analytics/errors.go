package analytics

import (
	"errors"
	"fmt"
)

// ErrAnalyticsBusy marks a bounded analytics operation that could not finish
// within its admission-plus-query budget.
var ErrAnalyticsBusy = errors.New("analytics busy")

type analyticsBusyError struct {
	cause error
}

func (e *analyticsBusyError) Error() string {
	if e.cause == nil {
		return ErrAnalyticsBusy.Error()
	}
	return fmt.Sprintf("%s: %v", ErrAnalyticsBusy, e.cause)
}

func (e *analyticsBusyError) Unwrap() error { return e.cause }

func (e *analyticsBusyError) Is(target error) bool { return target == ErrAnalyticsBusy }

// NewAnalyticsBusyError wraps the internal timeout cause with the stable
// classification used by HTTP and plugin callers.
func NewAnalyticsBusyError(cause error) error {
	return &analyticsBusyError{cause: cause}
}

// IsAnalyticsBusy reports whether an analytics operation exhausted its bounded
// budget rather than failing because the caller canceled it.
func IsAnalyticsBusy(err error) bool { return errors.Is(err, ErrAnalyticsBusy) }
