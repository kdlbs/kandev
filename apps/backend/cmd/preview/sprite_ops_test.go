package main

import (
	"strings"
	"testing"
)

func TestBuildExtractScript(t *testing.T) {
	script := buildExtractScript(12345)

	if !strings.Contains(script, "KANDEV_SERVER_PORT=12345") {
		t.Errorf("expected KANDEV_SERVER_PORT=12345 in script, got:\n%s", script)
	}
	if !strings.Contains(script, "rm -rf /data") {
		t.Errorf("expected rm -rf /data in script")
	}
	if !strings.Contains(script, "KANDEV_MOCK_AGENT=only") {
		t.Errorf("expected KANDEV_MOCK_AGENT=only in script")
	}
}
