//go:build unix

package utility

import (
	"context"
	"go.uber.org/zap"
	"path/filepath"
	"testing"

	"github.com/kandev/kandev/internal/common/launchwrapper"
	"github.com/stretchr/testify/require"
)

func TestNodePolicyRejectsUnwrappedUtility(t *testing.T) {
	t.Setenv(launchwrapper.NodePolicyEnv, filepath.Join(t.TempDir(), "missing-node-policy"))
	_, err := prepareACPLaunch(&InferenceConfigDTO{WorkDir: t.TempDir()}, "/bin/echo", []string{"/bin/echo", "never-start"}, nil)
	require.ErrorIs(t, err, launchwrapper.ErrNodePolicy)
}

func TestNodePolicyRejectsNativeCodexUtility(t *testing.T) {
	t.Setenv(launchwrapper.NodePolicyEnv, filepath.Join(t.TempDir(), "missing-node-policy"))
	executor := &CodexAppServerInferenceExecutor{logger: zap.NewNop()}
	_, cleanup, _, err := executor.start(context.Background(), "/bin/echo", []string{"never-start"}, &InferenceConfigDTO{WorkDir: t.TempDir()})
	if cleanup != nil {
		cleanup()
	}
	require.ErrorIs(t, err, launchwrapper.ErrNodePolicy)
}
