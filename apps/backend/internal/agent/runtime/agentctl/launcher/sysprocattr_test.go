//go:build unix

package launcher

import (
	"testing"
)

func TestBuildSysProcAttr_IsolatesAgentctlFromTerminalInterrupt(t *testing.T) {
	attr := buildSysProcAttr()
	if !attr.Setpgid {
		t.Error("Setpgid must be true: standalone agentctl should not receive terminal Ctrl+C directly")
	}
}
