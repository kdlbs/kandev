//go:build linux

package sandbox

import (
	"github.com/stretchr/testify/require"
	"os"
	"path/filepath"
	"testing"
)

func TestAuditSourceHasStableBirthIdentity(t *testing.T) {
	state, scratch, source := auditArchiveFixture(t)
	require.True(t, validRuntimeDirectory(state))
	require.True(t, validRuntimeDirectory(scratch))
	require.NoError(t, source.ValidateForProtocol(2))
	path := filepath.Join(state, "audit", "events.ndjson")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	require.NoError(t, err)
	defer func() { require.NoError(t, file.Close()) }()
	identity, err := auditFileIdentity(file)
	require.NoError(t, err)
	require.Len(t, identity, 64)
	_, err = file.WriteString("append\n")
	require.NoError(t, err)
	next, err := auditFileIdentity(file)
	require.NoError(t, err)
	require.Equal(t, identity, next)
}
