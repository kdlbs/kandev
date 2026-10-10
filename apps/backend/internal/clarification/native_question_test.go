package clarification

import (
	"testing"
	"time"
)

func TestNormalizeNativeQuestionsAllowsExplicitTextOnlyAnswers(t *testing.T) {
	allowText := true
	questions := []Question{{ID: "q1", Prompt: "What should change?", AllowCustomText: &allowText}}
	if err := NormalizeAndValidateQuestionsAllowFreeTextOnly(questions); err != "" {
		t.Fatalf("native text-only validation = %q", err)
	}
	if got := NormalizeAndValidateQuestions(questions); got == "" {
		t.Fatal("ordinary clarification validation accepted a text-only question")
	}
}

func TestNormalizeNativeQuestionsRejectsTextOnlyWithoutExplicitPermission(t *testing.T) {
	for name, question := range map[string]Question{
		"unset":  {ID: "q1", Prompt: "What should change?"},
		"denied": {ID: "q1", Prompt: "What should change?", AllowCustomText: boolPointer(false)},
	} {
		t.Run(name, func(t *testing.T) {
			if got := NormalizeAndValidateQuestionsAllowFreeTextOnly([]Question{question}); got == "" {
				t.Fatal("text-only question without explicit custom text permission was accepted")
			}
		})
	}
}

func TestStoreDoesNotDeduplicateDifferentCustomTextPolicies(t *testing.T) {
	store := NewStore(time.Minute)
	allowText := true
	denyText := false
	base := Question{
		ID: "q1", Prompt: "Choose a mode",
		Options: []Option{{ID: "fast", Label: "Fast"}, {ID: "safe", Label: "Safe"}},
	}
	first := base
	first.AllowCustomText = &denyText
	firstID, firstCreated := store.CreateRequest(&Request{SessionID: "session-1", Questions: []Question{first}})
	if !firstCreated {
		t.Fatal("first request was not created")
	}
	second := base
	second.AllowCustomText = &allowText
	secondID, secondCreated := store.CreateRequest(&Request{SessionID: "session-1", Questions: []Question{second}})
	if !secondCreated || firstID == secondID {
		t.Fatalf("different custom text policies deduplicated: first=%q second=%q created=%v", firstID, secondID, secondCreated)
	}
}

func boolPointer(value bool) *bool { return &value }
