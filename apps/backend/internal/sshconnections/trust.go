package sshconnections

import (
	"context"
	"errors"
	"github.com/jmoiron/sqlx"
	"time"
)

var ErrTrust = errors.New("ssh_trust_required")

type Observation struct {
	Receipt     string `json:"receipt"`
	Fingerprint string `json:"fingerprint"`
	PublicKey   string `json:"public_key"`
	Stamp       string `json:"stamp"`
}
type receipt struct {
	actor       string
	connection  string
	observation Observation
	expires     time.Time
}

func (s *Service) Trust(ctx context.Context, id, stamp, token, fingerprint string) (Route, error) {
	actor, err := s.authorize(ctx)
	if err != nil {
		return Route{}, err
	}
	s.receiptsMu.Lock()
	defer s.receiptsMu.Unlock()
	r, ok := s.receipts[token]
	if !ok || r.actor != actor || r.connection != id || r.observation.Stamp != stamp || r.observation.Fingerprint != fingerprint || !time.Now().Before(r.expires) {
		return Route{}, ErrTrust
	}
	var out Route
	err = s.store.write(ctx, func(tx *sqlx.Tx) error {
		route, err := routeIn(ctx, tx, id)
		if err != nil {
			return err
		}
		if route.Stamp != stamp {
			return ErrConflict
		}
		if err = routeInUse(ctx, tx, id, true); err != nil {
			return err
		}
		c := route.Destination
		c.Fingerprint = fingerprint
		c.PublicKey = r.observation.PublicKey
		c.Version++
		c.Revision++
		c.VerifiedRoute = ""
		c.KnownHostsPending = false
		if err = putConnection(ctx, tx, c); err != nil {
			return err
		}
		if err = audit(ctx, tx, actor, id, "connection.trust", "trusted"); err != nil {
			return err
		}
		out, err = routeIn(ctx, tx, id)
		return err
	})
	if err == nil {
		delete(s.receipts, token)
	}
	return out, err
}
