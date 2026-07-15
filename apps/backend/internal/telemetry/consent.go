package telemetry

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// ConsentStatus is the tri-state opt-in record. "unasked" drives the
// one-time onboarding prompt; only "granted" ever allows emission.
type ConsentStatus string

const (
	ConsentUnasked ConsentStatus = "unasked"
	ConsentGranted ConsentStatus = "granted"
	ConsentDenied  ConsentStatus = "denied"
)

// Settings keys on the install-wide settings table.
const (
	consentKey          = "telemetry.consent"
	installIDKey        = "telemetry.install_id"
	consentUpdatedAtKey = "telemetry.consent_updated_at"
)

// SettingsStore is the slice of internal/system/settings.Store the
// service needs; narrowed to an interface so tests can use an in-memory
// fake without a database pool.
type SettingsStore interface {
	Get(ctx context.Context, key string) ([]byte, bool, error)
	Save(ctx context.Context, key string, value []byte) error
}

// ConsentState is the wire shape of GET/PUT /api/v1/telemetry/consent.
type ConsentState struct {
	Status      ConsentStatus `json:"status"`
	InstallID   string        `json:"install_id,omitempty"`
	EnvDisabled bool          `json:"env_disabled"`
}

// loadConsent reads the persisted consent + install ID into the service's
// in-memory copies. Missing rows mean "unasked".
func (s *Service) loadConsent(ctx context.Context) error {
	status := ConsentUnasked
	raw, found, err := s.store.Get(ctx, consentKey)
	if err != nil {
		return fmt.Errorf("load telemetry consent: %w", err)
	}
	if found {
		switch ConsentStatus(raw) {
		case ConsentGranted, ConsentDenied:
			status = ConsentStatus(raw)
		}
	}
	installID := ""
	if raw, found, err = s.store.Get(ctx, installIDKey); err == nil && found {
		installID = string(raw)
	}
	s.mu.Lock()
	s.consent = status
	s.installID = installID
	s.mu.Unlock()
	return nil
}

// Consent returns the current consent state for the HTTP surface.
func (s *Service) Consent() ConsentState {
	s.mu.Lock()
	defer s.mu.Unlock()
	return ConsentState{Status: s.consent, InstallID: s.installID, EnvDisabled: s.opts.EnvDisabled}
}

// SetConsent persists a grant or denial. Granting mints the anonymous
// install UUID (only then — never before consent) and emits an immediate
// heartbeat so opted-in installs register without waiting a day. Denying
// clears the install ID and drops anything still queued.
func (s *Service) SetConsent(ctx context.Context, status ConsentStatus) (ConsentState, error) {
	if status != ConsentGranted && status != ConsentDenied {
		return ConsentState{}, fmt.Errorf("invalid consent status %q", status)
	}
	installID := ""
	if status == ConsentGranted {
		installID = s.currentOrNewInstallID()
	}
	if err := s.persistConsent(ctx, status, installID); err != nil {
		return ConsentState{}, err
	}
	s.mu.Lock()
	s.consent = status
	s.installID = installID
	s.mu.Unlock()

	if status == ConsentGranted {
		s.enqueue(Event{Name: EventTelemetryEnabled})
		s.enqueue(Event{Name: EventInstallHeartbeat})
	} else {
		s.drainQueue()
	}
	return s.Consent(), nil
}

func (s *Service) currentOrNewInstallID() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.installID != "" {
		return s.installID
	}
	return uuid.New().String()
}

func (s *Service) persistConsent(ctx context.Context, status ConsentStatus, installID string) error {
	if err := s.store.Save(ctx, consentKey, []byte(status)); err != nil {
		return fmt.Errorf("save telemetry consent: %w", err)
	}
	if err := s.store.Save(ctx, installIDKey, []byte(installID)); err != nil {
		return fmt.Errorf("save telemetry install id: %w", err)
	}
	timestamp := time.Now().UTC().Format(time.RFC3339)
	if err := s.store.Save(ctx, consentUpdatedAtKey, []byte(timestamp)); err != nil {
		return fmt.Errorf("save telemetry consent timestamp: %w", err)
	}
	return nil
}
