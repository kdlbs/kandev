package sshtransport

import (
	"context"
	"errors"
	"fmt"
	"golang.org/x/crypto/ssh"
	"net"
	"strconv"
)

const (
	destinationHop = "destination"
	bastionHop     = "bastion"
)

// Hop contains the resolved endpoint and credentials for one explicitly trusted host.
type Hop struct {
	Host              string
	Port              int
	User              string
	Fingerprint       string
	HostKeyTypes      []string
	Auth              []ssh.AuthMethod
	VerifyImportedKey ssh.HostKeyCallback
}

// HopError identifies the failed hop while retaining its cause for internal classification.
type HopError struct {
	Hop string
	Err error
}

func (e *HopError) Error() string { return fmt.Sprintf("ssh %s: %v", e.Hop, e.Err) }
func (e *HopError) Unwrap() error { return e.Err }

func (h Hop) address() string { return net.JoinHostPort(h.Host, strconv.Itoa(h.Port)) }
func (h Hop) config() (*ssh.ClientConfig, error) {
	if h.Host == "" || h.Port < 1 || h.Port > 65535 || h.User == "" {
		return nil, errors.New("invalid endpoint")
	}
	if h.Fingerprint == "" && h.VerifyImportedKey == nil {
		return nil, errors.New("host trust required")
	}
	cfg := &ssh.ClientConfig{
		User: h.User, Auth: h.Auth, Timeout: sshDialTimeout, HostKeyCallback: h.verifyKey,
	}
	if len(h.HostKeyTypes) != 0 {
		cfg.HostKeyAlgorithms = managedAlgorithmsForKeyTypes(h.HostKeyTypes)
		if len(cfg.HostKeyAlgorithms) == 0 {
			return nil, errors.New("no supported host key algorithm")
		}
	}
	return cfg, nil
}

func (h Hop) verifyKey(host string, remote net.Addr, key ssh.PublicKey) error {
	if h.Fingerprint == "" {
		return h.VerifyImportedKey(host, remote, key)
	}
	got := SSHFingerprint(key)
	if got != h.Fingerprint {
		return &HostKeyMismatchError{Expected: h.Fingerprint, Got: got}
	}
	return nil
}

// DialRoute authenticates each hop independently without reading SSH configuration.
func DialRoute(ctx context.Context, destination Hop, bastion *Hop) (*ssh.Client, error) {
	cfg, err := destination.config()
	if err != nil {
		return nil, &HopError{destinationHop, err}
	}
	return dialConfigured(ctx, destination, cfg, bastion)
}

func dialConfigured(ctx context.Context, destination Hop, cfg *ssh.ClientConfig, bastion *Hop) (*ssh.Client, error) {
	ctx, cancel := context.WithTimeout(ctx, sshDialTimeout)
	defer cancel()
	d := &managedHostKeyHandshake{config: cfg, fingerprint: destination.Fingerprint}
	var b *managedHostKeyHandshake
	if bastion != nil {
		bc, err := bastion.config()
		if err != nil {
			return nil, &HopError{bastionHop, err}
		}
		b = &managedHostKeyHandshake{config: bc, fingerprint: bastion.Fingerprint}
	}
	for {
		var bastionConfig *ssh.ClientConfig
		if b != nil {
			bastionConfig = b.prepare()
		}
		client, failure := dialManagedRouteOnce(ctx, destination, d.prepare(), bastion, bastionConfig)
		if failure == nil {
			return client, nil
		}
		if err := ctx.Err(); err != nil {
			return nil, &HopError{failure.Hop, err}
		}
		handshake := d
		if failure.Hop == bastionHop {
			handshake = b
		}
		retry, err := handshake.retry(failure.Err)
		if !retry {
			return nil, &HopError{failure.Hop, err}
		}
	}
}

func dialManagedRouteOnce(ctx context.Context, destination Hop, cfg *ssh.ClientConfig, bastion *Hop, bastionConfig *ssh.ClientConfig) (*ssh.Client, *HopError) {
	if bastion == nil {
		client, err := dialDirect(ctx, destination.address(), cfg)
		if err != nil {
			return nil, &HopError{destinationHop, err}
		}
		return client, nil
	}
	client, err := dialDirect(ctx, bastion.address(), bastionConfig)
	if err != nil {
		return nil, &HopError{bastionHop, err}
	}
	tunnel, err := client.DialContext(ctx, "tcp", destination.address())
	if err != nil {
		_ = client.Close()
		return nil, &HopError{destinationHop, err}
	}
	owned := &sshProxyJumpConn{Conn: tunnel, bastion: client}
	conn, ch, r, err := handshakeWithDeadline(ctx, owned, destination.address(), cfg)
	if err != nil {
		_ = owned.Close()
		return nil, &HopError{destinationHop, err}
	}
	destinationClient := ssh.NewClient(conn, ch, r)
	go func() { _ = destinationClient.Wait(); _ = owned.Close() }()
	return destinationClient, nil
}

// Observe captures a host key during key exchange and aborts before
// destination authentication. An existing pin takes precedence during review.
func Observe(ctx context.Context, destination Hop, bastion *Hop) (ssh.PublicKey, error) {
	ctx, cancel := context.WithTimeout(ctx, sshDialTimeout)
	defer cancel()
	key, err := observeHostKey(ctx, destination, bastion)
	var hop *HopError
	var mismatch *HostKeyMismatchError
	if destination.Fingerprint == "" || !errors.As(err, &hop) ||
		hop.Hop != destinationHop || !errors.As(err, &mismatch) {
		return key, err
	}
	destination.Fingerprint = ""
	destination.HostKeyTypes = nil
	return observeHostKey(ctx, destination, bastion)
}

func observeHostKey(ctx context.Context, destination Hop, bastion *Hop) (ssh.PublicKey, error) {
	pinned := destination.Fingerprint
	destination.Fingerprint = "observation"
	cfg, err := destination.config()
	if err != nil {
		return nil, err
	}
	cfg.Auth = nil
	observed := errors.New("host key observed")
	var key ssh.PublicKey
	cfg.HostKeyCallback = func(_ string, _ net.Addr, k ssh.PublicKey) error {
		if got := SSHFingerprint(k); pinned != "" && got != pinned {
			return &HostKeyMismatchError{Expected: pinned, Got: got}
		}
		key = k
		return observed
	}
	_, err = dialConfigured(ctx, destination, cfg, bastion)
	if errors.Is(err, observed) && key != nil {
		return key, nil
	}
	return nil, err
}
