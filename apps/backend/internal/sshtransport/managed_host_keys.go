package sshtransport

import (
	"errors"
	"net"
	"slices"
	"sync/atomic"

	"golang.org/x/crypto/ssh"
)

// ErrTryOtherKnownHostKey marks a known-hosts mismatch that can be checked
// against another already trusted algorithm. Revocation must not use it.
var ErrTryOtherKnownHostKey = errors.New("other known host key may match")

type managedHostKeyHandshake struct {
	config         *ssh.ClientConfig
	fingerprint    string
	rejectedFormat *atomic.Pointer[string]
	lastRejection  error
}

func (a *managedHostKeyHandshake) prepare() *ssh.ClientConfig {
	rejected := new(atomic.Pointer[string])
	a.rejectedFormat = rejected
	cfg := *a.config
	verify := cfg.HostKeyCallback
	cfg.HostKeyCallback = func(host string, remote net.Addr, key ssh.PublicKey) error {
		err := verify(host, remote, key)
		var mismatch *HostKeyMismatchError
		if errors.As(err, &mismatch) || errors.Is(err, ErrTryOtherKnownHostKey) {
			format := key.Type()
			rejected.Store(&format)
		}
		return err
	}
	return &cfg
}

func (a *managedHostKeyHandshake) retry(err error) (bool, error) {
	rejected := a.rejectedFormat.Swap(nil)
	if rejected == nil {
		return false, a.classifyNegotiationError(err)
	}
	a.lastRejection = err
	remaining := slices.Clone(a.config.HostKeyAlgorithms)
	if len(remaining) == 0 {
		remaining = managedHostKeyAlgorithms()
	}
	a.config.HostKeyAlgorithms = slices.DeleteFunc(remaining, func(algorithm string) bool {
		return hostKeyFormat(algorithm) == *rejected
	})
	return len(a.config.HostKeyAlgorithms) > 0, err
}

func (a *managedHostKeyHandshake) classifyNegotiationError(err error) error {
	var negotiation *ssh.AlgorithmNegotiationError
	if !errors.As(err, &negotiation) || negotiation.What != "host key" {
		return err
	}
	if a.lastRejection != nil {
		return a.lastRejection
	}
	if len(a.config.HostKeyAlgorithms) > 0 {
		return &HostKeyMismatchError{Expected: a.fingerprint, Got: "no offered key uses a trusted algorithm"}
	}
	return err
}

func managedHostKeyAlgorithms() []string {
	return ssh.SupportedAlgorithms().HostKeys
}

func managedAlgorithmsForKeyTypes(types []string) []string {
	return slices.DeleteFunc(managedHostKeyAlgorithms(), func(algorithm string) bool {
		return !slices.Contains(types, hostKeyFormat(algorithm))
	})
}

func hostKeyFormat(algorithm string) string {
	switch algorithm {
	case ssh.KeyAlgoRSASHA256, ssh.KeyAlgoRSASHA512:
		return ssh.KeyAlgoRSA
	case ssh.CertAlgoRSASHA256v01, ssh.CertAlgoRSASHA512v01:
		return ssh.CertAlgoRSAv01
	default:
		return algorithm
	}
}
