package lifecycle

import (
	"fmt"
	mrand "math/rand/v2"
	"os"
	"path"
	"strings"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

// sftpReplaceWindowsFile installs data at remotePath on a Windows host.
// Windows rename never overwrites an existing file, and the image of a running
// process cannot be deleted, but it can be renamed. The existing file therefore
// moves aside before the new file moves in, so a running agentctl.exe keeps its
// image while a new launch gets the replacement.
func sftpReplaceWindowsFile(client *ssh.Client, remotePath string, data []byte) error {
	c, err := sftp.NewClient(client)
	if err != nil {
		return fmt.Errorf("sftp: new client: %w", err)
	}
	defer func() { _ = c.Close() }()

	target := toSFTPPath(remotePath)
	tmp := fmt.Sprintf("%s.tmp.%d.%d", target, os.Getpid(), mrand.Uint64())
	if err := writeSFTPFile(c, tmp, data); err != nil {
		_ = c.Remove(tmp)
		return err
	}
	aside := ""
	if _, err := c.Stat(target); err == nil {
		aside = fmt.Sprintf("%s.old-%d", target, mrand.Uint64())
		if err := c.Rename(target, aside); err != nil {
			_ = c.Remove(tmp)
			return fmt.Errorf("sftp: move aside %s: %w", target, err)
		}
	}
	if err := c.Rename(tmp, target); err != nil {
		if aside != "" {
			_ = c.Rename(aside, target)
		}
		_ = c.Remove(tmp)
		return fmt.Errorf("sftp: rename %s -> %s: %w", tmp, target, err)
	}
	removeStaleWindowsAsideFiles(c, target)
	return nil
}

func writeSFTPFile(c *sftp.Client, remotePath string, data []byte) error {
	f, err := c.Create(remotePath)
	if err != nil {
		return fmt.Errorf("sftp: create %s: %w", remotePath, err)
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return fmt.Errorf("sftp: write %s: %w", remotePath, err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("sftp: close %s: %w", remotePath, err)
	}
	return nil
}

// removeStaleWindowsAsideFiles deletes earlier aside copies of target. A copy
// that a running process still uses cannot be deleted and stays for a later
// replacement.
func removeStaleWindowsAsideFiles(c *sftp.Client, target string) {
	dir, base := path.Split(target)
	entries, err := c.ReadDir(path.Clean(dir))
	if err != nil {
		return
	}
	prefix := base + ".old-"
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), prefix) {
			_ = c.Remove(path.Join(dir, entry.Name()))
		}
	}
}
