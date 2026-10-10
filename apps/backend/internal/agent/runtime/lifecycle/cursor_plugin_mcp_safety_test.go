package lifecycle

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWriteCursorProjectMCPAndOwnershipPropagatesOwnershipFailure(t *testing.T) {
	projectDir := t.TempDir()
	configPath := filepath.Join(projectDir, "mcp.json")
	ownershipPath := filepath.Join(projectDir, ".kandev-mcp-imports.json")
	require.NoError(t, os.Mkdir(ownershipPath, 0o700))

	err := writeCursorProjectMCPAndOwnership(
		projectDir,
		configPath,
		ownershipPath,
		map[string]json.RawMessage{},
		map[string]any{"plugin-fixture-server": map[string]any{"url": "https://fixture.example/mcp"}},
		map[string]string{"plugin-fixture-server": "fingerprint"},
		false,
	)
	require.ErrorContains(t, err, "write Cursor MCP import ownership")
}
