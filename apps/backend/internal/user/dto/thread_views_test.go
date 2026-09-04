package dto

import (
	"encoding/json"
	"testing"

	"github.com/kandev/kandev/internal/user/models"
)

func TestThreadViewDTOMapsPersistedFields(t *testing.T) {
	maxColumns := 3
	settings := &models.UserSettings{
		ThreadViews: []models.ThreadView{{
			ID:         "view-one",
			Name:       "One",
			TaskScope:  models.ThreadTaskScope{Mode: models.ThreadTaskScopeSelected, TaskIDs: []string{"task-a"}},
			Filters:    []models.ThreadViewClause{},
			Sort:       models.ThreadViewSort{Key: "priority", Direction: "desc"},
			MaxColumns: &maxColumns,
		}},
		ThreadActiveViewID: "view-one",
		ThreadViewDraft:    &models.ThreadViewDraft{BaseViewID: "view-one"},
	}
	got := FromUserSettings(settings)
	if len(got.ThreadViews) != 1 || got.ThreadViews[0].ID != "view-one" {
		t.Fatalf("thread views = %+v, want view-one", got.ThreadViews)
	}
	if got.ThreadActiveViewID != "view-one" || got.ThreadViewDraft == nil {
		t.Fatalf("active/draft = %q/%+v, want persisted values", got.ThreadActiveViewID, got.ThreadViewDraft)
	}
}

func TestNullableThreadViewDraftDistinguishesOmittedAndNull(t *testing.T) {
	var omitted UpdateUserSettingsRequest
	if omitted.ThreadViewDraft.Set || omitted.ThreadViewDraft.ServiceValue() != nil {
		t.Fatal("zero thread draft should mean omitted")
	}
	var explicit UpdateUserSettingsRequest
	if err := json.Unmarshal([]byte(`{"thread_view_draft":null}`), &explicit); err != nil {
		t.Fatalf("decode explicit null: %v", err)
	}
	if !explicit.ThreadViewDraft.Set {
		t.Fatal("explicit null thread draft should be marked set")
	}
	value := explicit.ThreadViewDraft.ServiceValue()
	if value == nil || *value != nil {
		t.Fatalf("service draft = %v, want pointer to nil", value)
	}
}
