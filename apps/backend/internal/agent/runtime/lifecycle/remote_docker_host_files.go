package lifecycle

import (
	"context"
	"fmt"
	"os"
	"path"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/kandev/kandev/internal/common/logger"
)

// remoteHostFileTimeout bounds each remote materialization step so a wedged
// host fails the launch instead of hanging it.
const remoteHostFileTimeout = 2 * time.Minute

// sshHostFileStore materializes container inputs on the remote host over the
// same SSH connection that carries the Engine API.
type sshHostFileStore struct {
	client   *ssh.Client
	resolver *AgentctlResolver
	logger   *logger.Logger

	mu           sync.Mutex
	agentctlPath string
	sessionDirs  map[string]string
	files        map[string]string
}

func newSSHHostFileStore(client *ssh.Client, resolver *AgentctlResolver, log *logger.Logger) *sshHostFileStore {
	return &sshHostFileStore{
		client:      client,
		resolver:    resolver,
		logger:      log,
		sessionDirs: map[string]string{},
		files:       map[string]string{},
	}
}

// EnsureAgentctl uploads the platform-matched helper, or reuses the cached one
// already on the host. The upload is content-hashed, so a matching cache is
// not re-sent.
func (s *sshHostFileStore) EnsureAgentctl(platform SSHRemotePlatform) (string, error) {
	s.mu.Lock()
	cached := s.agentctlPath
	s.mu.Unlock()
	if cached != "" {
		return cached, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), remoteHostFileTimeout)
	defer cancel()

	remotePath, err := ensureAgentctlOnHost(ctx, s.client, s.resolver, platform, s.logger)
	if err != nil {
		return "", err
	}

	s.mu.Lock()
	s.agentctlPath = remotePath
	s.mu.Unlock()
	return remotePath, nil
}

// EnsureSessionDir creates the per-instance agent session directory on the
// remote host. It lives under the remote user's Kandev home, mirroring where
// the local runtime puts it under the backend's own Kandev home.
func (s *sshHostFileStore) EnsureSessionDir(instanceID string) (string, error) {
	if instanceID == "" {
		return "", fmt.Errorf("remote docker: instance ID is required for a session dir")
	}

	s.mu.Lock()
	cached := s.sessionDirs[instanceID]
	s.mu.Unlock()
	if cached != "" {
		return cached, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), remoteHostFileTimeout)
	defer cancel()

	root, err := expandRemoteHome(ctx, s.client, remoteKandevHomeDir)
	if err != nil {
		return "", fmt.Errorf("remote docker: resolve remote home: %w", err)
	}
	dir := path.Join(root, "agent-sessions", instanceID)

	if _, stderr, err := runSSHCommand(ctx, s.client, "mkdir -p "+shellQuote(dir)); err != nil {
		return "", fmt.Errorf("remote docker: create session dir %s: %w (%s)", dir, err, stderr)
	}

	s.mu.Lock()
	s.sessionDirs[instanceID] = dir
	s.mu.Unlock()
	return dir, nil
}

// EnsureFile uploads a local file into the remote Kandev bin directory,
// alongside the cached agentctl helper, and returns its remote path.
func (s *sshHostFileStore) EnsureFile(name, localPath string) (string, error) {
	if name == "" || localPath == "" {
		return "", fmt.Errorf("remote docker: file name and source are required")
	}

	s.mu.Lock()
	cached := s.files[name]
	s.mu.Unlock()
	if cached != "" {
		return cached, nil
	}

	data, err := os.ReadFile(localPath) //nolint:gosec // operator-supplied build artifact
	if err != nil {
		return "", fmt.Errorf("remote docker: read %s: %w", localPath, err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), remoteHostFileTimeout)
	defer cancel()

	root, err := expandRemoteHome(ctx, s.client, remoteKandevHomeDir)
	if err != nil {
		return "", fmt.Errorf("remote docker: resolve remote home: %w", err)
	}
	dir := path.Join(root, "bin")
	if _, stderr, err := runSSHCommand(ctx, s.client, "mkdir -p "+shellQuote(dir)); err != nil {
		return "", fmt.Errorf("remote docker: create %s: %w (%s)", dir, err, stderr)
	}

	remotePath := path.Join(dir, name)
	if err := sftpUploadBytes(s.client, remotePath, data, 0o755); err != nil {
		return "", fmt.Errorf("remote docker: upload %s: %w", name, err)
	}

	s.mu.Lock()
	s.files[name] = remotePath
	s.mu.Unlock()
	return remotePath, nil
}

// remoteKandevHomeDir is the Kandev root on the remote host, matching the SSH
// executor's default workdir root.
const remoteKandevHomeDir = "~/.kandev"
