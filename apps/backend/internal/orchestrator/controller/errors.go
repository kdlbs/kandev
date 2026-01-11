package controller

import "errors"

var (
	// ErrACPHandlerNotConfigured is returned when ACP handler is not set
	ErrACPHandlerNotConfigured = errors.New("ACP handler not configured")
)

