package main

import (
	"strings"
	"testing"
)

func TestBuildExtractScript(t *testing.T) {
	script := buildExtractScript(12345)

	if !strings.Contains(script, "--backend-port 12345") {
		t.Errorf("expected --backend-port 12345 in script, got:\n%s", script)
	}
	if !strings.Contains(script, "rm -rf /data") {
		t.Errorf("expected rm -rf /data in script")
	}
	if !strings.Contains(script, "KANDEV_MOCK_AGENT=only") {
		t.Errorf("expected KANDEV_MOCK_AGENT=only in script")
	}
	if !strings.Contains(script, "KANDEV_WEB_DIST_DIR=/app/apps/web/dist") {
		t.Errorf("expected KANDEV_WEB_DIST_DIR to point at packaged Vite dist")
	}
	if !strings.Contains(script, "ln -sf /usr/local/lib/kandev-cli/bin/cli.js /usr/local/bin/kandev") {
		t.Errorf("expected kandev cli symlink in script")
	}
	if !strings.Contains(script, "exec kandev start") {
		t.Errorf("expected script to launch through kandev start")
	}
	if !strings.Contains(script, "--headless") {
		t.Errorf("expected headless CLI launch")
	}
	if strings.Contains(script, "nohup node") {
		t.Errorf("script should not start web outside the CLI supervisor")
	}
	if strings.Contains(script, ".next") {
		t.Errorf("script should not refer to Next.js build output")
	}
	if strings.Contains(script, "/app/apps/backend/bin/kandev >") {
		t.Errorf("script should not launch the backend binary directly")
	}
}
