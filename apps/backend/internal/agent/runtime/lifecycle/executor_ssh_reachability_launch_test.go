package lifecycle

import (
	"context"
	"strconv"
	"strings"
	"testing"
)

// @covers AC-EXECUTORS-SSH-REACHABILITY-003.2
//
// A launch's own dial failure must be attributed with the target host and
// the same reason ClassifyDialError would produce for a standalone probe —
// not read from any stored record, since the stored record can predate this
// attempt by a full interval.
func TestSSHExecutorCreateInstanceDialErrorNamesHostAndReason(t *testing.T) {
	server := newFakeSSHServer(t, nil)
	newTestHomeDir(t)
	identity := writeTestIdentityFile(t)

	exec := NewSSHExecutor(nil, nil, NewAgentctlResolver(newTestLogger()), newTestLogger())
	t.Cleanup(func() { _ = exec.Close() })

	_, err := exec.CreateInstance(context.Background(), &ExecutorCreateRequest{
		InstanceID: "instance-1",
		Metadata: map[string]interface{}{
			MetadataKeySSHHost:            "127.0.0.1",
			MetadataKeySSHPort:            strconv.Itoa(server.port()),
			MetadataKeySSHUser:            "kandev",
			MetadataKeySSHHostFingerprint: "SHA256:not-the-server-key",
			MetadataKeySSHIdentitySource:  string(SSHIdentitySourceFile),
			MetadataKeySSHIdentityFile:    identity,
		},
	})

	if err == nil {
		t.Fatal("expected a dial failure, got nil error")
	}
	if !strings.Contains(err.Error(), "127.0.0.1") {
		t.Fatalf("error = %q, want it to name the target host", err.Error())
	}
	if !strings.Contains(err.Error(), string(SSHReachabilityReasonHostKey)) {
		t.Fatalf("error = %q, want it to carry the classified reason %q", err.Error(), SSHReachabilityReasonHostKey)
	}
}
