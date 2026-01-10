// Package errors provides custom error types for the Kandev application.
package errors

import (
	"errors"
	"fmt"
	"net/http"
)

// Error codes as constants
const (
	ErrCodeNotFound           = "NOT_FOUND"
	ErrCodeBadRequest         = "BAD_REQUEST"
	ErrCodeUnauthorized       = "UNAUTHORIZED"
	ErrCodeForbidden          = "FORBIDDEN"
	ErrCodeInternalError      = "INTERNAL_ERROR"
	ErrCodeConflict           = "CONFLICT"
	ErrCodeValidationError    = "VALIDATION_ERROR"
	ErrCodeServiceUnavailable = "SERVICE_UNAVAILABLE"
)

// AppError represents an application-specific error with additional context.
type AppError struct {
	Code       string `json:"code"`
	Message    string `json:"message"`
	HTTPStatus int    `json:"http_status"`
	Err        error  `json:"-"`
}

// Error implements the error interface.
func (e *AppError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %s: %v", e.Code, e.Message, e.Err)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// Unwrap returns the wrapped error for use with errors.Is and errors.As.
func (e *AppError) Unwrap() error {
	return e.Err
}

// NotFound creates a new not found error for a resource.
func NotFound(resource string, id string) *AppError {
	return &AppError{
		Code:       ErrCodeNotFound,
		Message:    fmt.Sprintf("%s with id '%s' not found", resource, id),
		HTTPStatus: http.StatusNotFound,
	}
}

// BadRequest creates a new bad request error.
func BadRequest(message string) *AppError {
	return &AppError{
		Code:       ErrCodeBadRequest,
		Message:    message,
		HTTPStatus: http.StatusBadRequest,
	}
}

// Unauthorized creates a new unauthorized error.
func Unauthorized(message string) *AppError {
	return &AppError{
		Code:       ErrCodeUnauthorized,
		Message:    message,
		HTTPStatus: http.StatusUnauthorized,
	}
}

// Forbidden creates a new forbidden error.
func Forbidden(message string) *AppError {
	return &AppError{
		Code:       ErrCodeForbidden,
		Message:    message,
		HTTPStatus: http.StatusForbidden,
	}
}

// InternalError creates a new internal server error with a wrapped underlying error.
func InternalError(message string, err error) *AppError {
	return &AppError{
		Code:       ErrCodeInternalError,
		Message:    message,
		HTTPStatus: http.StatusInternalServerError,
		Err:        err,
	}
}

// Conflict creates a new conflict error.
func Conflict(message string) *AppError {
	return &AppError{
		Code:       ErrCodeConflict,
		Message:    message,
		HTTPStatus: http.StatusConflict,
	}
}

// ValidationError creates a new validation error for a specific field.
func ValidationError(field string, message string) *AppError {
	return &AppError{
		Code:       ErrCodeValidationError,
		Message:    fmt.Sprintf("validation failed for field '%s': %s", field, message),
		HTTPStatus: http.StatusBadRequest,
	}
}

// ServiceUnavailable creates a new service unavailable error.
func ServiceUnavailable(service string) *AppError {
	return &AppError{
		Code:       ErrCodeServiceUnavailable,
		Message:    fmt.Sprintf("service '%s' is currently unavailable", service),
		HTTPStatus: http.StatusServiceUnavailable,
	}
}

// Wrap wraps an existing error with additional context, returning an AppError.
func Wrap(err error, message string) *AppError {
	if err == nil {
		return nil
	}

	// If the error is already an AppError, preserve its code and status
	var appErr *AppError
	if errors.As(err, &appErr) {
		return &AppError{
			Code:       appErr.Code,
			Message:    fmt.Sprintf("%s: %s", message, appErr.Message),
			HTTPStatus: appErr.HTTPStatus,
			Err:        err,
		}
	}

	// Otherwise, wrap as an internal error
	return &AppError{
		Code:       ErrCodeInternalError,
		Message:    message,
		HTTPStatus: http.StatusInternalServerError,
		Err:        err,
	}
}

// IsNotFound checks if the error is a not found error.
func IsNotFound(err error) bool {
	var appErr *AppError
	if errors.As(err, &appErr) {
		return appErr.Code == ErrCodeNotFound
	}
	return false
}

// IsBadRequest checks if the error is a bad request error.
func IsBadRequest(err error) bool {
	var appErr *AppError
	if errors.As(err, &appErr) {
		return appErr.Code == ErrCodeBadRequest || appErr.Code == ErrCodeValidationError
	}
	return false
}

// GetHTTPStatus returns the HTTP status code for an error.
// Returns 500 Internal Server Error if the error is not an AppError.
func GetHTTPStatus(err error) int {
	var appErr *AppError
	if errors.As(err, &appErr) {
		return appErr.HTTPStatus
	}
	return http.StatusInternalServerError
}

