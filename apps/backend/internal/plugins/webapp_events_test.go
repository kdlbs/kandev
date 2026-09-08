package plugins

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
)

func TestProjectWebAppEventDropsInternalPayloadFields(t *testing.T) {
	event := bus.NewEvent(events.CanvasReleaseActivated, "canvas", map[string]any{
		"canvas_id":         "canvas-1",
		"workspace_id":      "workspace-1",
		"release_id":        "release-1",
		"active_release_id": "release-1",
		"secret":            strings.Repeat("s", 128),
		"manifest_json":     "must-not-leak",
	})

	projected, ok := projectWebAppEvent(event)
	if !ok {
		t.Fatal("projectWebAppEvent() rejected a public lifecycle event")
	}
	raw, err := json.Marshal(projected.Data)
	if err != nil {
		t.Fatalf("marshal projected data: %v", err)
	}
	encoded := string(raw)
	for _, forbidden := range []string{"secret", "manifest_json", "must-not-leak"} {
		if strings.Contains(encoded, forbidden) {
			t.Fatalf("projected event contains forbidden field %q: %s", forbidden, encoded)
		}
	}
	if projected.Scope.WorkspaceID != "workspace-1" || projected.Scope.InstanceID != "" {
		t.Fatalf("projected scope = %+v", projected.Scope)
	}
}
