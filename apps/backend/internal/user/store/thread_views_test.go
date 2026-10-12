package store

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/kandev/kandev/internal/user/models"
)

func TestScanUserSettingsThreadViewDefaultsAndRoundTrip(t *testing.T) {
	settings, err := scanUserSettings(settingsScanner{raw: "{}"}, DefaultUserID)
	if err != nil {
		t.Fatalf("scan defaults: %v", err)
	}
	if !reflect.DeepEqual(settings.ThreadViews, DefaultThreadViews()) {
		t.Fatalf("thread views = %+v, want %+v", settings.ThreadViews, DefaultThreadViews())
	}
	if settings.ThreadActiveViewID != DefaultThreadViewID {
		t.Fatalf("active thread view = %q, want %q", settings.ThreadActiveViewID, DefaultThreadViewID)
	}

	custom := `{"thread_views":[{"id":"custom","name":"Custom","task_scope":{"mode":"selected","task_ids":["task-a"]},"filters":[],"sort":{"key":"updatedAt","direction":"desc"},"max_columns":3}],"thread_active_view_id":"custom","thread_view_draft":{"base_view_id":"custom","task_scope":{"mode":"all","task_ids":[]},"filters":[],"sort":{"key":"attention","direction":"asc"},"max_columns":null}}`
	settings, err = scanUserSettings(settingsScanner{raw: custom}, DefaultUserID)
	if err != nil {
		t.Fatalf("scan custom settings: %v", err)
	}
	if len(settings.ThreadViews) != 1 || settings.ThreadViews[0].ID != "custom" {
		t.Fatalf("custom thread views = %+v, want custom view", settings.ThreadViews)
	}
	if settings.ThreadActiveViewID != "custom" || settings.ThreadViewDraft == nil {
		t.Fatalf("custom active/draft = %q/%+v, want custom values", settings.ThreadActiveViewID, settings.ThreadViewDraft)
	}
	encoded, err := marshalUserSettingsPayload(settings)
	if err != nil {
		t.Fatalf("marshal settings: %v", err)
	}
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &payload); err != nil {
		t.Fatalf("decode settings: %v", err)
	}
	if string(payload["thread_active_view_id"]) != `"custom"` {
		t.Fatalf("encoded active thread view = %s, want custom", payload["thread_active_view_id"])
	}
	if string(payload["thread_view_draft"]) == "null" {
		t.Fatalf("encoded thread draft = %s, want draft", payload["thread_view_draft"])
	}
}

func TestScanUserSettingsEmptyThreadViewListUsesCanonicalDefault(t *testing.T) {
	settings, err := scanUserSettings(settingsScanner{raw: `{"thread_views":[],"thread_active_view_id":""}`}, DefaultUserID)
	if err != nil {
		t.Fatalf("scan settings: %v", err)
	}
	if len(settings.ThreadViews) != 1 || settings.ThreadViews[0].ID != DefaultThreadViewID {
		t.Fatalf("thread views = %+v, want canonical default", settings.ThreadViews)
	}
	if settings.ThreadActiveViewID != DefaultThreadViewID {
		t.Fatalf("active thread view = %q, want canonical default", settings.ThreadActiveViewID)
	}
}

func TestDefaultThreadViewUsesFiveColumns(t *testing.T) {
	view := DefaultThreadViews()[0]
	if view.MaxColumns == nil || *view.MaxColumns != 5 {
		t.Fatalf("default max columns = %v, want 5", view.MaxColumns)
	}
	if view.TaskScope.Mode != models.ThreadTaskScopeAll || len(view.TaskScope.TaskIDs) != 0 {
		t.Fatalf("default task scope = %+v, want all", view.TaskScope)
	}
}

// @covers AC-UI-THREADS-SAVED-VIEWS-005.1, AC-UI-THREADS-SAVED-VIEWS-005.3
func TestStoredThreadPresentationNormalizesIndependently(t *testing.T) {
	cases := []struct {
		name, fields, layout string
		autoHide             bool
	}{
		{"legacy", ``, "columns", false},
		{"grid", `,"layout":"grid","auto_hide_composer":true`, "grid", true},
		{"unknown layout", `,"layout":"masonry","auto_hide_composer":true`, "columns", true},
		{"malformed layout", `,"layout":{},"auto_hide_composer":true`, "columns", true},
		{"malformed toggle", `,"layout":"grid","auto_hide_composer":"true"`, "grid", false},
		{"null fields", `,"layout":null,"auto_hide_composer":null`, "columns", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body := `"task_scope":{"mode":"selected","task_ids":["task-sentinel"]},"filters":[],"sort":{"key":"priority","direction":"desc"},"max_columns":7` + tc.fields
			raw := `{"thread_views":[{"id":"custom","name":"Custom",` + body + `}],"thread_active_view_id":"custom","thread_view_draft":{"base_view_id":"custom",` + body + `}}`
			settings, err := scanUserSettings(settingsScanner{raw: raw}, DefaultUserID)
			if err != nil {
				t.Fatalf("scan: %v", err)
			}
			encoded, err := marshalUserSettingsPayload(settings)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			var payload struct {
				Views []map[string]any `json:"thread_views"`
				Draft map[string]any   `json:"thread_view_draft"`
			}
			if err := json.Unmarshal(encoded, &payload); err != nil {
				t.Fatal(err)
			}
			for _, value := range []map[string]any{payload.Views[0], payload.Draft} {
				if value["layout"] != tc.layout || value["auto_hide_composer"] != tc.autoHide {
					t.Fatalf("presentation = %v/%v, want %s/%v", value["layout"], value["auto_hide_composer"], tc.layout, tc.autoHide)
				}
				if value["max_columns"] != float64(7) || value["task_scope"].(map[string]any)["task_ids"].([]any)[0] != "task-sentinel" {
					t.Fatalf("query changed: %+v", value)
				}
			}
		})
	}
}
