// Package webpush delivers notification-provider messages to browsers through
// the Web Push protocol (RFC 8030 with VAPID, RFC 8292). Subscriptions are
// stored per user and device; the VAPID key pair is generated once and kept in
// the same database as the rest of the notification state.
package webpush

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
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

var defaultPushHostSuffixes = []string{
	"fcm.googleapis.com",
	"push.services.mozilla.com",
	"push.apple.com",
	"notify.windows.com",
}

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
	// ErrDeliveryQueueFull reports that asynchronous push delivery is at capacity.
	ErrDeliveryQueueFull = errors.New("push delivery queue is full")
	// ErrDeliveryQueueUnavailable reports that asynchronous delivery is not started.
	ErrDeliveryQueueUnavailable = errors.New("push delivery queue is not started")
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
	deliveryQueueMu sync.RWMutex
	deliveryQueue   *deliveryQueue
	repo            store.WebPushRepository
	sender          Sender
	subject         string
	allowedHosts    []string
	logger          *logger.Logger
}

// StartAsyncDelivery starts the bounded queue used by notification-provider
// events. Its cleanup drains accepted jobs before the backend closes storage.
func (s *Service) StartAsyncDelivery() func() error {
	queue := newDeliveryQueue(s)
	s.deliveryQueueMu.Lock()
	if s.deliveryQueue != nil {
		existing := s.deliveryQueue
		s.deliveryQueueMu.Unlock()
		_ = queue.Close()
		return existing.Close
	}
	s.deliveryQueue = queue
	s.deliveryQueueMu.Unlock()
	return queue.Close
}

// Enqueue accepts provider notifications for background delivery. The direct
// Notify method remains synchronous for the user-triggered test endpoint.
func (s *Service) Enqueue(ctx context.Context, userID string, payload Payload, opts SendOptions) error {
	s.deliveryQueueMu.RLock()
	queue := s.deliveryQueue
	s.deliveryQueueMu.RUnlock()
	if queue == nil {
		return ErrDeliveryQueueUnavailable
	}
	return queue.Enqueue(ctx, userID, payload, opts)
}

// NewService builds the service. An empty subject falls back to DefaultSubject.
// Extra allowed host suffixes are for operators who use a private push service.
func NewService(repo store.WebPushRepository, sender Sender, subject string, log *logger.Logger, allowedHosts ...string) *Service {
	return &Service{
		repo:         repo,
		sender:       sender,
		subject:      NormalizeSubject(subject),
		allowedHosts: mergePushHostSuffixes(allowedHosts),
		logger:       log.WithFields(zap.String("component", "webpush")),
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
	if err := validateSubscriptionForHosts(in, s.allowedHosts); err != nil {
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
		accepted, err := s.sendOne(ctx, sub, body, keys, opts)
		if err != nil {
			lastErr = err
			continue
		}
		if accepted {
			delivered++
		}
	}
	if delivered == 0 && lastErr != nil {
		return 0, lastErr
	}
	return delivered, nil
}

func (s *Service) sendOne(ctx context.Context, sub *models.PushSubscription, body []byte, keys models.VAPIDKeys, opts SendOptions) (bool, error) {
	status, err := s.sender.Send(ctx, sub, body, keys, s.subject, opts)
	switch {
	case status == http.StatusNotFound || status == http.StatusGone:
		if delErr := s.repo.DeletePushSubscriptionByEndpoint(ctx, sub.Endpoint); delErr != nil {
			s.logger.Warn("failed to prune expired push subscription", zap.Error(delErr))
		}
		return false, nil
	case err != nil:
		s.logger.Warn("push delivery failed", zap.Int("status", status), zap.Error(err))
		return false, err
	case status < 200 || status >= 300:
		return false, fmt.Errorf("push service responded %d", status)
	}
	if touchErr := s.repo.TouchPushSubscription(ctx, sub.Endpoint, time.Now()); touchErr != nil {
		s.logger.Debug("failed to record push delivery time", zap.Error(touchErr))
	}
	return true, nil
}

func validateSubscriptionForHosts(in SubscribeInput, allowedHosts []string) error {
	if err := validatePushEndpoint(in.Endpoint, allowedHosts); err != nil {
		return err
	}
	if in.P256dh == "" || in.Auth == "" || len(in.P256dh) > maxKeyLen || len(in.Auth) > maxKeyLen {
		return fmt.Errorf("%w: p256dh and auth keys are required", ErrInvalidSubscription)
	}
	return nil
}

func validatePushEndpoint(raw string, allowedHosts []string) error {
	endpoint, err := url.Parse(raw)
	if err != nil || !strings.EqualFold(endpoint.Scheme, "https") || endpoint.Host == "" || len(raw) > maxEndpointLen {
		return fmt.Errorf("%w: endpoint must be an https URL", ErrInvalidSubscription)
	}
	if endpoint.User != nil || endpoint.Fragment != "" || !isAllowedPushHost(endpoint, allowedHosts) {
		return fmt.Errorf("%w: endpoint host is not an allowed push service", ErrInvalidSubscription)
	}
	return nil
}

func mergePushHostSuffixes(extra []string) []string {
	allowed := make([]string, 0, len(defaultPushHostSuffixes)+len(extra))
	allowed = append(allowed, defaultPushHostSuffixes...)
	for _, host := range extra {
		host = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(host)), ".")
		if host == "" || strings.ContainsAny(host, "/:@?#[]") || net.ParseIP(host) != nil {
			continue
		}
		allowed = append(allowed, host)
	}
	return allowed
}

func isAllowedPushHost(endpoint *url.URL, allowedHosts []string) bool {
	if endpoint == nil || endpoint.Port() != "" && endpoint.Port() != "443" {
		return false
	}
	host := strings.TrimSuffix(strings.ToLower(endpoint.Hostname()), ".")
	if host == "" || net.ParseIP(host) != nil {
		return false
	}
	for _, suffix := range allowedHosts {
		if host == suffix || strings.HasSuffix(host, "."+suffix) {
			return true
		}
	}
	return false
}
