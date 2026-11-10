package webpush

import (
	"context"
	"net/http"
	"time"

	webpushlib "github.com/SherClockHolmes/webpush-go"

	"github.com/kandev/kandev/internal/notifications/models"
)

// sendTimeout bounds one push request so a stalled push service cannot hold up
// the notification fan-out.
const sendTimeout = 15 * time.Second

// HTTPSender is the production Sender. It uses the process's standard HTTP
// transport, so the usual HTTPS_PROXY environment variables apply and no
// inbound connectivity is needed.
type HTTPSender struct {
	client                 *http.Client
	allowedHosts           []string
	allowUntrustedEndpoint bool
}

// NewHTTPSender builds the production sender. Extra allowed host suffixes are
// for operators who use a private push service.
func NewHTTPSender(allowedHosts ...string) *HTTPSender {
	return &HTTPSender{
		client: &http.Client{
			Timeout: sendTimeout,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
		allowedHosts: mergePushHostSuffixes(allowedHosts),
	}
}

// Send encrypts body for sub and posts it to the subscription's push service.
func (s *HTTPSender) Send(ctx context.Context, sub *models.PushSubscription, body []byte, keys models.VAPIDKeys, subject string, opts SendOptions) (int, error) {
	if !s.allowUntrustedEndpoint {
		if err := validatePushEndpoint(sub.Endpoint, s.allowedHosts); err != nil {
			return 0, err
		}
	}
	resp, err := webpushlib.SendNotificationWithContext(ctx, body, &webpushlib.Subscription{
		Endpoint: sub.Endpoint,
		Keys:     webpushlib.Keys{P256dh: sub.P256dh, Auth: sub.Auth},
	}, &webpushlib.Options{
		HTTPClient:      s.client,
		Subscriber:      subject,
		VAPIDPublicKey:  keys.PublicKey,
		VAPIDPrivateKey: keys.PrivateKey,
		TTL:             int(opts.TTL / time.Second),
		Urgency:         webpushlib.Urgency(opts.Urgency),
		Topic:           opts.Topic,
	})
	if err != nil {
		return 0, err
	}
	defer func() { _ = resp.Body.Close() }()
	return resp.StatusCode, nil
}
