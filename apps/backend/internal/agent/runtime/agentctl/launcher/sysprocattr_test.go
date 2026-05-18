//go:build !windows

package launcher

import (
	"testing"
)

func TestBuildSysProcAttr_NoSetpgid(t *testing.T) {
	attr := buildSysProcAttr()
	if attr.Setpgid {
		t.Error("Setpgid must be false: standalone agentctl should share the backend's process group")
	}
}
