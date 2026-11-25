package sshtransport

import (
	"context"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"errors"
	"golang.org/x/crypto/ssh"
	"io"
	"net"
	"slices"
	"strconv"
	"sync/atomic"
	"testing"
	"time"
)

func managedServer(t *testing.T, forward ...string) (Hop, *atomic.Int32) {
	t.Helper()
	_, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(key)
	if err != nil {
		t.Fatal(err)
	}
	return managedServerWithHostKeys(t, []ssh.Signer{signer}, forward...)
}

func managedServerWithHostKeys(t *testing.T, keys []ssh.Signer, forward ...string) (Hop, *atomic.Int32) {
	t.Helper()
	auth := new(atomic.Int32)
	cfg := &ssh.ServerConfig{PasswordCallback: func(_ ssh.ConnMetadata, password []byte) (*ssh.Permissions, error) {
		auth.Add(1)
		expected := "secret"
		if len(forward) > 0 {
			expected = "bastion-secret"
		}
		if string(password) != expected {
			return nil, errors.New("incorrect credential")
		}
		return nil, nil
	}}
	for _, signer := range keys {
		cfg.AddHostKey(signer)
	}
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = l.Close() })
	go func() {
		for {
			c, e := l.Accept()
			if e != nil {
				return
			}
			go func() {
				defer func() { _ = c.Close() }()
				s, ch, r, e := ssh.NewServerConn(c, cfg)
				if e != nil {
					return
				}
				defer func() { _ = s.Close() }()
				go ssh.DiscardRequests(r)
				for channel := range ch {
					if len(forward) == 0 || channel.ChannelType() != "direct-tcpip" {
						_ = channel.Reject(ssh.Prohibited, "unsupported")
						continue
					}
					peer, err := net.Dial("tcp", forward[0])
					if err != nil {
						_ = channel.Reject(ssh.ConnectionFailed, "unreachable")
						continue
					}
					stream, requests, err := channel.Accept()
					if err != nil {
						_ = peer.Close()
						continue
					}
					go ssh.DiscardRequests(requests)
					go func() {
						defer func() { _ = peer.Close() }()
						defer func() { _ = stream.Close() }()
						_, _ = io.Copy(peer, stream)
					}()
					go func() {
						defer func() { _ = peer.Close() }()
						defer func() { _ = stream.Close() }()
						_, _ = io.Copy(stream, peer)
					}()
				}
			}()
		}
	}()
	host, port, _ := net.SplitHostPort(l.Addr().String())
	p, _ := strconv.Atoi(port)
	return Hop{Host: host, Port: p, User: "user", Fingerprint: ssh.FingerprintSHA256(keys[0].PublicKey()), Auth: []ssh.AuthMethod{ssh.Password("secret")}}, auth
}

func TestManagedRouteTrustBeforeAuthentication(t *testing.T) {
	for _, mode := range []string{"trusted", "changed", "untrusted"} {
		t.Run(mode, func(t *testing.T) {
			hop, auth := managedServer(t)
			if mode == "changed" {
				hop.Fingerprint = "SHA256:changed"
			}
			if mode == "untrusted" {
				hop.Fingerprint = ""
			}
			client, err := DialRoute(context.Background(), hop, nil)
			if mode == "trusted" {
				if err != nil {
					t.Fatal(err)
				}
				_ = client.Close()
				if auth.Load() != 1 {
					t.Fatalf("auth calls = %d", auth.Load())
				}
				return
			}
			if err == nil {
				_ = client.Close()
				t.Fatal("untrusted host connected")
			}
			if auth.Load() != 0 {
				t.Fatal("sent credentials before host trust")
			}
		})
	}
}

func TestObserveDoesNotAuthenticate(t *testing.T) {
	hop, auth := managedServer(t)
	hop.Fingerprint = ""
	key, err := Observe(context.Background(), hop, nil)
	if err != nil {
		t.Fatal(err)
	}
	if key == nil || auth.Load() != 0 {
		t.Fatal("observation must capture a key without sending credentials")
	}
}

func TestManagedProxyJumpUsesIndependentCredentialsAndPins(t *testing.T) {
	destination, destinationAuth := managedServer(t)
	bastion, bastionAuth := managedServer(t, destination.address())
	bastion.Auth = []ssh.AuthMethod{ssh.Password("bastion-secret")}
	client, err := DialRoute(context.Background(), destination, &bastion)
	if err != nil {
		t.Fatal(err)
	}
	_ = client.Close()
	if destinationAuth.Load() != 1 || bastionAuth.Load() != 1 {
		t.Fatal("both hops must authenticate")
	}
	bastion.Fingerprint = "SHA256:changed"
	if _, err = DialRoute(context.Background(), destination, &bastion); err == nil {
		t.Fatal("changed bastion accepted")
	}
	if destinationAuth.Load() != 1 || bastionAuth.Load() != 1 {
		t.Fatal("credentials sent after bastion trust failure")
	}
}

func TestManagedFingerprintFallbackNeverOffersInsecureSignatures(t *testing.T) {
	rsaSHA1 := managedRSAHostSigner(t, ssh.KeyAlgoRSA)
	alternate, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	ecdsaSigner, err := ssh.NewSignerFromKey(alternate)
	if err != nil {
		t.Fatal(err)
	}
	hop, auth := managedServerWithHostKeys(t, []ssh.Signer{rsaSHA1, ecdsaSigner})
	client, err := DialRoute(t.Context(), hop, nil)
	if client != nil {
		_ = client.Close()
	}
	var mismatch *HostKeyMismatchError
	if !errors.As(err, &mismatch) || auth.Load() != 0 {
		t.Fatalf("insecure fallback authenticated or lost the trust error: %v, auth calls %d", err, auth.Load())
	}
	algorithms := managedHostKeyAlgorithms()
	for _, insecure := range []string{"ssh-rsa", "ssh-dss", "ssh-rsa-cert-v01@openssh.com", "ssh-dss-cert-v01@openssh.com"} {
		if slices.Contains(algorithms, insecure) {
			t.Fatalf("fallback offered %s", insecure)
		}
	}
}

func TestManagedRSASignatureFormatPreservesFingerprintVerification(t *testing.T) {
	for _, signature := range []string{ssh.KeyAlgoRSASHA256, ssh.KeyAlgoRSASHA512} {
		t.Run(signature, func(t *testing.T) {
			signer := managedRSAHostSigner(t, signature)
			hop, auth := managedServerWithHostKeys(t, []ssh.Signer{signer})
			hop.HostKeyTypes = []string{ssh.KeyAlgoRSA}
			client, err := DialRoute(t.Context(), hop, nil)
			if err != nil {
				t.Fatal(err)
			}
			_ = client.Close()
			hop.Fingerprint = "SHA256:other-rsa-key"
			client, err = DialRoute(t.Context(), hop, nil)
			if client != nil {
				_ = client.Close()
			}
			var mismatch *HostKeyMismatchError
			if !errors.As(err, &mismatch) || auth.Load() != 1 {
				t.Fatalf("shared RSA algorithm bypassed fingerprint check: %v, auth calls %d", err, auth.Load())
			}
		})
	}
}

func managedRSAHostSigner(t *testing.T, signature string) ssh.Signer {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(key)
	if err != nil {
		t.Fatal(err)
	}
	restricted, err := ssh.NewSignerWithAlgorithms(signer.(ssh.AlgorithmSigner), []string{signature})
	if err != nil {
		t.Fatal(err)
	}
	return restricted
}

func TestManagedHandshakeDeadlineBoundsSilentDestination(t *testing.T) {
	for _, mode := range []string{"direct", "tunneled"} {
		t.Run(mode, func(t *testing.T) {
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = listener.Close() }()
			accepted := make(chan net.Conn, 1)
			go func() {
				if peer, err := listener.Accept(); err == nil {
					accepted <- peer
				}
			}()
			defer func() {
				select {
				case peer := <-accepted:
					_ = peer.Close()
				default:
				}
			}()
			var bastion *Hop
			if mode == "tunneled" {
				jump, _ := managedServer(t, listener.Addr().String())
				jump.Auth = []ssh.AuthMethod{ssh.Password("bastion-secret")}
				bastion = &jump
			}
			ctx, cancel := context.WithTimeout(t.Context(), 250*time.Millisecond)
			defer cancel()
			destination := Hop{Host: "127.0.0.1", Port: listener.Addr().(*net.TCPAddr).Port, User: "user", Fingerprint: "SHA256:trusted"}
			finished := make(chan error, 1)
			go func() {
				client, err := DialRoute(ctx, destination, bastion)
				if client != nil {
					_ = client.Close()
				}
				finished <- err
			}()
			select {
			case err := <-finished:
				var timeout net.Error
				isNetworkTimeout := errors.As(err, &timeout) && timeout.Timeout()
				if !errors.Is(err, context.DeadlineExceeded) && !isNetworkTimeout {
					t.Fatalf("handshake did not return a timeout: %v", err)
				}
			case <-time.After(2 * time.Second):
				t.Fatal("silent SSH destination exceeded the caller's deadline")
			}
		})
	}
}
