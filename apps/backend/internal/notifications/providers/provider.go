package providers

import (
	"context"
	"errors"
)

var ErrNoEligibleSubscriber = errors.New("no eligible notification subscriber")

type Message struct {
	EventType     string
	Title         string
	Body          string
	TaskID        string
	TaskSessionID string
	OccurrenceID  string
	UserID        string
	Config        map[string]interface{}
	Payload       map[string]string
}

type Provider interface {
	Available() bool
	Validate(config map[string]interface{}) error
	Send(ctx context.Context, message Message) error
}
