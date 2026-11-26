package clarification

import "testing"

func TestValidateOutcomeExportMatchesInternal(t *testing.T) {
	qs := twoQuestionBundle()
	bad := Outcome{Answers: []Answer{{QuestionID: "q1"}}}
	if err := ValidateOutcome(qs, bad); err == nil || !IsValidationError(err) {
		t.Fatalf("want validation error, got %v", err)
	}
	good := Outcome{Answers: []Answer{
		{QuestionID: "q1", SelectedOptions: []string{"opt-a"}},
		{QuestionID: "q2", SelectedOptions: []string{"opt-c"}},
	}}
	if got, want := ValidateOutcome(qs, good), validateOutcome(qs, good); (got == nil) != (want == nil) {
		t.Fatalf("export %v, internal %v", got, want)
	}
}

func TestBundleQuestionsExportMatchesInternal(t *testing.T) {
	if got := BundleQuestions(nil); len(got) != len(bundleQuestions(nil)) {
		t.Fatalf("export disagrees with bundleQuestions: %v", got)
	}
}
