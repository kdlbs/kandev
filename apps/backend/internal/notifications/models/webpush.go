package models

import "time"

// PushSubscription is one browser/device endpoint registered for Web Push. The
// endpoint is the push service URL and is unique per browser installation.
type PushSubscription struct {
	ID         string     `json:"id"`
	UserID     string     `json:"user_id"`
	Endpoint   string     `json:"endpoint"`
	P256dh     string     `json:"p256dh"`
	Auth       string     `json:"auth"`
	UserAgent  string     `json:"user_agent"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
}

// VAPIDKeys is the instance-wide application server key pair.
type VAPIDKeys struct {
	PrivateKey string
	PublicKey  string
}
