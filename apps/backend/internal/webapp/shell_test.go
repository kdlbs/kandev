package webapp

import (
	"strings"
	"testing"
	"testing/fstest"
)

func TestRenderShellInjectsBootPayloadBeforeHeadClose(t *testing.T) {
	t.Parallel()

	assets := fstest.MapFS{
		"index.html": {
			Data: []byte("<!doctype html><html><head><title>Kandev</title></head><body></body></html>"),
		},
	}
	payload := NewBootPayload(
		ClassifyRoute("/t/task-1"),
		RuntimeConfig{APIPrefix: "/api/v1", WebSocketPath: "/ws"},
		map[string]any{"title": "Task"},
	)

	html, err := RenderShell(assets, "index.html", payload)
	if err != nil {
		t.Fatalf("RenderShell: %v", err)
	}

	got := string(html)
	scriptIdx := strings.Index(got, bootPayloadGlobal)
	headCloseIdx := strings.Index(got, "</head>")
	if scriptIdx < 0 {
		t.Fatalf("rendered shell missing boot payload script: %s", got)
	}
	if headCloseIdx < 0 || scriptIdx > headCloseIdx {
		t.Fatalf("boot payload script should be injected before </head>: %s", got)
	}
	if !strings.Contains(got, `"taskId":"task-1"`) {
		t.Fatalf("rendered shell missing route params: %s", got)
	}
}

func TestBootPayloadScriptEscapesScriptTerminators(t *testing.T) {
	t.Parallel()

	payload := NewBootPayload(
		ClassifyRoute("/"),
		RuntimeConfig{},
		map[string]any{"title": "</script><script>alert(1)</script>"},
	)

	script, err := BootPayloadScript(payload)
	if err != nil {
		t.Fatalf("BootPayloadScript: %v", err)
	}
	if strings.Contains(string(script), "</script><script>") {
		t.Fatalf("script contains unescaped script terminator: %s", script)
	}
	if !strings.HasPrefix(string(script), "<script>window.__KANDEV_BOOT_PAYLOAD__=") {
		t.Fatalf("script has unexpected prefix: %s", script)
	}
}

func TestRenderShellPrependsScriptWhenHeadCloseIsMissing(t *testing.T) {
	t.Parallel()

	assets := fstest.MapFS{
		"index.html": {Data: []byte("<div id=\"root\"></div>")},
	}

	html, err := RenderShell(assets, "index.html", NewBootPayload(ClassifyRoute("/"), RuntimeConfig{}, nil))
	if err != nil {
		t.Fatalf("RenderShell: %v", err)
	}

	if !strings.HasPrefix(string(html), "<script>") {
		t.Fatalf("expected script prefix for shell without </head>: %s", html)
	}
}

func TestBytesCapacityReturnsZeroOnOverflow(t *testing.T) {
	t.Parallel()

	if got := bytesCapacity(3, 4, 5); got != 12 {
		t.Fatalf("bytesCapacity returned %d, want 12", got)
	}
	if got := bytesCapacity(maxInt, 1); got != 0 {
		t.Fatalf("bytesCapacity overflow returned %d, want 0", got)
	}
}
