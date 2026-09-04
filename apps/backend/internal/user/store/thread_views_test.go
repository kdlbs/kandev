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
