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

func TestBootPayloadScriptSetsDebugGlobalBeforeBootPayload(t *testing.T) {
	t.Parallel()

	payload := NewBootPayload(
		ClassifyRoute("/"),
		RuntimeConfig{Debug: true},
		nil,
	)

	script, err := BootPayloadScript(payload)
	if err != nil {
		t.Fatalf("BootPayloadScript: %v", err)
	}
	got := string(script)
	debugIdx := strings.Index(got, "window.__KANDEV_DEBUG=true;")
	payloadIdx := strings.Index(got, bootPayloadGlobal)
	if debugIdx < 0 {
		t.Fatalf("script missing debug global assignment: %s", got)
	}
	if payloadIdx < 0 || debugIdx > payloadIdx {
		t.Fatalf("debug global should be assigned before boot payload: %s", got)
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

func TestRenderShellSetsHTMLLangFromLocale(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		locale   string
		wantLang string
	}{
		{name: "default when empty", locale: "", wantLang: `lang="en"`},
		{name: "supported simplified chinese", locale: "zh-cn", wantLang: `lang="zh-cn"`},
		{name: "supported european portuguese", locale: "pt-pt", wantLang: `lang="pt-pt"`},
		{name: "supported pseudo", locale: "pseudo", wantLang: `lang="pseudo"`},
		{name: "unknown coerces to en", locale: "klingon", wantLang: `lang="en"`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assets := fstest.MapFS{
				"index.html": {
					Data: []byte(`<!doctype html><html lang="en"><head><title>Kandev</title></head><body></body></html>`),
				},
			}
			payload := NewBootPayload(ClassifyRoute("/"), RuntimeConfig{Locale: tc.locale}, nil)
			html, err := RenderShell(assets, "index.html", payload)
			if err != nil {
				t.Fatalf("RenderShell: %v", err)
			}
			got := string(html)
			if !strings.Contains(got, tc.wantLang) {
				t.Fatalf("expected %q in shell, got: %s", tc.wantLang, got)
			}
			// Exactly one lang attribute on the <html> tag.
			if n := strings.Count(got, "lang="); n != 1 {
				t.Fatalf("expected one lang attribute, found %d: %s", n, got)
			}
		})
	}
}

func TestRenderShellInsertsLangWhenAbsent(t *testing.T) {
	t.Parallel()

	assets := fstest.MapFS{
		"index.html": {Data: []byte(`<!doctype html><html><head></head><body></body></html>`)},
	}
	payload := NewBootPayload(ClassifyRoute("/"), RuntimeConfig{Locale: "pseudo"}, nil)
	html, err := RenderShell(assets, "index.html", payload)
	if err != nil {
		t.Fatalf("RenderShell: %v", err)
	}
	if !strings.Contains(string(html), `<html lang="pseudo">`) {
		t.Fatalf("expected inserted lang attribute, got: %s", html)
	}
}

func TestRenderShellRewritesTitleFromPrefix(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		prefix    string
		wantTitle string
	}{
		{name: "prefix set", prefix: "TEST", wantTitle: "<title>TEST Kandev</title>"},
		{name: "prefix trimmed", prefix: "  TEST  ", wantTitle: "<title>TEST Kandev</title>"},
		{name: "empty prefix keeps shell title", prefix: "", wantTitle: "<title>Kandev</title>"},
		{name: "blank prefix keeps shell title", prefix: "   ", wantTitle: "<title>Kandev</title>"},
		{
			name:      "html special characters are escaped",
			prefix:    `</title><script>alert(1)</script>`,
			wantTitle: "<title>&lt;/title&gt;&lt;script&gt;alert(1)&lt;/script&gt; Kandev</title>",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assets := fstest.MapFS{
				"index.html": {
					Data: []byte(`<!doctype html><html lang="en"><head><title>Kandev</title></head><body></body></html>`),
				},
			}
			payload := NewBootPayload(ClassifyRoute("/"), RuntimeConfig{TitlePrefix: tc.prefix}, nil)
			html, err := RenderShell(assets, "index.html", payload)
			if err != nil {
				t.Fatalf("RenderShell: %v", err)
			}
			if got := string(html); !strings.Contains(got, tc.wantTitle) {
				t.Fatalf("expected %q in shell, got: %s", tc.wantTitle, got)
			}
		})
	}
}

func TestRenderShellLeavesShellWithoutTitleUntouched(t *testing.T) {
	t.Parallel()

	assets := fstest.MapFS{
		"index.html": {Data: []byte(`<!doctype html><html lang="en"><head></head><body></body></html>`)},
	}
	payload := NewBootPayload(ClassifyRoute("/"), RuntimeConfig{TitlePrefix: "TEST"}, nil)
	html, err := RenderShell(assets, "index.html", payload)
	if err != nil {
		t.Fatalf("RenderShell: %v", err)
	}
	if strings.Contains(string(html), "<title") {
		t.Fatalf("expected no title element to be created, got: %s", html)
	}
}

func TestComposeTitle(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct{ in, want string }{
		{"", "Kandev"},
		{"   ", "Kandev"},
		{"TEST", "TEST Kandev"},
		{"  staging  ", "staging Kandev"},
	} {
		if got := ComposeTitle(tc.in); got != tc.want {
			t.Fatalf("ComposeTitle(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestNormalizeLocale(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct{ in, want string }{
		{"en", "en"},
		{"zh-cn", "zh-cn"},
		{"zh-CN", "zh-cn"},
		{"pt-pt", "pt-pt"},
		{"pt-PT", "pt-pt"},
		{"pseudo", "pseudo"},
		{"", "en"},
		{"fr", "en"},
	} {
		if got := NormalizeLocale(tc.in); got != tc.want {
			t.Fatalf("NormalizeLocale(%q) = %q, want %q", tc.in, got, tc.want)
		}
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
