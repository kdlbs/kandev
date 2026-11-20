package webpush

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"

	"github.com/kandev/kandev/internal/notifications/providers"
)

// Event types that warrant an immediate, high-urgency push. They mirror the
// notification service constants; duplicating the strings keeps this package
// free of an import cycle with the service package.
const (
	eventPermissionRequested = "session.permission_requested"
	eventClarificationAsked  = "session.clarification_requested"
	eventSessionFailed       = "session.failed"
)

// Notifier is the part of Service the provider needs.
type Notifier interface {
	Notify(ctx context.Context, userID string, payload Payload, opts SendOptions) (int, error)
}

type queuedNotifier interface {
	Enqueue(ctx context.Context, userID string, payload Payload, opts SendOptions) error
}

// Provider adapts Web Push to the notification provider interface.
type Provider struct {
	notifier Notifier
}

var _ providers.Provider = (*Provider)(nil)

// NewProvider builds the provider adapter.
func NewProvider(notifier Notifier) *Provider {
	return &Provider{notifier: notifier}
}

func (p *Provider) Available() bool { return p.notifier != nil }

func (p *Provider) Validate(_ map[string]interface{}) error { return nil }

// Send pushes message to every device of message.UserID.
func (p *Provider) Send(ctx context.Context, message providers.Message) error {
	if p.notifier == nil {
		return errors.New("web push not available")
	}
	payload := Payload{
		Title:     message.Title,
		Body:      message.Body,
		TaskID:    message.TaskID,
		SessionID: message.TaskSessionID,
		URL:       TaskURL(message.TaskID, message.TaskSessionID),
		Tag:       payloadTag(message),
	}
	opts := SendOptions{Urgency: urgencyFor(message.EventType), Topic: topicFor(payload.Tag)}
	if queued, ok := p.notifier.(queuedNotifier); ok {
		return queued.Enqueue(ctx, message.UserID, payload, opts)
	}
	_, err := p.notifier.Notify(ctx, message.UserID, payload, opts)
	return err
}

// payloadTag collapses repeats of the same event on the same session into one
// notification on the device.
func payloadTag(message providers.Message) string {
	scope := message.TaskSessionID
	if scope == "" {
		scope = message.TaskID
	}
	if scope == "" {
		return "kandev-" + message.EventType
	}
	return "kandev-" + message.EventType + "-" + scope
}

func urgencyFor(eventType string) Urgency {
	switch eventType {
	case eventPermissionRequested, eventClarificationAsked, eventSessionFailed:
		return UrgencyHigh
	default:
		return UrgencyNormal
	}
}

// topicFor derives an RFC 8030 Topic (at most 32 URL-safe base64 characters)
// so the push service replaces a queued message for the same tag instead of
// delivering a backlog when a device comes online.
func topicFor(tag string) string {
	if tag == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(tag))
	return base64.RawURLEncoding.EncodeToString(sum[:])[:32]
}
