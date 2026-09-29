package ssh

import (
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/kevinburke/ssh_config"
	"golang.org/x/crypto/ssh"
)

// Where a discovered identity came from. Both are fixed roots; neither is
// reachable from caller input.
const (
	SourceSSHDir    = "ssh_dir"
	SourceSSHConfig = "ssh_config"
)

// headerProbeBytes bounds the read used to decide whether a file is a private
// key at all. Every supported header form fits well inside this, so a large
// or binary file is rejected without being read into memory.
const headerProbeBytes = 4096

// privateKeyHeaders are the first-line banners that make a file a candidate.
var privateKeyHeaders = []string{
	"-----BEGIN OPENSSH PRIVATE KEY-----",
	"-----BEGIN RSA PRIVATE KEY-----",
	"-----BEGIN DSA PRIVATE KEY-----",
	"-----BEGIN EC PRIVATE KEY-----",
	"-----BEGIN PRIVATE KEY-----",
	"-----BEGIN ENCRYPTED PRIVATE KEY-----",
}

// excludedSSHDirNames are entries that live beside keys and never are one.
// Everything else still has to pass the header and parse checks.
var excludedSSHDirNames = map[string]bool{
	"config":           true,
	"known_hosts":      true,
	"known_hosts.old":  true,
	"authorized_keys":  true,
	"authorized_keys2": true,
	"environment":      true,
	"rc":               true,
}

// Identity is one private key file the backend could use as a `file` identity
// source. It carries no key material: only where the file is and what the
// backend was able to determine about it without disclosing its contents.
type Identity struct {
	Path        string `json:"path"`
	DisplayPath string `json:"display_path"`
	KeyType     string `json:"key_type,omitempty"`
	Encrypted   bool   `json:"encrypted"`
	Source      string `json:"source"`
}

// IdentitiesResponse is the body of GET /api/v1/ssh/identities.
type IdentitiesResponse struct {
	HomeDir    string     `json:"home_dir"`
	Identities []Identity `json:"identities"`
}

// DiscoverIdentities enumerates usable private keys under two fixed roots:
// the immediate contents of <home>/.ssh, and the IdentityFile values named by
// <home>/.ssh/config. It takes no caller-supplied path, so there is nothing to
// traverse with.
//
// An unreadable root yields no entries rather than an error. "No keys here"
// and "cannot look" are the same actionable outcome for the user, and telling
// them apart would disclose whether the directory exists.
func DiscoverIdentities(home string) IdentitiesResponse {
	identities := make([]Identity, 0, 4)
	seen := map[string]bool{}

	add := func(path, source string) {
		resolved, err := filepath.EvalSymlinks(path)
		if err != nil {
			resolved = path
		}
		if seen[resolved] {
			return
		}
		id, ok := classifyIdentity(path, home, source)
		if !ok {
			return
		}
		seen[resolved] = true
		identities = append(identities, id)
	}

	for _, name := range sortedSSHDirEntries(filepath.Join(home, ".ssh")) {
		add(filepath.Join(home, ".ssh", name), SourceSSHDir)
	}
	for _, path := range sshConfigIdentityFiles(home) {
		add(path, SourceSSHConfig)
	}

	return IdentitiesResponse{HomeDir: home, Identities: identities}
}

// sortedSSHDirEntries lists the regular-file names directly inside dir,
// alphabetically. The listing is not recursive: a key filed away in a
// subdirectory is reachable through the custom path instead.
func sortedSSHDirEntries(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || excludedSSHDirNames[name] || strings.HasSuffix(name, ".pub") {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// sshConfigIdentityFiles collects every IdentityFile value in the user's
// config, in file order. A config that fails to parse contributes nothing, so
// discovery degrades to the .ssh root rather than failing the request.
func sshConfigIdentityFiles(home string) []string {
	f, err := os.Open(filepath.Join(home, ".ssh", "config"))
	if err != nil {
		return nil
	}
	defer func() { _ = f.Close() }()

	cfg, err := ssh_config.Decode(f)
	if err != nil {
		return nil
	}

	var paths []string
	for _, host := range cfg.Hosts {
		for _, node := range host.Nodes {
			kv, ok := node.(*ssh_config.KV)
			if !ok || !strings.EqualFold(kv.Key, "IdentityFile") {
				continue
			}
			value := strings.Trim(strings.TrimSpace(kv.Value), `"`)
			if value == "" {
				continue
			}
			paths = append(paths, expandIdentityHome(value, home))
		}
	}
	return paths
}

// expandIdentityHome resolves a leading ~ against the same home discovery is
// reading, and makes a relative value absolute beneath it.
func expandIdentityHome(value, home string) string {
	switch {
	case value == "~":
		return home
	case strings.HasPrefix(value, "~/"):
		return filepath.Join(home, value[2:])
	case filepath.IsAbs(value):
		return value
	default:
		return filepath.Join(home, value)
	}
}

// classifyIdentity decides whether path is a private key and, if so, whether
// it is passphrase-protected and what type it is. It reads only a bounded
// prefix and returns no bytes from the file.
func classifyIdentity(path, home, source string) (Identity, bool) {
	info, err := os.Lstat(path)
	if err != nil {
		return Identity{}, false
	}
	if info.Mode()&os.ModeSymlink != 0 {
		if info, err = os.Stat(path); err != nil {
			return Identity{}, false
		}
	}
	if !info.Mode().IsRegular() {
		return Identity{}, false
	}

	head, err := readHead(path, headerProbeBytes)
	if err != nil || !hasPrivateKeyHeader(head) {
		return Identity{}, false
	}

	id := Identity{
		Path:        path,
		DisplayPath: displayPath(path, home),
		Source:      source,
	}

	// The prefix proves the banner; parsing needs the whole file. Only a key
	// that either parses or reports a missing passphrase is offered.
	data, err := os.ReadFile(path)
	if err != nil {
		return Identity{}, false
	}
	key, err := ssh.ParseRawPrivateKey(data)
	switch {
	case err == nil:
		id.KeyType = publicKeyType(key)
	case isPassphraseMissing(err):
		id.Encrypted = true
	default:
		return Identity{}, false
	}
	return id, true
}

// readHead returns at most n bytes from the start of path.
func readHead(path string, n int) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	buf := make([]byte, n)
	read, err := io.ReadFull(f, buf)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		return nil, err
	}
	return buf[:read], nil
}

func hasPrivateKeyHeader(head []byte) bool {
	first := strings.TrimSpace(strings.SplitN(string(head), "\n", 2)[0])
	for _, header := range privateKeyHeaders {
		if first == header {
			return true
		}
	}
	return false
}

func isPassphraseMissing(err error) bool {
	var missing *ssh.PassphraseMissingError
	return errors.As(err, &missing)
}

// publicKeyType derives the wire type name from a parsed private key. A key
// whose public half cannot be derived is still a valid identity, just an
// unlabelled one.
func publicKeyType(key any) string {
	signer, err := ssh.NewSignerFromKey(key)
	if err != nil {
		return ""
	}
	return signer.PublicKey().Type()
}

// displayPath renders path relative to home with a leading ~, matching how the
// user writes it in the form and in ~/.ssh/config.
func displayPath(path, home string) string {
	rel, err := filepath.Rel(home, path)
	if err != nil || strings.HasPrefix(rel, "..") {
		return path
	}
	return "~/" + filepath.ToSlash(rel)
}

// httpListIdentities serves GET /api/v1/ssh/identities.
func (h *Handler) httpListIdentities(c *gin.Context) {
	home, err := os.UserHomeDir()
	if err != nil {
		c.JSON(http.StatusOK, IdentitiesResponse{Identities: []Identity{}})
		return
	}
	c.JSON(http.StatusOK, DiscoverIdentities(home))
}
