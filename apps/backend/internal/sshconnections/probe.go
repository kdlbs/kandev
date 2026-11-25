package sshconnections

import (
	"context"
	"database/sql"
	"errors"
	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/sshtransport"
	"golang.org/x/crypto/ssh"
	"time"
)

var ErrCredential = errors.New("ssh_credential_unavailable")
var ErrBastionNotReady = errors.New("ssh_bastion_not_ready")
var ErrConnectionFailed = errors.New("ssh_connection_failed")

func (s *Service) readRoute(ctx context.Context, id, stamp string) (Route, error) {
	tx, err := s.store.db.BeginTxx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return Route{}, err
	}
	defer func() { _ = tx.Rollback() }()
	route, err := routeIn(ctx, tx, id)
	if err != nil {
		return Route{}, err
	}
	if stamp != "" && route.Stamp != stamp {
		return Route{}, ErrConflict
	}
	return route, nil
}
func hopFromConnection(c Connection) sshtransport.Hop {
	hop := sshtransport.Hop{Host: c.Host, Port: c.Port, User: c.User, Fingerprint: c.Fingerprint}
	if key, _, _, _, err := ssh.ParseAuthorizedKey([]byte(c.PublicKey)); err == nil && ssh.FingerprintSHA256(key) == c.Fingerprint {
		hop.HostKeyTypes = []string{key.Type()}
	}
	return hop
}

func (s *Service) hop(ctx context.Context, c Connection) (sshtransport.Hop, func(), error) {
	empty := func() {}
	hop := hopFromConnection(c)
	tx, err := s.store.db.BeginTxx(ctx, nil)
	if err != nil {
		return hop, empty, err
	}
	defer func() { _ = tx.Rollback() }()
	identity, err := identityIn(ctx, tx, c.IdentityID)
	if err != nil {
		return hop, empty, &CredentialFailure{Code: "ssh_identity_required"}
	}
	if identity.Kind == identityManaged {
		key, err := s.vault.Reveal(ctx, tx, "ssh-identity:"+identity.ID)
		if err != nil {
			return hop, empty, &CredentialFailure{Code: codeManagedKeyUnavailable}
		}
		defer clear(key)
		signer, err := ssh.ParsePrivateKey(key)
		if err != nil {
			return hop, empty, &CredentialFailure{Code: codeManagedKeyUnavailable}
		}
		hop.Auth = []ssh.AuthMethod{ssh.PublicKeys(signer)}
		return hop, empty, nil
	}
	if identity.Kind == identityAgent {
		auth, cleanup, err := agentAuthentication(ctx, identity.Reference)
		hop.Auth = auth
		return hop, cleanup, err
	}

	target := &sshtransport.SSHTarget{IdentitySource: sshtransport.SSHIdentitySource(identity.Kind), IdentityFile: identity.Reference, IdentityAgent: identity.Reference}
	auth, cleanup, err := sshtransport.Authentication(target)
	if err != nil {
		cleanup()
		return hop, empty, &CredentialFailure{Code: codeKeyFileUnavailable}
	}
	hop.Auth = auth
	return hop, cleanup, nil
}
func (s *Service) bastionHop(ctx context.Context, route Route) (*sshtransport.Hop, func(), error) {
	if route.Bastion == nil {
		return nil, func() {}, nil
	}
	bastion, err := s.readRoute(ctx, route.Bastion.ID, "")
	if err != nil {
		return nil, func() {}, err
	}
	if bastion.Destination.VerifiedRoute != bastion.Stamp || bastion.Destination.Revision != route.Bastion.Revision {
		return nil, func() {}, ErrBastionNotReady
	}
	hop, cleanup, err := s.hop(ctx, *route.Bastion)
	return &hop, cleanup, err
}
func (s *Service) Observe(ctx context.Context, id, stamp string) (Observation, error) {
	actor, err := s.authorize(ctx)
	if err != nil {
		return Observation{}, err
	}
	if stamp == "" {
		return Observation{}, ErrConflict
	}
	route, err := s.readRoute(ctx, id, stamp)
	if err != nil {
		return Observation{}, err
	}
	b, cleanup, err := s.bastionHop(ctx, route)
	if err != nil {
		return Observation{}, err
	}
	defer cleanup()
	key, err := sshtransport.Observe(ctx, hopFromConnection(route.Destination), b)
	if err != nil {
		return Observation{}, transportFailure(err)
	}
	if _, err = s.readRoute(ctx, id, route.Stamp); err != nil {
		return Observation{}, err
	}
	observation := Observation{Receipt: uuid.NewString(), Fingerprint: ssh.FingerprintSHA256(key), PublicKey: string(ssh.MarshalAuthorizedKey(key)), Stamp: route.Stamp}
	s.receiptsMu.Lock()
	defer s.receiptsMu.Unlock()
	if s.receipts == nil {
		s.receipts = make(map[string]receipt)
	}
	now := time.Now()
	for token, r := range s.receipts {
		if !now.Before(r.expires) {
			delete(s.receipts, token)
		}
	}
	if len(s.receipts) >= 1024 {
		return Observation{}, ErrConflict
	}
	s.receipts[observation.Receipt] = receipt{actor: actor, connection: id, observation: observation, expires: now.Add(5 * time.Minute)}
	return observation, nil
}
func (s *Service) TestConnection(ctx context.Context, id, stamp string) (Route, error) {
	actor, err := s.authorize(ctx)
	if err != nil {
		return Route{}, err
	}
	if stamp == "" {
		return Route{}, ErrConflict
	}
	route, err := s.readRoute(ctx, id, stamp)
	if err != nil {
		return Route{}, err
	}
	probeErr := s.authenticateRoute(ctx, route)
	route, err = s.recordProbe(ctx, actor, route, probeErr)
	if err != nil {
		return Route{}, err
	}
	return route, probeErr
}
