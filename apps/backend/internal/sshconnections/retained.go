package sshconnections

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strconv"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/sshtransport"
	"golang.org/x/crypto/ssh"
)

// DialRetainedLegacy translates the historical direct runtime evidence into a
// pinned hop. Historical jump selectors carry no bastion trust or resolved
// endpoint and therefore cannot authorize a reconnect.
func (s *Service) DialRetainedLegacy(ctx context.Context, target sshtransport.SSHTarget) (*ssh.Client, error) {
	if !s.enabled {
		return nil, ErrDisabled
	}
	if target.Host == "" || target.Port < 1 || target.Port > 65535 || target.User == "" ||
		target.PinnedFingerprint == "" || target.ProxyJump != "" || target.OriginalHost != "" {
		return nil, ErrInvalidRoute
	}
	auth, cleanup, err := retainedLegacyAuth(ctx, target)
	if err != nil {
		return nil, err
	}
	defer cleanup()
	return sshtransport.DialRoute(ctx, sshtransport.Hop{
		Host: target.Host, Port: target.Port, User: target.User, Fingerprint: target.PinnedFingerprint, Auth: auth,
	}, nil)
}

func retainedLegacyAuth(ctx context.Context, target sshtransport.SSHTarget) ([]ssh.AuthMethod, func(), error) {
	switch target.IdentitySource {
	case sshtransport.SSHIdentitySourceAgent:
		if filepath.IsAbs(target.IdentityAgent) {
			return agentAuthentication(ctx, target.IdentityAgent)
		}
	case sshtransport.SSHIdentitySourceFile:
		if filepath.IsAbs(target.IdentityFile) {
			return sshtransport.Authentication(&target)
		}
	}
	return nil, func() {}, ErrCredential
}

// OpenRetainedRoute holds a verified route for a self-contained cleanup job
// after the job's original runtime reference has been released.
func (s *Service) OpenRetainedRoute(ctx context.Context, snapshot RuntimeSnapshot) (*ssh.Client, func(), error) {
	if !s.enabled {
		return nil, func() {}, ErrDisabled
	}
	client, _, cleanup, err := s.openProbe(ctx, s.dialProtectedRoute, func(tx *sqlx.Tx) (Route, error) {
		expected := snapshot.Destination
		route, err := routeIn(ctx, tx, expected.ID)
		if err != nil {
			return Route{}, err
		}
		c := route.Destination
		if snapshot.Stamp == "" || snapshot.Stamp != route.Stamp || c.VerifiedRoute != route.Stamp ||
			expected.Host != c.Host || expected.Port != c.Port || expected.User != c.User || expected.Fingerprint != c.Fingerprint {
			return Route{}, ErrConflict
		}
		current, err := runtimeSnapshotIn(ctx, tx, "", route)
		if err != nil {
			return Route{}, err
		}
		if current.IdentityGeneration != snapshot.IdentityGeneration || current.BastionIdentityGeneration != snapshot.BastionIdentityGeneration {
			return Route{}, ErrConflict
		}
		return route, nil
	})
	return client, cleanup, err
}

// DialRetainedLegacySession accepts historical transport fields only when they
// agree with the original runtime inventory for the session.
func (s *Service) DialRetainedLegacySession(ctx context.Context, sessionID string, target sshtransport.SSHTarget) (*ssh.Client, error) {
	if !s.enabled {
		return nil, ErrDisabled
	}
	var data string
	query := s.store.db.Rebind(`SELECT metadata FROM executors_running WHERE session_id=? AND runtime='ssh'`)
	if err := s.store.db.GetContext(ctx, &data, query, sessionID); err != nil {
		return nil, ErrInvalidRoute
	}
	var metadata map[string]any
	if json.Unmarshal([]byte(data), &metadata) != nil || !matchesRetainedLegacyTarget(metadata, target) {
		return nil, ErrConflict
	}
	return s.DialRetainedLegacy(ctx, target)
}

func matchesRetainedLegacyTarget(metadata map[string]any, target sshtransport.SSHTarget) bool {
	value := func(key string) string {
		v, _ := metadata[key].(string)
		return v
	}
	return value(ConnectionConfigKey) == "" && value("ssh_host_alias") == "" &&
		value("ssh_host") == target.Host && value("ssh_port") == strconv.Itoa(target.Port) &&
		value("ssh_user") == target.User && value("ssh_host_fingerprint") == target.PinnedFingerprint &&
		value("ssh_identity_source") == string(target.IdentitySource) && value("ssh_identity_file") == target.IdentityFile &&
		value("ssh_proxy_jump") == target.ProxyJump && value("ssh_identity_agent") == target.IdentityAgent
}
