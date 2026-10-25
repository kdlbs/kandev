package lifecycle

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/kandev/kandev/internal/agent/docker"
)

// remoteDockerIntegrationImage is an sshd host carrying the Docker CLI, run
// with the host's daemon socket mounted in. Build it with
// scripts/build-remote-docker-test-image.sh.
const remoteDockerIntegrationImage = "kandev-rdocker-int:test"

// TestRemoteDockerTransportReachesARealDaemon drives the whole transport
// against a real Docker daemon over a real SSH connection: dial, run
// `docker system dial-stdio`, and speak the Engine API across it.
//
// The unit tests cover this wiring with stubs. This is the test that catches
// what stubs cannot: a wrong client option order, a broken half-close, or a
// dial-stdio invocation the real CLI rejects.
//
// Opt-in, because it needs a Docker daemon and a prebuilt image.
func TestRemoteDockerTransportReachesARealDaemon(t *testing.T) {
	if os.Getenv("KANDEV_TEST_REMOTE_DOCKER") != "1" {
		t.Skip("set KANDEV_TEST_REMOTE_DOCKER=1 and build " + remoteDockerIntegrationImage)
	}

	signer, authorizedKey := generateRemoteDockerTestKey(t)
	port, stop := startRemoteDockerTestHost(t, authorizedKey)
	t.Cleanup(stop)

	sshClient, err := dialTestSSH(port, signer, 30*time.Second)
	if err != nil {
		t.Fatalf("dial the test sshd: %v", err)
	}
	defer func() { _ = sshClient.Close() }()

	log := newTestLogger()
	dockerClient, err := docker.NewRemoteClient(NewSSHDockerDialer(sshClient, log), log)
	if err != nil {
		t.Fatalf("NewRemoteClient: %v", err)
	}
	defer func() { _ = dockerClient.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	version, err := dockerClient.PingVersion(ctx)
	if err != nil {
		t.Fatalf("PingVersion over the SSH transport: %v", err)
	}
	if version == "" {
		t.Fatal("daemon reported no API version")
	}
	t.Logf("reached a real daemon over SSH: API %s", version)

	// A second request proves the pooled connection survives the first one.
	// Binding the connection's lifetime to its dialing request fails here.
	if _, err := dockerClient.PingVersion(ctx); err != nil {
		t.Fatalf("second PingVersion (connection reuse): %v", err)
	}

	// Listing containers exercises a request with a response body, which is
	// what the half-close in CloseWrite exists for.
	if _, err := dockerClient.ListContainers(ctx, nil); err != nil {
		t.Fatalf("ListContainers over the SSH transport: %v", err)
	}
}

// dialTestSSH retries the handshake rather than only the TCP connect: the
// published port accepts connections as soon as Docker maps it, which is
// before sshd is ready, so a single dial races the container's startup.
func dialTestSSH(port int, signer ssh.Signer, budget time.Duration) (*ssh.Client, error) {
	cfg := &ssh.ClientConfig{
		User:            "kandev",
		Auth:            []ssh.AuthMethod{ssh.PublicKeys(signer)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), //nolint:gosec // throwaway container, key generated per run
		Timeout:         10 * time.Second,
	}
	addr := net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
	deadline := time.Now().Add(budget)
	var lastErr error
	for time.Now().Before(deadline) {
		client, err := ssh.Dial("tcp", addr, cfg)
		if err == nil {
			return client, nil
		}
		lastErr = err
		time.Sleep(300 * time.Millisecond)
	}
	return nil, lastErr
}

func generateRemoteDockerTestKey(t *testing.T) (ssh.Signer, string) {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	block, err := ssh.MarshalPrivateKey(priv, "")
	if err != nil {
		t.Fatalf("marshal key: %v", err)
	}
	signer, err := ssh.ParsePrivateKey(pem.EncodeToMemory(block))
	if err != nil {
		t.Fatalf("parse key: %v", err)
	}
	return signer, strings.TrimSpace(string(ssh.MarshalAuthorizedKey(signer.PublicKey())))
}

func startRemoteDockerTestHost(t *testing.T, authorizedKey string) (int, func()) {
	t.Helper()
	out, err := exec.Command("docker", "run", "-d", "--rm",
		"-p", "0:22",
		"-e", "AUTHORIZED_KEY="+authorizedKey,
		"-v", "/var/run/docker.sock:/var/run/docker.sock",
		remoteDockerIntegrationImage,
	).Output()
	if err != nil {
		t.Fatalf("start the test sshd container: %v", err)
	}
	id := strings.TrimSpace(string(out))
	stop := func() { _ = exec.Command("docker", "rm", "-f", id).Run() }

	port, err := mappedSSHPort(id)
	if err != nil {
		stop()
		t.Fatalf("read the mapped port: %v", err)
	}
	if err := waitForTCP(port, 30*time.Second); err != nil {
		stop()
		t.Fatalf("sshd never accepted a connection: %v", err)
	}
	return port, stop
}

func mappedSSHPort(containerID string) (int, error) {
	out, err := exec.Command("docker", "port", containerID, "22/tcp").Output()
	if err != nil {
		return 0, err
	}
	line := strings.TrimSpace(strings.Split(strings.TrimSpace(string(out)), "\n")[0])
	_, portText, err := net.SplitHostPort(line)
	if err != nil {
		return 0, fmt.Errorf("parse %q: %w", line, err)
	}
	return strconv.Atoi(portText)
}

func waitForTCP(port int, budget time.Duration) error {
	deadline := time.Now().Add(budget)
	addr := net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
	var lastErr error
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
		if err == nil {
			_ = conn.Close()
			return nil
		}
		lastErr = err
		time.Sleep(200 * time.Millisecond)
	}
	return lastErr
}
