//go:build e2e

package e2e

import (
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agent/agents"
)

func TestACPCommandRejectsEmptySecondaryArg(t *testing.T) {
	if os.Getenv("KANDEV_TEST_ACP_COMMAND_EMPTY") == "1" {
		acpCommand(t, argvAgent{command: agents.NewCommand("agent", "")})
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=^TestACPCommandRejectsEmptySecondaryArg$")
	cmd.Env = append(os.Environ(), "KANDEV_TEST_ACP_COMMAND_EMPTY=1")
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("acpCommand accepted an empty secondary argument; output: %s", output)
	}
	if !strings.Contains(string(output), "empty; it cannot round-trip") {
		t.Fatalf("acpCommand failed for an unexpected reason; output: %s", output)
	}
}

type argvAgent struct {
	agents.Agent
	command agents.Command
}

func (a argvAgent) BuildCommand(agents.CommandOptions) agents.Command {
	return a.command
}
