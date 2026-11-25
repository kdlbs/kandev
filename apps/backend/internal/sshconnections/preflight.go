package sshconnections

import (
	"context"
	"errors"
)

type Diagnostic struct {
	Code string `json:"code"`
	Hop  string `json:"hop"`
}
type Preflight struct {
	Stamp  string       `json:"stamp"`
	Checks []Diagnostic `json:"checks"`
	Ready  bool         `json:"ready"`
}

func (s *Service) Preflight(ctx context.Context, id, stamp string) (Preflight, error) {
	if _, err := s.authorize(ctx); err != nil {
		return Preflight{}, err
	}
	if stamp == "" {
		return Preflight{}, ErrConflict
	}
	route, err := s.readRoute(ctx, id, stamp)
	if err != nil {
		return Preflight{}, err
	}
	out := Preflight{Stamp: route.Stamp, Checks: make([]Diagnostic, 0), Ready: route.Destination.VerifiedRoute == route.Stamp}
	out.Checks = append(out.Checks, s.checkHop(ctx, route.Destination, hopDestination)...)
	if route.Bastion != nil {
		out.Checks = append(out.Checks, s.checkHop(ctx, *route.Bastion, hopBastion)...)
		b, err := s.readRoute(ctx, route.Bastion.ID, "")
		if err != nil {
			return Preflight{}, err
		}
		if b.Destination.VerifiedRoute != b.Stamp {
			out.Checks = append(out.Checks, Diagnostic{Code: "ssh_bastion_not_ready", Hop: hopBastion})
		}
	}
	if len(out.Checks) > 0 {
		out.Ready = false
	}
	return out, nil
}
func (s *Service) checkHop(ctx context.Context, c Connection, name string) []Diagnostic {
	out := make([]Diagnostic, 0)
	add := func(code string) { out = append(out, Diagnostic{Code: code, Hop: name}) }
	if c.Host == "" || c.Port < 1 || c.User == "" {
		add("ssh_target_required")
	}
	if c.Fingerprint == "" {
		add("ssh_trust_required")
	}
	if c.IdentityID == "" {
		add("ssh_identity_required")
		return out
	}
	_, cleanup, err := s.hop(ctx, c)
	cleanup()
	if err != nil {
		code := "ssh_credential_unavailable"
		var failure *CredentialFailure
		if errors.As(err, &failure) {
			code = failure.Code
		}
		add(code)
	}
	return out
}
