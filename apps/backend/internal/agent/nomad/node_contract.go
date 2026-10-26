package nomad

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// Node metadata keys a worker publishes to join the pool.
const (
	MetaAttach           = "kandev.attach"
	MetaStateRoot        = "kandev.state_root"
	MetaSSHHost          = "kandev.ssh_host"
	MetaSSHPort          = "kandev.ssh_port"
	MetaSSHHostKeySHA256 = "kandev.ssh_host_key_sha256"
	MetaHostIdentity     = "kandev.host_identity_sha256"

	AttachLocal = "local"
	AttachSSH   = "ssh"
)

var (
	sshHostKeyPattern  = regexp.MustCompile(`^SHA256:[A-Za-z0-9+/]{43}$`)
	hostIdentityDigest = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

// NodeContract is the validated transport and storage contract of one node.
type NodeContract struct {
	Attach             string
	StateRoot          string
	SSHHost            string
	SSHPort            int
	SSHHostKeySHA256   string
	HostIdentitySHA256 string
}

// ParseNodeContract validates the kandev.* metadata of a placed node. Errors
// name metadata keys only, never their values.
func ParseNodeContract(node *NodeRecord) (NodeContract, error) {
	if node == nil {
		return NodeContract{}, errors.New("nomad node record is missing")
	}
	meta := node.Meta
	var c NodeContract
	c.Attach = meta[MetaAttach]
	if c.Attach != AttachLocal && c.Attach != AttachSSH {
		return c, fmt.Errorf("node metadata %q must be %q or %q", MetaAttach, AttachLocal, AttachSSH)
	}
	root := meta[MetaStateRoot]
	if root == "" || !filepath.IsAbs(root) || filepath.Clean(root) != root {
		return c, fmt.Errorf("node metadata %q must be a normalized absolute path", MetaStateRoot)
	}
	c.StateRoot = root
	if c.Attach == AttachLocal {
		return parseLocalContract(c, meta)
	}
	return parseSSHContract(c, meta)
}

func parseLocalContract(c NodeContract, meta map[string]string) (NodeContract, error) {
	for _, key := range []string{MetaSSHHost, MetaSSHPort, MetaSSHHostKeySHA256} {
		if _, ok := meta[key]; ok {
			return c, fmt.Errorf("node metadata %q must not be set on a local node", key)
		}
	}
	if !hostIdentityDigest.MatchString(meta[MetaHostIdentity]) {
		return c, fmt.Errorf("node metadata %q must be 64 lowercase hex characters", MetaHostIdentity)
	}
	c.HostIdentitySHA256 = meta[MetaHostIdentity]
	return c, nil
}

func parseSSHContract(c NodeContract, meta map[string]string) (NodeContract, error) {
	if _, ok := meta[MetaHostIdentity]; ok {
		return c, fmt.Errorf("node metadata %q must not be set on an ssh node", MetaHostIdentity)
	}
	host := meta[MetaSSHHost]
	if host == "" || strings.IndexFunc(host, func(r rune) bool { return r <= ' ' || r == 0x7f || r == '@' }) >= 0 {
		return c, fmt.Errorf("node metadata %q must be a host name without whitespace", MetaSSHHost)
	}
	port, err := strconv.Atoi(meta[MetaSSHPort])
	if err != nil || port < 1 || port > 65535 {
		return c, fmt.Errorf("node metadata %q must be a port from 1 to 65535", MetaSSHPort)
	}
	if !sshHostKeyPattern.MatchString(meta[MetaSSHHostKeySHA256]) {
		return c, fmt.Errorf("node metadata %q must be an unpadded SHA256 host key fingerprint", MetaSSHHostKeySHA256)
	}
	c.SSHHost, c.SSHPort, c.SSHHostKeySHA256 = host, port, meta[MetaSSHHostKeySHA256]
	return c, nil
}

// NodeContractDigest is the domain-separated SHA-256 of the canonical contract
// fields, used to detect metadata drift between launch and recovery.
func NodeContractDigest(c NodeContract) string {
	canonical := fmt.Sprintf("kandev.nomad.node-contract.v1\x00%s\x00%s\x00%s\x00%d\x00%s\x00%s",
		c.Attach, c.StateRoot, c.SSHHost, c.SSHPort, c.SSHHostKeySHA256, c.HostIdentitySHA256)
	digest := sha256.Sum256([]byte(canonical))
	return hex.EncodeToString(digest[:])
}
