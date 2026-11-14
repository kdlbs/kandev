package service

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/google/uuid"
)

var ErrFormatterCaptureNotFound = errors.New("notification capture lease not found")

type FormatterCaptureLease struct {
	ID        string    `json:"id"`
	ExpiresAt time.Time `json:"expires_at"`
	owner     string
	release   func()
	timer     *time.Timer
}

type formatterCaptureLeases struct {
	mu       sync.Mutex
	current  *FormatterCaptureLease
	starting bool
	closed   bool
}

// StartFormatterCapture creates a bounded operator lease, not an unbounded
// request lock. It drains config mutations and freezes formatter installations.
func (s *Service) StartFormatterCapture(ctx context.Context, owner string) (*FormatterCaptureLease, error) {
	if owner == "" {
		return nil, ErrFormatterCaptureNotFound
	}
	state := &s.formatterLeases
	state.mu.Lock()
	if state.closed || state.starting || state.current != nil {
		state.mu.Unlock()
		return nil, ErrFormatterCapture
	}
	state.starting = true
	state.mu.Unlock()
	release, err := s.CaptureFormatterConfiguration(ctx)
	state.mu.Lock()
	defer state.mu.Unlock()
	state.starting = false
	if err != nil {
		return nil, err
	}
	if state.closed {
		release()
		return nil, ErrFormatterCapture
	}
	lease := &FormatterCaptureLease{ID: uuid.NewString(), ExpiresAt: time.Now().UTC().Add(5 * time.Minute), owner: owner, release: release}
	state.current = lease
	lease.timer = time.AfterFunc(5*time.Minute, func() { _ = s.ReleaseFormatterCapture(owner, lease.ID) })
	return &FormatterCaptureLease{ID: lease.ID, ExpiresAt: lease.ExpiresAt}, nil
}

func (s *Service) FormatterCapture(owner, id string) (*FormatterCaptureLease, error) {
	state := &s.formatterLeases
	state.mu.Lock()
	defer state.mu.Unlock()
	lease := state.current
	if lease == nil || lease.ID != id || lease.owner != owner || !time.Now().Before(lease.ExpiresAt) {
		return nil, ErrFormatterCaptureNotFound
	}
	return &FormatterCaptureLease{ID: lease.ID, ExpiresAt: lease.ExpiresAt}, nil
}

func (s *Service) ReleaseFormatterCapture(owner, id string) error {
	state := &s.formatterLeases
	state.mu.Lock()
	lease := state.current
	if lease == nil || lease.ID != id || lease.owner != owner {
		state.mu.Unlock()
		return ErrFormatterCaptureNotFound
	}
	state.current = nil
	state.mu.Unlock()
	lease.timer.Stop()
	lease.release()
	return nil
}

func (s *Service) CloseFormatterCaptures() {
	state := &s.formatterLeases
	state.mu.Lock()
	state.closed = true
	lease := state.current
	state.current = nil
	state.mu.Unlock()
	if lease != nil {
		lease.timer.Stop()
		lease.release()
	}
}
