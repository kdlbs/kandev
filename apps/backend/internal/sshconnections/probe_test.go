package sshconnections

import (
	"crypto/ed25519"
	"crypto/rand"
	"github.com/google/uuid"
	"golang.org/x/crypto/ssh"
	"net"
	"strconv"
	"sync/atomic"
	"testing"
)

func TestObserveTrustAndAuthenticateManagedIdentity(t *testing.T) {
	service, ctx := identityService(t)
	identity, err := service.CreateIdentity(ctx, CreateIdentityRequest{RequestID: uuid.NewString(), Label: "host access", Kind: "generate"})
	if err != nil {
		t.Fatal(err)
	}
	_, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	host, err := ssh.NewSignerFromKey(private)
	if err != nil {
		t.Fatal(err)
	}
	var auth atomic.Int32
	var rejectAuth atomic.Bool
	cfg := &ssh.ServerConfig{PublicKeyCallback: func(_ ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
		auth.Add(1)
		if rejectAuth.Load() || ssh.FingerprintSHA256(key) != identity.Fingerprint {
			return nil, ErrForbidden
		}
		return nil, nil
	}}
	cfg.AddHostKey(host)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	go func() {
		for {
			c, err := listener.Accept()
			if err != nil {
				return
			}
			go func() {
				defer func() { _ = c.Close() }()
				conn, ch, r, err := ssh.NewServerConn(c, cfg)
				if err != nil {
					return
				}
				defer func() { _ = conn.Close() }()
				go ssh.DiscardRequests(r)
				for request := range ch {
					_ = request.Reject(ssh.Prohibited, "not needed")
				}
			}()
		}
	}()
	hostName, port, _ := net.SplitHostPort(listener.Addr().String())
	number, _ := strconv.Atoi(port)
	c, err := service.store.SaveConnection(ctx, Connection{ID: uuid.NewString(), Host: hostName, Port: number, User: "user", IdentityID: identity.ID}, 0)
	if err != nil {
		t.Fatal(err)
	}
	route, err := service.GetConnection(ctx, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	observation, err := service.Observe(ctx, c.ID, route.Stamp)
	if err != nil {
		t.Fatal(err)
	}
	if auth.Load() != 0 {
		t.Fatal("observation authenticated")
	}
	trusted, err := service.Trust(ctx, c.ID, observation.Stamp, observation.Receipt, observation.Fingerprint)
	if err != nil {
		t.Fatal(err)
	}
	ready, err := service.TestConnection(ctx, c.ID, trusted.Stamp)
	if err != nil {
		t.Fatal(err)
	}
	if ready.Destination.VerifiedRoute != ready.Stamp || auth.Load() == 0 {
		t.Fatal("connection not proven with its managed identity")
	}
	rejectAuth.Store(true)
	if _, err = service.TestConnection(ctx, c.ID, ready.Stamp); err == nil {
		t.Fatal("revoked key authenticated")
	}
	saved, err := service.store.GetConnection(ctx, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	if saved.VerifiedRoute != "" {
		t.Fatal("failed authentication retained readiness")
	}

}
