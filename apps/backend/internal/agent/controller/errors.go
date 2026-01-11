package controller

import "errors"

var (
	// ErrLifecycleManagerNotAvailable is returned when lifecycle manager is nil
	ErrLifecycleManagerNotAvailable = errors.New("agent lifecycle manager not available")

	// ErrRegistryNotAvailable is returned when registry is nil
	ErrRegistryNotAvailable = errors.New("agent registry not available")

	// ErrAgentNotFound is returned when agent is not found
	ErrAgentNotFound = errors.New("agent not found")
)

