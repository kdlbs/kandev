package models

import "time"

type ProviderType string

const (
	ProviderTypeLocal   ProviderType = "local"
	ProviderTypeApprise ProviderType = "apprise"
)

type Provider struct {
	ID        string                 `json:"id"`
	UserID    string                 `json:"user_id"`
	Name      string                 `json:"name"`
	Type      ProviderType           `json:"type"`
	Config    map[string]interface{} `json:"config"`
	Enabled   bool                   `json:"enabled"`
	CreatedAt time.Time              `json:"created_at"`
	UpdatedAt time.Time              `json:"updated_at"`
}

type Subscription struct {
	ID         string    `json:"id"`
	UserID     string    `json:"user_id"`
	ProviderID string    `json:"provider_id"`
	EventType  string    `json:"event_type"`
	Enabled    bool      `json:"enabled"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type Delivery struct {
	ID            string    `json:"id"`
	UserID        string    `json:"user_id"`
	ProviderID    string    `json:"provider_id"`
	EventType     string    `json:"event_type"`
	TaskSessionID string    `json:"session_id"`
	CreatedAt     time.Time `json:"created_at"`
}
