package runtime

import (
	"context"
	"errors"
	"io"
	"net"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"golang.org/x/crypto/ssh"
)

type testSSHConn struct {
	net.Conn
}

func (c testSSHConn) User() string          { return "test" }
func (c testSSHConn) SessionID() []byte     { return nil }
func (c testSSHConn) ClientVersion() []byte { return []byte("client") }
func (c testSSHConn) ServerVersion() []byte { return []byte("server") }
func (c testSSHConn) SendRequest(string, bool, []byte) (bool, []byte, error) {
	return false, nil, nil
}
func (c testSSHConn) OpenChannel(string, []byte) (ssh.Channel, <-chan *ssh.Request, error) {
	return nil, nil, errors.New("test SSH connection has no channels")
}
func (c testSSHConn) Wait() error { return nil }

func TestSSHTaskDirReclaimerRejectsMissingFingerprint(t *testing.T) {
	_, err := (&sshTaskDirReclaimer{}).ReclaimTaskDir(context.Background(), SSHTaskDirReclaimRequest{
		Host: "build.example",
	})
	if err == nil || err.Error() != "ssh reclaim: no pinned host fingerprint recorded for build.example" {
		t.Fatalf("ReclaimTaskDir error = %v, want missing fingerprint error", err)
	}
}

func TestSSHTaskDirReclaimerMapsTargetAndResultAndClosesClient(t *testing.T) {
	serverConn, clientConn := net.Pipe()
	defer func() { _ = serverConn.Close() }()
	if err := serverConn.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatalf("SetReadDeadline: %v", err)
	}
	client := ssh.NewClient(testSSHConn{Conn: clientConn}, nil, nil)

	var gotRequest SSHTaskDirReclaimRequest
	var gotClient *ssh.Client
	reclaimer := &sshTaskDirReclaimer{
		open: func(_ context.Context, request SSHTaskDirReclaimRequest) (*ssh.Client, func(), error) {
			gotRequest = request
			return client, func() { _ = client.Close() }, nil
		},
		reclaim: func(_ context.Context, got *ssh.Client, shell, root, taskDir string) (lifecycle.SSHReclaimOutcome, lifecycle.SSHReclaimVerdict, error) {
			gotClient = got
			if shell != "bash" || root != "/srv/work" || taskDir != "/srv/work/task" {
				t.Fatalf("reclaim args = %q, %q, %q", shell, root, taskDir)
			}
			return lifecycle.SSHReclaimOutcomeSkipped, lifecycle.SSHReclaimVerdict{
				Reason: lifecycle.SSHReclaimSkipUnpushedCommits,
				Detail: "2 commits",
			}, nil
		},
	}

	result, err := reclaimer.ReclaimTaskDir(context.Background(), SSHTaskDirReclaimRequest{
		Host:            "build.example",
		Port:            22,
		User:            "builder",
		IdentitySource:  "file",
		IdentityFile:    "/home/builder/.ssh/id_ed25519",
		ProxyJump:       "bastion.example",
		HostFingerprint: "SHA256:pinned",
		Shell:           "bash",
		WorkdirRoot:     "/srv/work",
		TaskDir:         "/srv/work/task",
	})
	if err != nil {
		t.Fatalf("ReclaimTaskDir: %v", err)
	}
	if gotClient != client {
		t.Fatal("reclaim did not receive the dialled client")
	}
	if gotRequest.Host != "build.example" || gotRequest.Port != 22 || gotRequest.User != "builder" ||
		gotRequest.IdentitySource != "file" || gotRequest.IdentityFile != "/home/builder/.ssh/id_ed25519" ||
		gotRequest.ProxyJump != "bastion.example" || gotRequest.HostFingerprint != "SHA256:pinned" {
		t.Fatalf("retained route request = %+v", gotRequest)
	}
	if result != (SSHTaskDirReclaimResult{SkipReason: "unpushed_commits", Detail: "2 commits"}) {
		t.Fatalf("runtime result = %+v, want safety skip", result)
	}

	_, readErr := serverConn.Read(make([]byte, 1))
	if !errors.Is(readErr, io.EOF) {
		t.Fatalf("server read after adapter return = %v, want EOF from client close", readErr)
	}
}
