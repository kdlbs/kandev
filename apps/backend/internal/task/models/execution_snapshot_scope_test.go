package models

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestExecutionSandboxSnapshotValidatesOwnershipScope(t *testing.T) {
	digest := strings.Repeat("a", 64)
	base := ExecutionSandboxSnapshot{AttemptID: "attempt", Generation: 1, InstallationID: "i", ProviderKey: "k", ProviderVersion: "1", RuntimeVersion: "1", RevisionID: "r", RevisionNumber: 1, CeilingRevisionID: "c", SelectionSource: "agent", PackageDigest: digest, RuntimeDigest: digest, PolicyDigest: digest, CeilingDigest: digest}
	for scope, valid := range map[string]bool{"": true, "kernel_container": true, "process_tree": true, "host": false} {
		snapshot := base
		snapshot.OwnershipScope = scope
		require.Equal(t, valid, snapshot.valid(), scope)
	}
}
