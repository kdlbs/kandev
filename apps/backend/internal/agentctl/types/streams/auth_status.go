package streams

import "time"

// AuthStatus is a transient authentication snapshot for a provider-managed
// sign-in attempt. It is never journaled as a task event.
type AuthStatus struct {
	AttemptID        string    `json:"attempt_id,omitempty"`
	State            string    `json:"state"`
	ExpiresAt        time.Time `json:"expires_at,omitempty"`
	AuthorizationURL string    `json:"authorization_url,omitempty"`
	ErrorCode        string    `json:"error_code,omitempty"`
	Error            string    `json:"error,omitempty"`
}
