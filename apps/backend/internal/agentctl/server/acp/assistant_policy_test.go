package acp

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	acpsdk "github.com/coder/acp-go-sdk"
	"github.com/stretchr/testify/require"
)

func TestAssistantReadOnlyHostOperations(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "canary")
	require.NoError(t, os.WriteFile(file, []byte("unchanged"), 0600))
	c := NewClient(WithWorkspaceRoot(root), WithRestrictedTools(true))
	_, err := c.WriteTextFile(context.Background(), acpsdk.WriteTextFileRequest{Path: file, Content: "mutated"})
	require.Error(t, err)
	_, err = c.CreateTerminal(context.Background(), acpsdk.CreateTerminalRequest{Command: "sh", Args: []string{"-c", "exit 0"}})
	require.Error(t, err)
	_, err = c.ReadTextFile(context.Background(), acpsdk.ReadTextFileRequest{Path: file})
	require.Error(t, err)
	data, err := os.ReadFile(file)
	require.NoError(t, err)
	require.Equal(t, "unchanged", string(data))
}
