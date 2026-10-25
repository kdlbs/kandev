package sandboxruntime

import (
	"context"
	"testing"

	policy "github.com/kandev/kandev/internal/agent/sandbox"
	"github.com/stretchr/testify/require"
)

func TestSandboxRuntimeUnknownStartRetainsArtifacts(t *testing.T) {
	p := &Prepared{startDispatched: true}
	require.ErrorIs(t, p.canRemoveArtifacts(), policy.ErrConflict)
	p.startDispatched = false
	require.NoError(t, p.canRemoveArtifacts())
}

func TestSandboxRuntimeRepeatedStartRejected(t *testing.T) {
	p := &Prepared{startDispatched: true}
	require.ErrorIs(t, p.MarkStart(context.Background()), policy.ErrConflict)
	p.startDispatched = false
	p.closed = true
	require.ErrorIs(t, p.MarkStart(context.Background()), policy.ErrConflict)
}
