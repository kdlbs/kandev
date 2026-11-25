package sshconnections

import (
	"context"
	"errors"
	"github.com/jmoiron/sqlx"
	"testing"
	"time"
)

func TestTrustReceiptsAreBoundToActorAndRoute(t *testing.T) {
	s, ctx := identityService(t)
	c, err := s.store.SaveConnection(ctx, Connection{ID: "target", Host: "host", Port: 22, User: "user"}, 0)
	if err != nil {
		t.Fatal(err)
	}
	var route Route
	err = s.store.write(ctx, func(tx *sqlx.Tx) error { var err error; route, err = routeIn(ctx, tx, c.ID); return err })
	if err != nil {
		t.Fatal(err)
	}
	s.receipts = map[string]receipt{"token": {actor: "admin", connection: c.ID, observation: Observation{Stamp: route.Stamp, Fingerprint: "SHA256:test", PublicKey: "observed-key"}, expires: time.Now().Add(time.Minute)}}
	if _, err = s.Trust(ctx, c.ID, route.Stamp, "token", "SHA256:other"); !errors.Is(err, ErrTrust) {
		t.Fatalf("accepted unobserved key: %v", err)
	}
	trusted, err := s.Trust(ctx, c.ID, route.Stamp, "token", "SHA256:test")
	if err != nil {
		t.Fatal(err)
	}
	if trusted.Destination.Fingerprint != "SHA256:test" || trusted.Stamp == route.Stamp {
		t.Fatal("trust must change route revision")
	}
	if _, err = s.Trust(ctx, c.ID, trusted.Stamp, "token", "SHA256:test"); !errors.Is(err, ErrTrust) {
		t.Fatal("receipt replay accepted")
	}
	if _, err = s.Trust(context.Background(), c.ID, trusted.Stamp, "token", "SHA256:test"); !errors.Is(err, ErrForbidden) {
		t.Fatal("missing identity accepted")
	}
}
