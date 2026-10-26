package nomad

import (
	"strings"
	"testing"
)

const testHostKey = "SHA256:abcdefghijklmnopqrstuvwxyz0123456789ABCDEFG"

func nodeWithMeta(meta map[string]string) *NodeRecord { return &NodeRecord{ID: "n1", Meta: meta} }

func sshMeta() map[string]string {
	return map[string]string{MetaAttach: "ssh", MetaStateRoot: "/var/kandev", MetaSSHHost: "worker-b.example", MetaSSHPort: "22", MetaSSHHostKeySHA256: testHostKey}
}

func localMeta() map[string]string {
	return map[string]string{MetaAttach: "local", MetaStateRoot: "/var/kandev", MetaHostIdentity: strings.Repeat("a", 64)}
}

func TestParseNodeContract_LocalValid(t *testing.T) {
	c, err := ParseNodeContract(nodeWithMeta(localMeta()))
	if err != nil || c.Attach != AttachLocal || c.HostIdentitySHA256 != strings.Repeat("a", 64) || c.SSHHost != "" {
		t.Fatalf("%+v %v", c, err)
	}
	m := localMeta()
	m[MetaHostIdentity] = strings.Repeat("A", 64)
	if _, err := ParseNodeContract(nodeWithMeta(m)); err == nil {
		t.Fatal("uppercase identity accepted")
	}
}

func TestParseNodeContract_SSHValid(t *testing.T) {
	c, err := ParseNodeContract(nodeWithMeta(sshMeta()))
	if err != nil || c.SSHHost != "worker-b.example" || c.SSHPort != 22 || c.SSHHostKeySHA256 != testHostKey {
		t.Fatalf("%+v %v", c, err)
	}
	m := sshMeta()
	m[MetaSSHPort] = "70000"
	if _, err := ParseNodeContract(nodeWithMeta(m)); err == nil {
		t.Fatal("bad port accepted")
	}
}

func TestParseNodeContract_MissingAttach(t *testing.T) {
	m := sshMeta()
	delete(m, MetaAttach)
	if _, err := ParseNodeContract(nodeWithMeta(m)); err == nil {
		t.Fatal("missing attach accepted")
	}
	if _, err := ParseNodeContract(nil); err == nil {
		t.Fatal("nil node accepted")
	}
}

func TestParseNodeContract_RelativeStateRoot(t *testing.T) {
	for _, root := range []string{"relative/x", "/a/../b", "/a/", ""} {
		m := sshMeta()
		m[MetaStateRoot] = root
		if _, err := ParseNodeContract(nodeWithMeta(m)); err == nil {
			t.Fatalf("root %q accepted", root)
		}
	}
}

func TestParseNodeContract_SSHMissingKey(t *testing.T) {
	for _, key := range []string{MetaSSHHost, MetaSSHPort, MetaSSHHostKeySHA256} {
		m := sshMeta()
		delete(m, key)
		if _, err := ParseNodeContract(nodeWithMeta(m)); err == nil {
			t.Fatalf("missing %s accepted", key)
		}
	}
	m := sshMeta()
	m[MetaSSHHostKeySHA256] = testHostKey + "="
	if _, err := ParseNodeContract(nodeWithMeta(m)); err == nil {
		t.Fatal("padded fingerprint accepted")
	}
}

func TestParseNodeContract_MixedFields(t *testing.T) {
	m := sshMeta()
	m[MetaHostIdentity] = strings.Repeat("a", 64)
	if _, err := ParseNodeContract(nodeWithMeta(m)); err == nil {
		t.Fatal("ssh node with host identity accepted")
	}
	l := localMeta()
	l[MetaSSHHost] = "secret-host-value"
	_, err := ParseNodeContract(nodeWithMeta(l))
	if err == nil || strings.Contains(err.Error(), "secret-host-value") {
		t.Fatalf("local node with ssh keys: %v", err)
	}
}

func TestNodeContractDigest_Stable(t *testing.T) {
	a, _ := ParseNodeContract(nodeWithMeta(sshMeta()))
	b, _ := ParseNodeContract(nodeWithMeta(sshMeta()))
	d := NodeContractDigest(a)
	if d != NodeContractDigest(b) || !digestPattern.MatchString(d) {
		t.Fatalf("unstable digest %s", d)
	}
	b.SSHPort = 2222
	if NodeContractDigest(b) == d {
		t.Fatal("digest ignores port")
	}
	l, _ := ParseNodeContract(nodeWithMeta(localMeta()))
	if NodeContractDigest(l) == d {
		t.Fatal("digest collides across attach modes")
	}
}
