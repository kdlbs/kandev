package process

import (
	"errors"
	"sync"
	"time"
)

var ErrProviderReplacementPending = errors.New("provider replacement is in progress")

type providerAdmission struct {
	mu          sync.Mutex
	active      int
	operationID string
	expiresAt   time.Time
}

// BeginProviderOperation shares provider admission across transports. It never
// closes instance admission or cancels a previously admitted operation.
func (m *Manager) BeginProviderOperation(operationID string) (func(), error) {
	m.providerAdmission.mu.Lock()
	defer m.providerAdmission.mu.Unlock()
	if current := m.providerAdmission.operationID; (current != "" || operationID != "") && operationID != current {
		return nil, ErrProviderReplacementPending
	}
	m.providerAdmission.active++
	return sync.OnceFunc(func() { m.providerAdmission.mu.Lock(); m.providerAdmission.active--; m.providerAdmission.mu.Unlock() }), nil
}

func (m *Manager) ReserveProviderReplacement(operationID string, expiresAt time.Time) error {
	if operationID == "" || !expiresAt.After(time.Now()) {
		return errors.New("provider replacement reservation is invalid")
	}
	m.providerAdmission.mu.Lock()
	defer m.providerAdmission.mu.Unlock()
	if m.providerAdmission.operationID == operationID {
		return nil
	}
	if m.providerAdmission.operationID != "" || m.providerAdmission.active != 0 {
		return ErrProviderReplacementPending
	}
	m.providerAdmission.operationID = operationID
	m.providerAdmission.expiresAt = expiresAt
	return nil
}

// ReleaseProviderReplacement requires the operation owner. The durable protocol
// permits release only before retirement or after accepted activation.
func (m *Manager) ReleaseProviderReplacement(operationID string) {
	m.providerAdmission.mu.Lock()
	defer m.providerAdmission.mu.Unlock()
	if m.providerAdmission.operationID == operationID {
		m.providerAdmission.operationID = ""
		m.providerAdmission.expiresAt = time.Time{}
	}
}

// TakeoverProviderReplacement transfers closed admission between durable attempts.
func (m *Manager) TakeoverProviderReplacement(previous, next string, expiry time.Time) error {
	m.providerAdmission.mu.Lock()
	defer m.providerAdmission.mu.Unlock()
	if previous == "" || next == "" || previous == next || !expiry.After(time.Now()) || m.providerAdmission.active != 0 {
		return ErrProviderReplacementPending
	}
	if m.providerAdmission.operationID != previous && m.providerAdmission.operationID != next {
		return ErrProviderReplacementPending
	}
	m.providerAdmission.operationID = next
	m.providerAdmission.expiresAt = expiry
	return nil
}
