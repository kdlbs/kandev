package sandbox

import (
	"slices"
	"strings"
	"time"
)

const (
	StatusConfigured  = "configured"
	StatusStarting    = "starting"
	StatusActive      = "active"
	StatusFailed      = "failed"
	StatusUnknown     = "unknown"
	StatusStopped     = "stopped"
	HealthAvailable   = "available"
	HealthUnavailable = "unavailable"
	HealthUnknown     = "unknown"
)

// Observation separates current confinement evidence from provider availability.
type Observation struct {
	AttemptID      string     `json:"attempt_id"`
	Generation     uint64     `json:"generation"`
	AttemptVersion int64      `json:"attempt_version"`
	Status         string     `json:"status"`
	ProviderHealth string     `json:"provider_health"`
	Reason         string     `json:"reason"`
	Mechanism      string     `json:"mechanism,omitempty"`
	CheckedAt      time.Time  `json:"checked_at"`
	EvidenceAt     *time.Time `json:"evidence_at,omitempty"`
	ValidUntil     *time.Time `json:"valid_until,omitempty"`
}

func (o Observation) ValidFields() bool {
	if o.AttemptID == "" || len(o.AttemptID) > 128 || o.Generation == 0 || o.AttemptVersion < 1 || o.CheckedAt.IsZero() {
		return false
	}
	if !slices.Contains([]string{StatusConfigured, StatusStarting, StatusActive, StatusFailed, StatusUnknown, StatusStopped}, o.Status) || !slices.Contains([]string{HealthAvailable, HealthUnavailable, HealthUnknown}, o.ProviderHealth) {
		return false
	}
	if !slices.Contains([]string{"", "provider_unavailable", "ownership_unavailable", "evidence_unavailable", "launch_failed"}, o.Reason) {
		return false
	}
	if o.EvidenceAt != nil && (o.EvidenceAt.IsZero() || o.EvidenceAt.After(o.CheckedAt) || !ValidMechanismKey(o.Mechanism)) {
		return false
	}
	if o.EvidenceAt == nil && o.Mechanism != "" {
		return false
	}
	return o.validEvidenceWindow()
}

func (o Observation) validEvidenceWindow() bool {
	if o.Status != StatusActive {
		return o.ValidUntil == nil
	}
	return o.Reason == "" && o.EvidenceAt != nil && o.ValidUntil != nil && o.ValidUntil.Equal(o.EvidenceAt.Add(30*time.Second)) && o.CheckedAt.Before(*o.ValidUntil)
}

// Fresh never extends a timestamp supplied by the evidence producer.
func (o Observation) Fresh(now time.Time) bool {
	if o.Status != StatusActive {
		return true
	}
	return o.ValidUntil != nil && !o.CheckedAt.After(now) && now.Before(*o.ValidUntil)
}

func ValidMechanismKey(key string) bool {
	return key != "" && len(key) <= 64 && !strings.ContainsFunc(key, func(r rune) bool {
		valid := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '.' || r == '_' || r == '-'
		return !valid
	})
}
