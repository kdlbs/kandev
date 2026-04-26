// Package controller provides request/response handling for the orchestrate domain.
package controller

import (
	"github.com/kandev/kandev/internal/orchestrate/service"
)

// Controller wraps the orchestrate service for HTTP handlers.
type Controller struct {
	Svc *service.Service
}

// NewController creates a new orchestrate controller.
func NewController(svc *service.Service) *Controller {
	return &Controller{Svc: svc}
}
