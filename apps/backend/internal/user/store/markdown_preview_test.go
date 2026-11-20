package store

import (
	"encoding/json"
	"testing"
)

func TestMarkdownPreviewDefault(t *testing.T) {
	for _, tc := range []struct {
		raw  string
		want bool
	}{{`{}`, true}, {`{"markdown_preview_by_default":false}`, false}, {`{"markdown_preview_by_default":true}`, true}} {
		settings, err := scanUserSettings(settingsScanner{raw: tc.raw}, DefaultUserID)
		if err != nil {
			t.Fatal(err)
		}
		if settings.MarkdownPreviewByDefault != tc.want {
			t.Fatalf("%s: got %v", tc.raw, settings.MarkdownPreviewByDefault)
		}
		raw, err := marshalUserSettingsPayload(settings)
		if err != nil {
			t.Fatal(err)
		}
		var payload map[string]any
		if err := json.Unmarshal(raw, &payload); err != nil {
			t.Fatal(err)
		}
		if payload["markdown_preview_by_default"] != tc.want {
			t.Fatalf("round trip: %s", raw)
		}
	}
}
