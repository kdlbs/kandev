package bus

import "github.com/nats-io/nats.go"

// natsSubscription wraps a NATS subscription to implement the Subscription interface
type natsSubscription struct {
	sub *nats.Subscription
}

// Unsubscribe removes the subscription from the server
func (s *natsSubscription) Unsubscribe() error {
	if s.sub == nil {
		return nil
	}
	return s.sub.Unsubscribe()
}

// IsValid returns whether the subscription is still active
func (s *natsSubscription) IsValid() bool {
	if s.sub == nil {
		return false
	}
	return s.sub.IsValid()
}

