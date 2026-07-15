package dto

import (
	"encoding/json"
	"testing"

	"github.com/kandev/kandev/internal/user/models"
)

func TestFromUserSettingsIncludesArchiveConfirmation(t *testing.T) {
	dto := FromUserSettings(&models.UserSettings{ConfirmTaskArchive: true})
	if !dto.ConfirmTaskArchive {
		t.Fatal("expected archive confirmation preference in DTO")
	}
}

func TestNullableSidebarDraft(t *testing.T) {
	t.Run("omitted field is not set", func(t *testing.T) {
		var req UpdateUserSettingsRequest
		if err := json.Unmarshal([]byte(`{}`), &req); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if req.SidebarDraft.Set {
			t.Fatal("expected omitted sidebar_draft to remain unset")
		}
		if req.SidebarDraft.ServiceValue() != nil {
			t.Fatal("expected omitted sidebar_draft to map to nil service value")
		}
	})

	t.Run("null field is set to nil draft", func(t *testing.T) {
		var req UpdateUserSettingsRequest
		if err := json.Unmarshal([]byte(`{"sidebar_draft":null}`), &req); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		serviceValue := req.SidebarDraft.ServiceValue()
		if !req.SidebarDraft.Set || serviceValue == nil || *serviceValue != nil {
			t.Fatalf("expected explicit null to map to set nil draft, got set=%v value=%v", req.SidebarDraft.Set, serviceValue)
		}
	})

	t.Run("object field is set to draft", func(t *testing.T) {
		var req UpdateUserSettingsRequest
		raw := []byte(`{"sidebar_draft":{"base_view_id":"view-1","filters":[],"sort":{"key":"state","direction":"asc"},"group":"state"}}`)
		if err := json.Unmarshal(raw, &req); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		serviceValue := req.SidebarDraft.ServiceValue()
		if !req.SidebarDraft.Set || serviceValue == nil || *serviceValue == nil || (*serviceValue).BaseViewID != "view-1" {
			t.Fatalf("expected object to map to draft, got set=%v value=%v", req.SidebarDraft.Set, serviceValue)
		}
	})
}

func TestNullableRawMessage(t *testing.T) {
	t.Run("omitted field is not set", func(t *testing.T) {
		var req UpdateUserSettingsRequest
		if err := json.Unmarshal([]byte(`{}`), &req); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if req.JiraSavedViews.Set {
			t.Fatal("expected omitted jira_saved_views to remain unset")
		}
		if req.JiraSavedViews.ServiceValue() != nil {
			t.Fatal("expected omitted jira_saved_views to map to nil service value")
		}
	})

	t.Run("null field is set to nil raw message", func(t *testing.T) {
		var req UpdateUserSettingsRequest
		if err := json.Unmarshal([]byte(`{"jira_saved_views":null}`), &req); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		serviceValue := req.JiraSavedViews.ServiceValue()
		if !req.JiraSavedViews.Set || serviceValue == nil || *serviceValue != nil {
			t.Fatalf("expected explicit null to map to set nil raw message, got set=%v value=%v", req.JiraSavedViews.Set, serviceValue)
		}
	})

	t.Run("json field is set to raw message", func(t *testing.T) {
		var req UpdateUserSettingsRequest
		if err := json.Unmarshal([]byte(`{"jira_saved_views":[{"id":"view-1"}]}`), &req); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		serviceValue := req.JiraSavedViews.ServiceValue()
		if !req.JiraSavedViews.Set || serviceValue == nil || *serviceValue == nil || string(**serviceValue) != `[{"id":"view-1"}]` {
			t.Fatalf("expected JSON value to map to raw message, got set=%v value=%v", req.JiraSavedViews.Set, serviceValue)
		}
	})
}
