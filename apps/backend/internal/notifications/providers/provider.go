package providers

import (
	"context"
)

type Message struct {
	EventType     string
	Title         string
	Body          string
	TaskID        string
	TaskSessionID string
	UserID        string
	Config        map[string]interface{}
}

type Provider interface {
	Available() bool
	Validate(config map[string]interface{}) error
	Send(ctx context.Context, message Message) error
}
