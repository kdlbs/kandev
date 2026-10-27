//go:build linux

package sandboxruntime

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	contract "github.com/kandev/kandev/internal/common/sandbox"
	"github.com/stretchr/testify/require"
)

// Runtime fixtures grant launcher dependencies explicitly, as an operator must.
func runtimeEnvToolGrants(t *testing.T) []contract.FilesystemGrant {
	t.Helper()
	tool, err := exec.LookPath("env")
	require.NoError(t, err)
	canonical, err := filepath.EvalSymlinks(tool)
	require.NoError(t, err)
	grants := []contract.FilesystemGrant{{Path: canonical}}
	output, err := exec.Command("ldd", tool).Output()
	require.NoError(t, err)
	for _, field := range strings.Fields(string(output)) {
		if !filepath.IsAbs(field) {
			continue
		}
		dependency, err := filepath.EvalSymlinks(field)
		require.NoError(t, err)
		grants = append(grants, contract.FilesystemGrant{Path: dependency})
	}
	return grants
}
