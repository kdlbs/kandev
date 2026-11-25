package sshconnections

import (
	"context"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"net"
	"os"
	"time"
)

type CredentialFailure struct{ Code string }

func (e *CredentialFailure) Error() string { return e.Code }
func (e *CredentialFailure) Unwrap() error { return ErrCredential }

func agentAuthentication(ctx context.Context, socket string) ([]ssh.AuthMethod, func(), error) {
	empty := func() {}
	if socket == "" {
		socket = os.Getenv("SSH_AUTH_SOCK")
	}
	deadline := time.Now().Add(5 * time.Second)
	if parent, ok := ctx.Deadline(); ok && parent.Before(deadline) {
		deadline = parent
	}
	dialer := net.Dialer{Deadline: deadline}
	conn, err := dialer.DialContext(ctx, "unix", socket)
	if err != nil {
		return nil, empty, &CredentialFailure{Code: codeAgentUnavailable}
	}
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	cleanup := func() { stop(); _ = conn.Close() }
	if err = conn.SetDeadline(deadline); err != nil {
		cleanup()
		return nil, empty, &CredentialFailure{Code: codeAgentUnavailable}
	}
	signers, err := agent.NewClient(conn).Signers()
	if err != nil {
		cleanup()
		return nil, empty, &CredentialFailure{Code: codeAgentUnavailable}
	}
	if len(signers) == 0 {
		cleanup()
		return nil, empty, &CredentialFailure{Code: "ssh_agent_empty"}
	}
	deadline = time.Now().Add(30 * time.Second)
	if parent, ok := ctx.Deadline(); ok && parent.Before(deadline) {
		deadline = parent
	}
	if err = conn.SetDeadline(deadline); err != nil {
		cleanup()
		return nil, empty, &CredentialFailure{Code: codeAgentUnavailable}
	}
	return []ssh.AuthMethod{ssh.PublicKeys(signers...)}, cleanup, nil
}
