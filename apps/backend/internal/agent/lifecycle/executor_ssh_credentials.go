package lifecycle

import (
	"context"
	"os"

	"golang.org/x/crypto/ssh"
)

// sshFileUploader implements FileUploader for an SSH client by writing through
// SFTP. Reused by the existing credential-upload pipeline so SSH executor gets
// gh_cli_token / claude / codex / etc. credential propagation for free.
type sshFileUploader struct {
	client *ssh.Client
}

func newSSHFileUploader(client *ssh.Client) *sshFileUploader {
	return &sshFileUploader{client: client}
}

func (u *sshFileUploader) WriteFile(_ context.Context, path string, data []byte, mode os.FileMode) error {
	return sftpWriteFile(u.client, path, data, mode)
}
