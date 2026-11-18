package mcpconfig

import (
	p "github.com/kandev/kandev/internal/mcp/profile"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestBuiltinSelectionHashIgnoresOrderAndPreservesInput(t *testing.T) {
	ordered := []p.PackageID{p.PackageCore, p.PackageRichOutput, p.PackageDiagnostics}
	reversed := []p.PackageID{p.PackageDiagnostics, p.PackageCore, p.PackageRichOutput}
	require.Equal(t, BuiltinSelectionHash(ordered), BuiltinSelectionHash(reversed))
	require.Equal(t, []p.PackageID{p.PackageDiagnostics, p.PackageCore, p.PackageRichOutput}, reversed)
	require.NotEqual(t, BuiltinSelectionHash(ordered), BuiltinSelectionHash([]p.PackageID{p.PackageCore, p.PackageConfiguration}))
}
