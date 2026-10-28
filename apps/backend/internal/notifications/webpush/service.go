// Package webpush delivers notification-provider messages to browsers through
// the Web Push protocol (RFC 8030 with VAPID, RFC 8292). Subscriptions are
// stored per user and device; the VAPID key pair is generated once and kept in
// the same database as the rest of the notification state.
package webpush

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/notifications/models"
	"github.com/kandev/kandev/internal/notifications/store"
)

const (
	// DefaultSubject is the VAPID "sub" claim used when none is configured.
	// Push services use it only to contact the operator about abusive senders.
	DefaultSubject = "mailto:admin@localhost.invalid"

	// DefaultTTL is how long a push service may hold a message for an offline
	// device. A stale "agent finished" alert is noise, so keep it short.
	DefaultTTL = 6 * time.Hour

	maxEndpointLen  = 2048
	maxKeyLen       = 256
	maxUserAgentLen = 512
)

// Urgency maps to the RFC 8030 Urgency header.
type Urgency string

const (
	UrgencyNormal Urgency = "normal"
	UrgencyHigh   Urgency = "high"
)

var (
	// ErrInvalidSubscription reports a malformed or unacceptable subscription.
	ErrInvalidSubscription = errors.New("invalid push subscription")
	// ErrNotFound reports that the caller has no subscription for an endpoint.
	ErrNotFound = errors.New("push subscription not found")
)

// Payload is the JSON document the service worker receives. It carries titles
// and short status only: no code, no tokens, no message bodies.
type Payload struct {
	Title     string `json:"title"`
	Body      string `json:"body"`
	Tag       string `json:"tag,omitempty"`
	URL       string `json:"url,omitempty"`
	TaskID    string `json:"taskId,omitempty"`
	SessionID string `json:"sessionId,omitempty"`
}

// SendOptions control delivery of a single push message.
type SendOptions struct {
	TTL     time.Duration
	Urgency Urgency
	Topic   string
}

// Sender performs one HTTP push to a subscription. The production
// implementation talks to the browser vendor's push service; tests supply a
// fake. It returns the push service's HTTP status code, or 0 on a transport
// failure.
type Sender interface {
	Send(ctx context.Context, sub *models.PushSubscription, body []byte, keys models.VAPIDKeys, subject string, opts SendOptions) (int, error)
}

// Service owns subscription management and fan-out to a user's devices.
type Service struct {
	repo    store.WebPushRepository
	sender  Sender
	subject string
	logger  *logger.Logger
}

// NewService builds the service. An empty subject falls back to DefaultSubject.
func NewService(repo store.WebPushRepository, sender Sender, subject string, log *logger.Logger) *Service {
	return &Service{
		repo:    repo,
		sender:  sender,
		subject: NormalizeSubject(subject),
		logger:  log.WithFields(zap.String("component", "webpush")),
	}
}

// NormalizeSubject returns a valid VAPID subject: a mailto: or https: URL.
func NormalizeSubject(subject string) string {
	subject = strings.TrimSpace(subject)
	switch {
	case subject == "":
		return DefaultSubject
	case strings.HasPrefix(subject, "mailto:"), strings.HasPrefix(subject, "https://"):
		return subject
	case strings.Contains(subject, "@"):
		return "mailto:" + subject
	default:
		return DefaultSubject
	}
}

// PublicKey returns the base64url VAPID application server key, generating
// and persisting the pair on first use.
func (s *Service) PublicKey(ctx context.Context) (string, error) {
	keys, err := s.repo.LoadOrCreateVAPIDKeys(ctx, generateVAPIDKeys)
	if err != nil {
		return "", err
	}
	return keys.PublicKey, nil
}

// SubscribeInput is the validated shape of a browser PushSubscription.
type SubscribeInput struct {
	Endpoint  string
	P256dh    string
	Auth      string
	UserAgent string
}

// Subscribe registers or refreshes a device for userID.
func (s *Service) Subscribe(ctx context.Context, userID string, in SubscribeInput) (*models.PushSubscription, error) {
	if err := validateSubscription(in); err != nil {
		return nil, err
	}
	ua := in.UserAgent
	if len(ua) > maxUserAgentLen {
		ua = ua[:maxUserAgentLen]
	}
	sub := &models.PushSubscription{
		UserID:    userID,
		Endpoint:  in.Endpoint,
		P256dh:    in.P256dh,
		Auth:      in.Auth,
		UserAgent: ua,
	}
	if err := s.repo.UpsertPushSubscription(ctx, sub); err != nil {
		return nil, err
	}
	return sub, nil
}

// Unsubscribe removes the caller's subscription for endpoint.
func (s *Service) Unsubscribe(ctx context.Context, userID, endpoint string) error {
	removed, err := s.repo.DeletePushSubscription(ctx, userID, endpoint)
	if err != nil {
		return err
	}
	if !removed {
		return ErrNotFound
	}
	return nil
}

// Subscriptions lists the caller's registered devices.
func (s *Service) Subscriptions(ctx context.Context, userID string) ([]*models.PushSubscription, error) {
	return s.repo.ListPushSubscriptions(ctx, userID)
}

// Notify pushes payload to every device userID has registered. It returns how
// many devices accepted the message. Subscriptions the push service reports as
// gone (404/410) are pruned. An error is returned only when there was at least
// one device and none could be reached, so the caller may retry.
func (s *Service) Notify(ctx context.Context, userID string, payload Payload, opts SendOptions) (int, error) {
	subs, err := s.repo.ListPushSubscriptions(ctx, userID)
	if err != nil {
		return 0, err
	}
	if len(subs) == 0 {
		return 0, nil
	}
	body, err := encodePayload(payload)
	if err != nil {
		return 0, err
	}
	keys, err := s.repo.LoadOrCreateVAPIDKeys(ctx, generateVAPIDKeys)
	if err != nil {
		return 0, err
	}
	if opts.TTL <= 0 {
		opts.TTL = DefaultTTL
	}
	if opts.Urgency == "" {
		opts.Urgency = UrgencyNormal
	}
	delivered := 0
	var lastErr error
	for _, sub := range subs {
		if err := s.sendOne(ctx, sub, body, keys, opts); err != nil {
			lastErr = err
			continue
		}
		delivered++
	}
	if delivered == 0 && lastErr != nil {
		return 0, lastErr
	}
	return delivered, nil
}

func (s *Service) sendOne(ctx context.Context, sub *models.PushSubscription, body []byte, keys models.VAPIDKeys, opts SendOptions) error {
	status, err := s.sender.Send(ctx, sub, body, keys, s.subject, opts)
	switch {
	case status == http.StatusNotFound || status == http.StatusGone:
		if delErr := s.repo.DeletePushSubscriptionByEndpoint(ctx, sub.Endpoint); delErr != nil {
			s.logger.Warn("failed to prune expired push subscription", zap.Error(delErr))
		}
		return nil
	case err != nil:
		s.logger.Warn("push delivery failed", zap.Int("status", status), zap.Error(err))
		return err
	case status < 200 || status >= 300:
		return fmt.Errorf("push service responded %d", status)
	}
	if touchErr := s.repo.TouchPushSubscription(ctx, sub.Endpoint, time.Now()); touchErr != nil {
		s.logger.Debug("failed to record push delivery time", zap.Error(touchErr))
	}
	return nil
}

func validateSubscription(in SubscribeInput) error {
	u, err := url.Parse(in.Endpoint)
	if err != nil || u.Scheme != "https" || u.Host == "" || len(in.Endpoint) > maxEndpointLen {
		return fmt.Errorf("%w: endpoint must be an https URL", ErrInvalidSubscription)
	}
	if in.P256dh == "" || in.Auth == "" || len(in.P256dh) > maxKeyLen || len(in.Auth) > maxKeyLen {
		return fmt.Errorf("%w: p256dh and auth keys are required", ErrInvalidSubscription)
	}
	return nil
}
