package sshconnections

import (
	"context"
	"errors"
	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/sshtransport"
	"strings"
	"time"
)

// ProbeFailure contains only stable, public diagnostics, never transport output.
type ProbeFailure struct {
	Code string
	Hop  string
}

func (e *ProbeFailure) Error() string { return e.Code }
func transportFailure(err error) *ProbeFailure {
	var reported *ProbeFailure
	if errors.As(err, &reported) {
		return &ProbeFailure{Code: reported.Code, Hop: reported.Hop}
	}
	out := &ProbeFailure{Code: "ssh_connection_failed", Hop: hopDestination}
	var hop *sshtransport.HopError
	if errors.As(err, &hop) {
		out.Hop = hop.Hop
	}
	var changed *sshtransport.HostKeyMismatchError
	var credential *CredentialFailure
	switch {
	case errors.As(err, &credential):
		out.Code = credential.Code
	case errors.As(err, &changed):
		out.Code = codeHostKeyChanged
	case strings.Contains(err.Error(), "unable to authenticate"):
		out.Code = "ssh_authentication_rejected"
	}
	return out
}
func (s *Service) authenticateRoute(ctx context.Context, route Route) error {
	if route.Destination.Fingerprint == "" {
		return &ProbeFailure{Code: ErrTrust.Error(), Hop: hopDestination}
	}
	b, cleanup, err := s.bastionHop(ctx, route)
	if err != nil {
		return &ProbeFailure{Code: ErrBastionNotReady.Error(), Hop: hopBastion}
	}
	defer cleanup()
	d, cleanup, err := s.hop(ctx, route.Destination)
	if err != nil {
		return &ProbeFailure{Code: ErrCredential.Error(), Hop: hopDestination}
	}
	defer cleanup()
	client, err := sshtransport.DialRoute(ctx, d, b)
	if err != nil {
		return transportFailure(err)
	}
	_ = client.Close()
	return nil
}
func (s *Service) recordProbe(ctx context.Context, actor string, route Route, probeErr error) (Route, error) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	err := s.store.write(ctx, func(tx *sqlx.Tx) error {
		current, err := routeIn(ctx, tx, route.Destination.ID)
		if err != nil {
			return err
		}
		if current.Stamp != route.Stamp {
			return ErrConflict
		}
		result := outcomeVerified
		current.Destination.VerifiedRoute = current.Stamp
		if probeErr != nil {
			current.Destination.VerifiedRoute = ""
			result = outcomeFailed
		}
		current.Destination.Version++
		if err = putConnection(ctx, tx, current.Destination); err != nil {
			return err
		}
		if err = audit(ctx, tx, actor, current.Destination.ID, "connection.test", result); err != nil {
			return err
		}
		route = current
		return nil
	})
	return route, err
}
