package sandbox

import (
	"context"
	"reflect"
	"time"
)

// Availability describes a checked installation, never observed confinement.
type Availability struct {
	Available bool   `json:"available"`
	Blocker   string `json:"blocker,omitempty"`
}

func (s *Service) SetAvailabilityCheck(check func(context.Context, *Resolved) error) {
	s.availabilityCheck = check
}

func (s *Service) CheckAvailability(ctx context.Context, request PreviewContext) (*Availability, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	preview, err := s.PreviewSelection(ctx, request)
	if err != nil {
		return nil, err
	}
	if preview.ResolutionError != "" {
		return &Availability{Blocker: preview.ResolutionError}, nil
	}
	if preview.Resolved == nil {
		return &Availability{Blocker: "sandbox_policy_not_selected"}, nil
	}
	if s.availabilityCheck == nil || s.availabilityCheck(ctx, preview.Resolved) != nil || ctx.Err() != nil {
		return &Availability{Blocker: "sandbox_runtime_unavailable"}, nil
	}
	current, err := s.PreviewSelection(ctx, request)
	if err != nil {
		return nil, err
	}
	if current.ResolutionError != "" || !reflect.DeepEqual(preview.Resolved, current.Resolved) {
		return nil, ErrConflict
	}
	return &Availability{Available: true}, nil
}
