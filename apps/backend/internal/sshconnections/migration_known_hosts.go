package sshconnections

import (
	"bufio"
	"encoding/base64"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/kandev/kandev/internal/sshtransport"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

type migrationKnownHostFile struct {
	strict ssh.HostKeyCallback
	lines  map[int]migrationKnownHostLine
}

type migrationKnownHostLine struct {
	keyType string
	key     string
	dynamic bool
}

func migrationKnownHostsCallback() (ssh.HostKeyCallback, error) {
	file, err := loadMigrationKnownHosts()
	if err != nil {
		return nil, ErrTrust
	}
	return file.checkHostKey, nil
}

func (f migrationKnownHostFile) checkHostKey(hostname string, remote net.Addr, key ssh.PublicKey) error {
	if _, certificate := key.(*ssh.Certificate); certificate {
		return ErrTrust
	}
	if err := f.strict(hostname, remote, key); err != nil {
		return err
	}
	for _, candidate := range f.candidates(hostname, remote) {
		if ssh.FingerprintSHA256(candidate.Key) == ssh.FingerprintSHA256(key) && f.isRawKey(candidate) {
			return nil
		}
	}
	return ErrTrust
}

func loadMigrationKnownHosts() (migrationKnownHostFile, error) {
	home, _ := os.UserHomeDir()
	if home == "" {
		return migrationKnownHostFile{}, ErrTrust
	}
	path := filepath.Join(home, ".ssh", "known_hosts")
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return migrationKnownHostFile{}, ErrTrust
	}
	strict, err := knownhosts.New(path)
	if err != nil {
		return migrationKnownHostFile{}, err
	}
	lines, err := migrationRawHostKeyLines(path)
	if err != nil {
		return migrationKnownHostFile{}, err
	}
	after, err := os.Stat(path)
	if err != nil || !os.SameFile(info, after) || info.Size() != after.Size() || info.ModTime() != after.ModTime() {
		return migrationKnownHostFile{}, ErrTrust
	}
	return migrationKnownHostFile{strict: strict, lines: lines}, nil
}

// The library's raw-key callback also looks at certificate-authority lines.
// Only entries whose original marker and key identify an ordinary host key
// are eligible to become an exact fingerprint pin.
func migrationRawHostKeyLines(path string) (map[int]migrationKnownHostLine, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()
	lines := make(map[int]migrationKnownHostLine)
	scanner := bufio.NewScanner(file)
	for number := 1; scanner.Scan(); number++ {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 3 || strings.HasPrefix(fields[0], "#") || strings.HasPrefix(fields[0], "@") {
			continue
		}
		lines[number] = migrationKnownHostLine{
			keyType: fields[1], key: fields[2], dynamic: strings.ContainsAny(fields[0], "|*?!"),
		}
	}
	return lines, scanner.Err()
}

func (f migrationKnownHostFile) isRawKey(candidate knownhosts.KnownKey) bool {
	line := f.lines[candidate.Line]
	return line.keyType == candidate.Key.Type() && line.key == base64.StdEncoding.EncodeToString(candidate.Key.Marshal())
}

// migrationProbeKey enumerates candidate keys through the library's matcher
// without generating key material. It cannot match a valid host key.
type migrationProbeKey struct{ ssh.PublicKey }

func (migrationProbeKey) Type() string    { return "kandev-invalid-host-key" }
func (migrationProbeKey) Marshal() []byte { return nil }

func (f migrationKnownHostFile) candidates(hostname string, remote net.Addr) []knownhosts.KnownKey {
	var candidates *knownhosts.KeyError
	if err := f.strict(hostname, remote, migrationProbeKey{}); errors.As(err, &candidates) {
		return candidates.Want
	}
	return nil
}

type migrationHostAddr string

func (migrationHostAddr) Network() string  { return "tcp" }
func (a migrationHostAddr) String() string { return string(a) }

func migrationBastionTrust(target *sshtransport.SSHTarget) (fingerprint, publicKey string, validatePresented bool) {
	file, err := loadMigrationKnownHosts()
	if err != nil {
		return "", "", false
	}
	addr := net.JoinHostPort(target.Host, strconv.Itoa(target.Port))
	remote := migrationHostAddr(addr)
	var trusted []knownhosts.KnownKey
	for _, candidate := range file.candidates(addr, remote) {
		if file.isRawKey(candidate) && file.strict(addr, remote, candidate.Key) == nil {
			trusted = append(trusted, candidate)
		}
	}
	if len(trusted) == 0 {
		return "", "", false
	}
	// A hash, pattern or multiple matches defer selection to first authorized use.
	if len(trusted) != 1 || file.lines[trusted[0].Line].dynamic {
		return "", "", true
	}
	key := trusted[0].Key
	return ssh.FingerprintSHA256(key), string(ssh.MarshalAuthorizedKey(key)), false
}
