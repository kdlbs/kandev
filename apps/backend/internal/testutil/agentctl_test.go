package testutil

import (
	"os"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBuildAgentctlOutsideModule(t *testing.T) {
	if os.Getenv("KANDEV_TEST_AGENTCTL_OUTSIDE_MODULE") == "1" {
		t.Chdir(t.TempDir())
		BuildAgentctl(t)
		return
	}
	for _, moduleMode := range []string{"on", "off"} {
		t.Run(moduleMode, func(t *testing.T) {
			cmd := exec.Command(os.Args[0], "-test.run=^TestBuildAgentctlOutsideModule$")
			cmd.Env = append(os.Environ(), "KANDEV_TEST_AGENTCTL_OUTSIDE_MODULE=1",
				"GOWORK=off", "GO111MODULE="+moduleMode)
			output, err := cmd.CombinedOutput()
			require.Error(t, err)
			require.Contains(t, string(output), "resolve agentctl test helper module: not inside a Go module")
		})
	}
}
