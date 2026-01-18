package models

import (
	"encoding/json"
	"time"
)

type Editor struct {
	ID        string          `json:"id"`
	Type      string          `json:"type"`
	Name      string          `json:"name"`
	Kind      string          `json:"kind"`
	Command   string          `json:"command"`
	Scheme    string          `json:"scheme"`
	Config    json.RawMessage `json:"config"`
	Installed bool            `json:"installed"`
	Enabled   bool            `json:"enabled"`
	CreatedAt time.Time       `json:"created_at"`
	UpdatedAt time.Time       `json:"updated_at"`
}
