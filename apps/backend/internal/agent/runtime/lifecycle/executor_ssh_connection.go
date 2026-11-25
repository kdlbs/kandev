package lifecycle

import (
	"context"
	"github.com/kandev/kandev/internal/sshtransport"
	"golang.org/x/crypto/ssh"
)

type SSHIdentitySource = sshtransport.SSHIdentitySource
type SSHTarget = sshtransport.SSHTarget
type SSHConnConfig = sshtransport.SSHConnConfig

const (
	SSHIdentitySourceAgent = sshtransport.SSHIdentitySourceAgent
	SSHIdentitySourceFile  = sshtransport.SSHIdentitySourceFile
)

func ResolveSSHTarget(cfg SSHConnConfig) (*SSHTarget, error) {
	return sshtransport.ResolveSSHTarget(cfg)
}
func SSHFingerprint(key ssh.PublicKey) string { return sshtransport.SSHFingerprint(key) }
func DialSSH(ctx context.Context, target *SSHTarget) (*ssh.Client, error) {
	return sshtransport.DialSSH(ctx, target)
}
func dialSSH(ctx context.Context, target *SSHTarget) (*ssh.Client, error) {
	return DialSSH(ctx, target)
}

type errHostKeyMismatch = sshtransport.HostKeyMismatchError
