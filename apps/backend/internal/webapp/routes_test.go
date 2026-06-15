package webapp

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestClassifyRouteSPARoutes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		path       string
		wantRoute  RouteName
		wantParams map[string]string
	}{
		{name: "home", path: "/", wantRoute: RouteHome},
		{name: "tasks", path: "/tasks", wantRoute: RouteTasks},
		{name: "task detail", path: "/t/task-123", wantRoute: RouteTaskDetail, wantParams: map[string]string{"taskId": "task-123"}},
		{name: "office root", path: "/office", wantRoute: RouteOffice},
		{name: "office nested", path: "/office/agents/agent-1", wantRoute: RouteOffice},
		{name: "settings root", path: "/settings", wantRoute: RouteSettings},
		{name: "settings nested", path: "/settings/integrations/github", wantRoute: RouteSettings},
		{name: "github", path: "/github", wantRoute: RouteGitHub},
		{name: "gitlab", path: "/gitlab", wantRoute: RouteGitLab},
		{name: "jira", path: "/jira", wantRoute: RouteJira},
		{name: "linear", path: "/linear", wantRoute: RouteLinear},
		{name: "query stripped", path: "/tasks?view=all", wantRoute: RouteTasks},
		{name: "trailing slash normalized", path: "/settings/", wantRoute: RouteSettings},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := ClassifyRoute(tc.path)
			if got.Kind != RouteKindSPA {
				t.Fatalf("Kind = %q, want %q", got.Kind, RouteKindSPA)
			}
			if got.Route != tc.wantRoute {
				t.Fatalf("Route = %q, want %q", got.Route, tc.wantRoute)
			}
			assertParams(t, got.Params, tc.wantParams)
		})
	}
}

func TestClassifyRouteNonSPARoutes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		path     string
		wantKind RouteKind
	}{
		{name: "api root", path: "/api", wantKind: RouteKindAPI},
		{name: "api nested", path: "/api/v1/tasks", wantKind: RouteKindAPI},
		{name: "websocket", path: "/ws", wantKind: RouteKindWebSocket},
		{name: "websocket nested", path: "/ws/tasks", wantKind: RouteKindWebSocket},
		{name: "health", path: "/health", wantKind: RouteKindHealth},
		{name: "vite assets", path: "/assets/app.js", wantKind: RouteKindStatic},
		{name: "favicon", path: "/favicon.ico", wantKind: RouteKindStatic},
		{name: "robots", path: "/robots.txt", wantKind: RouteKindStatic},
		{name: "manifest", path: "/manifest.webmanifest", wantKind: RouteKindStatic},
		{name: "legacy next static", path: "/_next/static/chunk.js", wantKind: RouteKindStatic},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := ClassifyRoute(tc.path)
			if got.Kind != tc.wantKind {
				t.Fatalf("Kind = %q, want %q", got.Kind, tc.wantKind)
			}
			if got.Route != "" {
				t.Fatalf("Route = %q, want empty for non-SPA route", got.Route)
			}
		})
	}
}

func TestClassifyRouteUnknownPathFallsThroughToSPA(t *testing.T) {
	t.Parallel()

	got := ClassifyRoute("/future/client/route")
	if got.Kind != RouteKindSPA {
		t.Fatalf("Kind = %q, want %q", got.Kind, RouteKindSPA)
	}
	if got.Route != RouteUnknown {
		t.Fatalf("Route = %q, want %q", got.Route, RouteUnknown)
	}
}

func TestBootPayloadMarshalShapeAndScriptSafety(t *testing.T) {
	t.Parallel()

	payload := NewBootPayload(
		ClassifyRoute("/t/task-123"),
		RuntimeConfig{APIPrefix: "/api/v1", WebSocketPath: "/ws"},
		map[string]any{
			"task": map[string]any{
				"id":    "task-123",
				"title": "</script><script>alert(1)</script>",
			},
		},
	)

	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("Marshal BootPayload: %v", err)
	}
	if strings.Contains(string(data), "</script>") {
		t.Fatalf("payload contains raw script terminator: %s", data)
	}

	var decoded struct {
		Version      int             `json:"version"`
		Route        json.RawMessage `json:"route"`
		Runtime      json.RawMessage `json:"runtime"`
		InitialState json.RawMessage `json:"initialState"`
	}
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal BootPayload: %v", err)
	}
	if decoded.Version != BootPayloadVersion {
		t.Fatalf("Version = %d, want %d", decoded.Version, BootPayloadVersion)
	}
	assertRawJSONPresent(t, "route", decoded.Route)
	assertRawJSONPresent(t, "runtime", decoded.Runtime)
	assertRawJSONPresent(t, "initialState", decoded.InitialState)
}

func assertParams(t *testing.T, got, want map[string]string) {
	t.Helper()

	if len(got) != len(want) {
		t.Fatalf("Params = %#v, want %#v", got, want)
	}
	for key, wantValue := range want {
		if got[key] != wantValue {
			t.Fatalf("Params[%q] = %q, want %q", key, got[key], wantValue)
		}
	}
}

func assertRawJSONPresent(t *testing.T, name string, raw json.RawMessage) {
	t.Helper()

	if len(raw) == 0 || string(raw) == "null" {
		t.Fatalf("%s missing from payload", name)
	}
}
