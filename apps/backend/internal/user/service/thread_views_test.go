package service

import (
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/user/models"
)

func TestApplyThreadViewsAcceptsValidState(t *testing.T) {
	maxColumns := 3
	views := []models.ThreadView{{
		ID:         "view-one",
		Name:       "One",
		TaskScope:  models.ThreadTaskScope{Mode: models.ThreadTaskScopeSelected, TaskIDs: []string{"task-a"}},
		Filters:    []models.ThreadViewClause{},
		Sort:       models.ThreadViewSort{Key: "attention", Direction: "asc"},
		MaxColumns: &maxColumns,
	}}
	active := "view-one"
	settings := &models.UserSettings{ThreadViews: []models.ThreadView{}, ThreadActiveViewID: "view-all-threads"}
	req := &UpdateUserSettingsRequest{ThreadViews: &views, ThreadActiveViewID: &active}
	if err := applyThreadViews(settings, req); err != nil {
		t.Fatalf("applyThreadViews: %v", err)
	}
	if err := applyThreadViewState(settings, req); err != nil {
		t.Fatalf("applyThreadViews: %v", err)
	}
	if settings.ThreadActiveViewID != active || len(settings.ThreadViews) != 1 {
		t.Fatalf("settings = %+v, want one active view", settings)
	}
}

func TestApplyThreadViewsRejectsInvalidValues(t *testing.T) {
	tooManyViews := make([]models.ThreadView, maxThreadViews+1)
	for i := range tooManyViews {
		tooManyViews[i] = validThreadView("view-" + string(rune('a'+i)))
	}
	tooManyClauses := validThreadView("view-clauses")
	tooManyClauses.Filters = make([]models.ThreadViewClause, maxThreadViewClauses+1)
	tooManyTaskIDs := validThreadView("view-task-ids")
	tooManyTaskIDs.TaskScope = models.ThreadTaskScope{Mode: models.ThreadTaskScopeSelected, TaskIDs: make([]string, maxThreadViewTaskIDs+1)}
	tooManyColumns := validThreadView("view-columns")
	maxColumns := maxThreadViewColumns + 1
	tooManyColumns.MaxColumns = &maxColumns
	emptySelectedScope := validThreadView("view-empty-selected")
	emptySelectedScope.TaskScope = models.ThreadTaskScope{Mode: models.ThreadTaskScopeSelected, TaskIDs: []string{}}
	duplicate := validThreadView("view-duplicate")
	duplicateViews := []models.ThreadView{duplicate, duplicate}
	invalidActive := "missing"

	tests := []struct {
		name  string
		views []models.ThreadView
		req   UpdateUserSettingsRequest
		want  string
	}{
		{name: "view count", views: tooManyViews, want: "max 50 views"},
		{name: "filter count", views: []models.ThreadView{tooManyClauses}, req: UpdateUserSettingsRequest{ThreadActiveViewID: stringPtr("view-clauses")}, want: "max 20 filter clauses"},
		{name: "task id count", views: []models.ThreadView{tooManyTaskIDs}, req: UpdateUserSettingsRequest{ThreadActiveViewID: stringPtr("view-task-ids")}, want: "max 200 selected task ids"},
		{name: "column count", views: []models.ThreadView{tooManyColumns}, req: UpdateUserSettingsRequest{ThreadActiveViewID: stringPtr("view-columns")}, want: "between 1 and 30"},
		{name: "empty selected scope", views: []models.ThreadView{emptySelectedScope}, req: UpdateUserSettingsRequest{ThreadActiveViewID: stringPtr("view-empty-selected")}, want: "selected task scope must contain at least one task id"},
		{name: "duplicate id", views: duplicateViews, req: UpdateUserSettingsRequest{ThreadActiveViewID: stringPtr("view-duplicate")}, want: "duplicate view id"},
		{name: "active reference", views: []models.ThreadView{validThreadView("view-one")}, req: UpdateUserSettingsRequest{ThreadActiveViewID: &invalidActive}, want: "does not match any saved view"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.req.ThreadViews = &tt.views
			settings := &models.UserSettings{ThreadViews: []models.ThreadView{validThreadView("old")}, ThreadActiveViewID: "old"}
			err := applyThreadViews(settings, &tt.req)
			if err == nil {
				err = applyThreadViewState(settings, &tt.req)
			}
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want text containing %q", err, tt.want)
			}
		})
	}
}

func TestApplyThreadViewStateAllowsEmptySelectedDraft(t *testing.T) {
	settings := &models.UserSettings{ThreadViews: []models.ThreadView{validThreadView("view-one")}}
	draft := &models.ThreadViewDraft{
		BaseViewID: "view-one",
		TaskScope:  models.ThreadTaskScope{Mode: models.ThreadTaskScopeSelected, TaskIDs: []string{}},
	}
	if err := applyThreadViewState(settings, &UpdateUserSettingsRequest{ThreadViewDraft: &draft}); err != nil {
		t.Fatalf("applyThreadViewState: %v", err)
	}
}

func TestApplyThreadViewStateRejectsUnknownDraftBase(t *testing.T) {
	settings := &models.UserSettings{ThreadViews: []models.ThreadView{validThreadView("view-one")}}
	draft := &models.ThreadViewDraft{
		BaseViewID: "missing",
		TaskScope:  models.ThreadTaskScope{Mode: models.ThreadTaskScopeAll, TaskIDs: []string{}},
	}
	if err := applyThreadViewState(settings, &UpdateUserSettingsRequest{ThreadViewDraft: &draft}); err == nil || !strings.Contains(err.Error(), "base_view_id") {
		t.Fatalf("error = %v, want unknown base_view_id error", err)
	}
}

func TestApplyThreadViewsEmptyListUsesCanonicalDefault(t *testing.T) {
	views := []models.ThreadView{}
	settings := &models.UserSettings{}
	if err := applyThreadViews(settings, &UpdateUserSettingsRequest{ThreadViews: &views}); err != nil {
		t.Fatalf("applyThreadViews: %v", err)
	}
	if len(settings.ThreadViews) != 1 || settings.ThreadViews[0].ID != "view-all-threads" {
		t.Fatalf("views = %+v, want canonical default", settings.ThreadViews)
	}
	if settings.ThreadActiveViewID != "view-all-threads" {
		t.Fatalf("active view = %q, want canonical default", settings.ThreadActiveViewID)
	}
}

func TestApplyThreadViewStateClearsExplicitDraft(t *testing.T) {
	settings := &models.UserSettings{
		ThreadViews:        []models.ThreadView{validThreadView("view-one")},
		ThreadActiveViewID: "view-one",
		ThreadViewDraft:    &models.ThreadViewDraft{BaseViewID: "view-one"},
	}
	var clearDraft *models.ThreadViewDraft
	if err := applyThreadViewState(settings, &UpdateUserSettingsRequest{ThreadViewDraft: &clearDraft}); err != nil {
		t.Fatalf("applyThreadViewState: %v", err)
	}
	if settings.ThreadViewDraft != nil {
		t.Fatalf("draft = %+v, want nil", settings.ThreadViewDraft)
	}
}

func validThreadView(id string) models.ThreadView {
	return models.ThreadView{
		ID:        id,
		Name:      id,
		TaskScope: models.ThreadTaskScope{Mode: models.ThreadTaskScopeAll, TaskIDs: []string{}},
		Filters:   []models.ThreadViewClause{},
		Sort:      models.ThreadViewSort{Key: "attention", Direction: "asc"},
	}
}

func stringPtr(value string) *string { return &value }
