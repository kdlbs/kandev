package sandbox

import (
	"testing"

	contract "github.com/kandev/kandev/internal/common/sandbox"
	"github.com/stretchr/testify/require"
)

func TestCapturedAuditDeclarationsKeepLegacySnapshotsAndBoundNewReads(t *testing.T) {
	require.True(t, validAuditDeclarations(nil, 1), "legacy snapshots must keep the same representation")
	source := contract.RuntimeSource{Key: "fixture.audit.v1", Kind: "audit", Directory: "audit", Suffix: "events.ndjson", MaxDepth: 2, MaxFiles: 16, MaxBytes: 32 << 20}
	require.True(t, validAuditDeclarations([]contract.RuntimeSource{source}, 2))
	require.False(t, validAuditDeclarations([]contract.RuntimeSource{source}, 1))
	require.False(t, validAuditDeclarations([]contract.RuntimeSource{source, source}, 2))
	for _, change := range []string{"escape", "identity", "oversized", "unbounded"} {
		changed := source
		switch change {
		case "escape":
			changed.Directory = "../outside"
		case "identity":
			changed.Kind = "identity"
		case "oversized":
			changed.MaxBytes = 129 << 20
		case "unbounded":
			changed.MaxDepth = 0
		}
		require.False(t, validAuditDeclarations([]contract.RuntimeSource{changed}, 2), change)
	}
}
