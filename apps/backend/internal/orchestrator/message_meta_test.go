package orchestrator

import (
	"testing"

	v1 "github.com/kandev/kandev/pkg/api/v1"
)

func TestUserMessageMeta_ToMap_Empty(t *testing.T) {
	meta := NewUserMessageMeta()
	result := meta.ToMap()
	if result != nil {
		t.Errorf("expected nil for empty meta, got %v", result)
	}
}

func TestUserMessageMeta_ToMap_PlanModeOnly(t *testing.T) {
	meta := NewUserMessageMeta().WithPlanMode(true)
	result := meta.ToMap()
	if result == nil {
		t.Fatal("expected non-nil map")
	}
	if v, ok := result["plan_mode"]; !ok || v != true {
		t.Errorf("expected plan_mode=true, got %v", result)
	}
	if _, ok := result["has_review_comments"]; ok {
		t.Error("unexpected has_review_comments key")
	}
	if _, ok := result["attachments"]; ok {
		t.Error("unexpected attachments key")
	}
}

func TestUserMessageMeta_ToMap_ReviewCommentsOnly(t *testing.T) {
	meta := NewUserMessageMeta().WithReviewComments(true)
	result := meta.ToMap()
	if result == nil {
		t.Fatal("expected non-nil map")
	}
	if v, ok := result["has_review_comments"]; !ok || v != true {
		t.Errorf("expected has_review_comments=true, got %v", result)
	}
	if _, ok := result["plan_mode"]; ok {
		t.Error("unexpected plan_mode key")
	}
}

func TestUserMessageMeta_ToMap_AttachmentsOnly(t *testing.T) {
	attachments := []v1.MessageAttachment{{Type: "image", Data: "base64data", MimeType: "image/png"}}
	meta := NewUserMessageMeta().WithAttachments(attachments)
	result := meta.ToMap()
	if result == nil {
		t.Fatal("expected non-nil map")
	}
	att, ok := result["attachments"]
	if !ok {
		t.Fatal("expected attachments key")
	}
	if len(att.([]v1.MessageAttachment)) != 1 {
		t.Errorf("expected 1 attachment, got %d", len(att.([]v1.MessageAttachment)))
	}
}

func TestUserMessageMeta_ToMap_ContextFilesOnly(t *testing.T) {
	files := []v1.ContextFileMeta{{Path: "src/main.go", Name: "main.go"}}
	meta := NewUserMessageMeta().WithContextFiles(files)
	result := meta.ToMap()
	if result == nil {
		t.Fatal("expected non-nil map")
	}
	cf, ok := result["context_files"]
	if !ok {
		t.Fatal("expected context_files key")
	}
	if len(cf.([]v1.ContextFileMeta)) != 1 {
		t.Errorf("expected 1 context file, got %d", len(cf.([]v1.ContextFileMeta)))
	}
	if _, ok := result["plan_mode"]; ok {
		t.Error("unexpected plan_mode key")
	}
}

func TestUserMessageMeta_ToMap_AllFields(t *testing.T) {
	attachments := []v1.MessageAttachment{{Type: "image", Data: "data", MimeType: "image/jpeg"}}
	contextFiles := []v1.ContextFileMeta{{Path: "README.md", Name: "README.md"}}
	meta := NewUserMessageMeta().
		WithPlanMode(true).
		WithReviewComments(true).
		WithAttachments(attachments).
		WithContextFiles(contextFiles)
	result := meta.ToMap()
	if result == nil {
		t.Fatal("expected non-nil map")
	}
	if len(result) != 4 {
		t.Errorf("expected 4 keys, got %d", len(result))
	}
	if result["plan_mode"] != true {
		t.Error("expected plan_mode=true")
	}
	if result["has_review_comments"] != true {
		t.Error("expected has_review_comments=true")
	}
	if _, ok := result["attachments"]; !ok {
		t.Error("expected attachments key")
	}
	if _, ok := result["context_files"]; !ok {
		t.Error("expected context_files key")
	}
}

func TestUserMessageMeta_ToMap_FalseValues(t *testing.T) {
	meta := NewUserMessageMeta().
		WithPlanMode(false).
		WithReviewComments(false)
	result := meta.ToMap()
	if result != nil {
		t.Errorf("expected nil for all-false meta, got %v", result)
	}
}

func TestUserMessageMeta_Chaining(t *testing.T) {
	meta := NewUserMessageMeta()
	returned := meta.WithPlanMode(true)
	if returned != meta {
		t.Error("WithPlanMode should return the same pointer for chaining")
	}
	returned = meta.WithReviewComments(true)
	if returned != meta {
		t.Error("WithReviewComments should return the same pointer for chaining")
	}
	returned = meta.WithAttachments(nil)
	if returned != meta {
		t.Error("WithAttachments should return the same pointer for chaining")
	}
	returned = meta.WithContextFiles(nil)
	if returned != meta {
		t.Error("WithContextFiles should return the same pointer for chaining")
	}
}
