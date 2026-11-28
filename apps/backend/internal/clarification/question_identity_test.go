package clarification

import (
	"testing"

	taskmodels "github.com/kandev/kandev/internal/task/models"
)

func TestQuestionIdentitiesKeepIDAndPrompt(t *testing.T) {
	got := QuestionIdentities([]Question{
		{ID: "db-choice", Prompt: "Which database?"},
		{ID: "q2", Prompt: "Which region?"},
	})
	if len(got) != 2 || got[0].ID != "db-choice" || got[0].Prompt != "Which database?" || got[1].ID != "q2" {
		t.Fatalf("identities = %+v", got)
	}
}

func TestSameClarificationQuestionRules(t *testing.T) {
	cases := []struct {
		name string
		a, b taskmodels.ClarificationQuestionIdentity
		want bool
	}{
		{"normalized text", taskmodels.ClarificationQuestionIdentity{Prompt: " Which   DB? "}, taskmodels.ClarificationQuestionIdentity{Prompt: "which db?"}, true},
		{"agent id", taskmodels.ClarificationQuestionIdentity{ID: "db", Prompt: "A"}, taskmodels.ClarificationQuestionIdentity{ID: "db", Prompt: "B"}, true},
		{"positional id alone", taskmodels.ClarificationQuestionIdentity{ID: "q1", Prompt: "A"}, taskmodels.ClarificationQuestionIdentity{ID: "q1", Prompt: "B"}, false},
		{"different text", taskmodels.ClarificationQuestionIdentity{Prompt: "Which DB?"}, taskmodels.ClarificationQuestionIdentity{Prompt: "Which region?"}, false},
		{"empty text", taskmodels.ClarificationQuestionIdentity{Prompt: "  "}, taskmodels.ClarificationQuestionIdentity{Prompt: ""}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := taskmodels.SameClarificationQuestion(tc.a, tc.b); got != tc.want {
				t.Fatalf("SameClarificationQuestion = %v, want %v", got, tc.want)
			}
		})
	}
}
