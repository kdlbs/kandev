//go:build unix

package launcher

import (
	"testing"
)

func TestBuildSysProcAttr_IsolatesAgentctlFromTerminalInterrupt(t *testing.T) {
	attr := buildSysProcAttr(false)
	if !attr.Setsid {
		t.Error("Setsid must be true: standalone agentctl must own an isolated session for safe cleanup")
	}
}
