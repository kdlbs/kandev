package lifecycle

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	"golang.org/x/crypto/ssh"

	"github.com/kandev/kandev/internal/common/logger"
)

// nomadPortForward is a loopback endpoint on the Kandev host that reaches one
// port of the worker's allocation.
type nomadPortForward interface {
	LocalPort() int
	Close() error
}

// nomadWorkerTransport is the only thing that differs between attaching to an
// SSH worker and to the worker on the Kandev host itself: running commands,
// writing files and reaching the allocation's ports.
type nomadWorkerTransport interface {
	Run(ctx context.Context, command string) (stdout, stderr string, err error)
	RunStdin(ctx context.Context, command string, stdin io.Reader) (stdout, stderr string, err error)
	Upload(path string, data []byte, mode os.FileMode) error
	Forward(port int, log *logger.Logger) (nomadPortForward, error)
	Close() error
}

type nomadSSHTransport struct{ client *ssh.Client }

func (t nomadSSHTransport) Run(ctx context.Context, command string) (string, string, error) {
	return runSSHCommand(ctx, t.client, command)
}

func (t nomadSSHTransport) RunStdin(ctx context.Context, command string, stdin io.Reader) (string, string, error) {
	return runSSHCommandStdin(ctx, t.client, command, stdin)
}

func (t nomadSSHTransport) Upload(path string, data []byte, mode os.FileMode) error {
	return sftpUploadBytes(t.client, path, data, mode)
}

func (t nomadSSHTransport) Forward(port int, log *logger.Logger) (nomadPortForward, error) {
	return StartPortForward(t.client, port, log)
}

func (t nomadSSHTransport) Close() error { return t.client.Close() }

// nomadLocalTransport runs on the Kandev host. The allocation listens on the
// host's loopback, so a forward is the port itself.
type nomadLocalTransport struct{}

type nomadLoopbackForward int

func (f nomadLoopbackForward) LocalPort() int { return int(f) }
func (nomadLoopbackForward) Close() error     { return nil }

func (nomadLocalTransport) Run(ctx context.Context, command string) (string, string, error) {
	return nomadLocalRun(ctx, command, nil)
}

func (nomadLocalTransport) RunStdin(ctx context.Context, command string, stdin io.Reader) (string, string, error) {
	return nomadLocalRun(ctx, command, stdin)
}

func nomadLocalRun(ctx context.Context, command string, stdin io.Reader) (string, string, error) {
	cmd := exec.CommandContext(ctx, "/bin/sh", "-c", command)
	var stdout, stderr bytes.Buffer
	cmd.Stdin, cmd.Stdout, cmd.Stderr = stdin, &stdout, &stderr
	err := cmd.Run()
	return stdout.String(), stderr.String(), err
}

// Upload writes through a private temporary file and renames it, matching the
// SFTP upload so a reader never sees a partial file.
func (nomadLocalTransport) Upload(path string, data []byte, mode os.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".tmp.*")
	if err != nil {
		return fmt.Errorf("local upload: create %s: %w", path, err)
	}
	name := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(name)
		return fmt.Errorf("local upload: write %s: %w", path, err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(name)
		return fmt.Errorf("local upload: close %s: %w", path, err)
	}
	if err := os.Chmod(name, mode); err != nil {
		_ = os.Remove(name)
		return fmt.Errorf("local upload: chmod %s: %w", path, err)
	}
	if err := os.Rename(name, path); err != nil {
		_ = os.Remove(name)
		return fmt.Errorf("local upload: rename %s: %w", path, err)
	}
	return nil
}

func (nomadLocalTransport) Forward(port int, _ *logger.Logger) (nomadPortForward, error) {
	if port < 1 {
		return nil, fmt.Errorf("local forward: invalid port %d", port)
	}
	return nomadLoopbackForward(port), nil
}

func (nomadLocalTransport) Close() error { return nil }
