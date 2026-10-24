package pluginsdk

import (
	"fmt"
	"net/http"
)

// ActionErrorCode is a transport-independent failure category plugins can use
// when turning domain errors into stable action responses.
type ActionErrorCode string

const (
	ActionErrorInvalidArgument  ActionErrorCode = "invalid_argument"
	ActionErrorUnauthenticated  ActionErrorCode = "unauthenticated"
	ActionErrorPermissionDenied ActionErrorCode = "permission_denied"
	ActionErrorNotFound         ActionErrorCode = "not_found"
	ActionErrorConflict         ActionErrorCode = "conflict"
	ActionErrorRateLimited      ActionErrorCode = "rate_limited"
	ActionErrorUnavailable      ActionErrorCode = "unavailable"
	ActionErrorUpstream         ActionErrorCode = "upstream"
)

// ActionError preserves a machine-readable category and optional retry hint.
// Plugins should return sanitized response bodies; Err is retained only for
// errors.Is/errors.As and local diagnostics.
type ActionError struct {
	Code       ActionErrorCode
	RetryAfter string
	Err        error
}

func (e *ActionError) Error() string {
	if e == nil {
		return "plugin action failed"
	}
	if e.Err != nil {
		return fmt.Sprintf("plugin action %s: %v", e.Code, e.Err)
	}
	return fmt.Sprintf("plugin action %s", e.Code)
}

func (e *ActionError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// CategorizeActionError wraps err without flattening its cause.
func CategorizeActionError(code ActionErrorCode, err error) error {
	return CategorizeActionErrorWithRetry(code, "", err)
}

// CategorizeActionErrorWithRetry also projects a provider-neutral Retry-After
// value without flattening the original error.
func CategorizeActionErrorWithRetry(code ActionErrorCode, retryAfter string, err error) error {
	if err == nil {
		return nil
	}
	return &ActionError{Code: code, RetryAfter: retryAfter, Err: err}
}

// ActionErrorHTTPStatus is the shared action-category projection used by
// plugins. Unknown future categories fail closed as an upstream failure.
func ActionErrorHTTPStatus(code ActionErrorCode) int {
	switch code {
	case ActionErrorInvalidArgument:
		return http.StatusBadRequest
	case ActionErrorUnauthenticated:
		return http.StatusUnauthorized
	case ActionErrorPermissionDenied:
		return http.StatusForbidden
	case ActionErrorNotFound:
		return http.StatusNotFound
	case ActionErrorConflict:
		return http.StatusConflict
	case ActionErrorRateLimited:
		return http.StatusTooManyRequests
	case ActionErrorUnavailable:
		return http.StatusServiceUnavailable
	default:
		return http.StatusBadGateway
	}
}
