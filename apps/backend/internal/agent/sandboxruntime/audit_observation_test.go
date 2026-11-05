package sandboxruntime

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAuditSignerFactRequiresBothSignatureAndCurrentTrust(t *testing.T) {
	require.Equal(t, "trusted", auditSignerFact("verified", true))
	require.Equal(t, "untrusted", auditSignerFact("verified", false), "revocation cannot rewrite a mathematically valid signature")
	for _, signature := range []string{"unchecked", "unsigned", "untrusted", "invalid", "unsupported"} {
		require.NotEqual(t, "trusted", auditSignerFact(signature, true), "configured trust cannot repair an absent or invalid signature")
	}
}
