package clarification

import (
	"testing"
	"time"
)

func fixedNow() time.Time {
	return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
}

func twoQuestionBundle() []Question {
	return []Question{
		{ID: "q1", Options: []Option{{ID: "opt-a"}, {ID: "opt-b"}}},
		{ID: "q2", Options: []Option{{ID: "opt-c"}}},
	}
}

// TestValidateOutcome_N6Condition1_WrongCount proves N6 condition 1: the
// answers array must contain exactly one entry per question.
func TestValidateOutcome_N6Condition1_WrongCount(t *testing.T) {
	err := validateOutcome(twoQuestionBundle(), Outcome{
		Answers: []Answer{{QuestionID: "q1"}},
	})
	if err == nil || !IsValidationError(err) {
		t.Fatalf("expected a validation error, got %v", err)
	}
}

// TestValidateOutcome_N6Condition2_EmptyQuestionID proves N6 condition 2:
// any entry with an empty question_id is rejected, even against a bundle
// whose expected set legitimately contains "" (L16).
func TestValidateOutcome_N6Condition2_EmptyQuestionID(t *testing.T) {
	err := validateOutcome(twoQuestionBundle(), Outcome{
		Answers: []Answer{{QuestionID: ""}, {QuestionID: "q2"}},
	})
	if err == nil || !IsValidationError(err) {
		t.Fatalf("expected a validation error, got %v", err)
	}
}

// TestValidateOutcome_N6Condition3_UnknownQuestionID proves N6 condition 3.
func TestValidateOutcome_N6Condition3_UnknownQuestionID(t *testing.T) {
	err := validateOutcome(twoQuestionBundle(), Outcome{
		Answers: []Answer{{QuestionID: "q1"}, {QuestionID: "not-in-bundle"}},
	})
	if err == nil || !IsValidationError(err) {
		t.Fatalf("expected a validation error, got %v", err)
	}
}

// TestValidateOutcome_N6Condition4_DuplicateQuestionID proves N6 condition 4.
func TestValidateOutcome_N6Condition4_DuplicateQuestionID(t *testing.T) {
	err := validateOutcome(twoQuestionBundle(), Outcome{
		Answers: []Answer{{QuestionID: "q1"}, {QuestionID: "q1"}},
	})
	if err == nil || !IsValidationError(err) {
		t.Fatalf("expected a validation error, got %v", err)
	}
}

// TestValidateOutcome_N7_EmptyEntryAccepted proves N7: an entry with neither
// selected_options nor custom_text is a legal "(no answer)" answer.
func TestValidateOutcome_N7_EmptyEntryAccepted(t *testing.T) {
	err := validateOutcome(twoQuestionBundle(), Outcome{
		Answers: []Answer{{QuestionID: "q1"}, {QuestionID: "q2"}},
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

// TestValidateOutcome_N8_UnknownOptionID proves N8: a selected_options entry
// naming an option_id absent from that question is rejected.
func TestValidateOutcome_N8_UnknownOptionID(t *testing.T) {
	err := validateOutcome(twoQuestionBundle(), Outcome{
		Answers: []Answer{
			{QuestionID: "q1", SelectedOptions: []string{"not-an-option"}},
			{QuestionID: "q2"},
		},
	})
	if err == nil || !IsValidationError(err) {
		t.Fatalf("expected a validation error, got %v", err)
	}
}

// TestValidateOutcome_N8_MultipleValidOptionsAccepted proves N8 constrains
// membership only, not cardinality: naming several valid option IDs on one
// question is accepted since nothing marks a question single- or
// multi-select.
func TestValidateOutcome_N8_MultipleValidOptionsAccepted(t *testing.T) {
	err := validateOutcome(twoQuestionBundle(), Outcome{
		Answers: []Answer{
			{QuestionID: "q1", SelectedOptions: []string{"opt-a", "opt-b"}},
			{QuestionID: "q2"},
		},
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

// TestValidateOutcome_N8a_RejectedWithAnswers proves N8a: rejected:true
// combined with a non-empty answers array is rejected.
func TestValidateOutcome_N8a_RejectedWithAnswers(t *testing.T) {
	err := validateOutcome(twoQuestionBundle(), Outcome{
		Rejected: true,
		Answers:  []Answer{{QuestionID: "q1"}, {QuestionID: "q2"}},
	})
	if err == nil || !IsValidationError(err) {
		t.Fatalf("expected a validation error, got %v", err)
	}
}

// TestValidateOutcome_N8a_RejectedTrueEmptyAnswersAccepted proves the
// legitimate rejection shape is accepted regardless of bundle size.
func TestValidateOutcome_N8a_RejectedTrueEmptyAnswersAccepted(t *testing.T) {
	err := validateOutcome(twoQuestionBundle(), Outcome{Rejected: true, RejectReason: "not needed"})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

// TestValidateOutcome_N8b_CustomTextOverCap proves N8b's rune cap on an
// answer's custom_text.
func TestValidateOutcome_N8b_CustomTextOverCap(t *testing.T) {
	long := make([]rune, answerTextRuneCap+1)
	for i := range long {
		long[i] = 'a'
	}
	err := validateOutcome(twoQuestionBundle(), Outcome{
		Answers: []Answer{{QuestionID: "q1", CustomText: string(long)}, {QuestionID: "q2"}},
	})
	if err == nil || !IsValidationError(err) {
		t.Fatalf("expected a validation error, got %v", err)
	}
}

// TestValidateOutcome_N8b_ReasonOverCap proves N8b's rune cap on a
// rejection's reason.
func TestValidateOutcome_N8b_ReasonOverCap(t *testing.T) {
	long := make([]rune, answerTextRuneCap+1)
	for i := range long {
		long[i] = '本' // multi-byte rune: proves the cap counts runes, not bytes
	}
	err := validateOutcome(twoQuestionBundle(), Outcome{Rejected: true, RejectReason: string(long)})
	if err == nil || !IsValidationError(err) {
		t.Fatalf("expected a validation error, got %v", err)
	}
}

// TestValidateOutcome_N6a_L16BundleUnanswerableExceptByRejection proves L16:
// a bundle whose sole question carries an empty question_id can never pass
// answers validation, whatever the caller submits, and can only be
// rejected.
func TestValidateOutcome_N6a_L16BundleUnanswerableExceptByRejection(t *testing.T) {
	l16 := []Question{{ID: ""}}

	if err := validateOutcome(l16, Outcome{Answers: []Answer{{QuestionID: ""}}}); err == nil {
		t.Fatalf("expected empty question_id to fail N6 condition 2")
	}
	if err := validateOutcome(l16, Outcome{Answers: []Answer{{QuestionID: "anything"}}}); err == nil {
		t.Fatalf("expected unknown question_id to fail N6 condition 3")
	}
	if err := validateOutcome(l16, Outcome{Rejected: true}); err != nil {
		t.Fatalf("expected rejection to be accepted, got %v", err)
	}
}

// TestNormalizeAnswered_OrdersByBundleQuestionOrder proves N3a rule 1: the
// normalized response orders answers by the bundle's own question order,
// not the order the caller supplied them.
func TestNormalizeAnswered_OrdersByBundleQuestionOrder(t *testing.T) {
	questions := twoQuestionBundle()
	resp := normalizeAnswered("pending-1", questions, []Answer{
		{QuestionID: "q2", CustomText: "second"},
		{QuestionID: "q1", CustomText: "first"},
	}, fixedNow())
	if len(resp.Answers) != 2 {
		t.Fatalf("expected 2 answers, got %d", len(resp.Answers))
	}
	if resp.Answers[0].QuestionID != "q1" || resp.Answers[1].QuestionID != "q2" {
		t.Fatalf("got order %q, %q; want q1, q2", resp.Answers[0].QuestionID, resp.Answers[1].QuestionID)
	}
}

// TestNormalizeAnswered_OrdersAndDedupsSelectedOptions proves N3a rule 2:
// selected_options is ordered by the option's position in the question's
// options array, and exact duplicates are removed.
func TestNormalizeAnswered_OrdersAndDedupsSelectedOptions(t *testing.T) {
	questions := []Question{
		{ID: "q1", Options: []Option{{ID: "opt-a"}, {ID: "opt-b"}, {ID: "opt-c"}}},
	}
	resp := normalizeAnswered("pending-1", questions, []Answer{
		{QuestionID: "q1", SelectedOptions: []string{"opt-c", "opt-a", "opt-c", "opt-b", "opt-a"}},
	}, fixedNow())
	got := resp.Answers[0].SelectedOptions
	want := []string{"opt-a", "opt-b", "opt-c"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

// TestNormalizeAnswered_TrimsWhitespace proves N3a rule 3: custom_text is
// stored verbatim after trimming leading/trailing whitespace, with no other
// transformation.
func TestNormalizeAnswered_TrimsWhitespace(t *testing.T) {
	questions := []Question{{ID: "q1"}}
	resp := normalizeAnswered("pending-1", questions, []Answer{
		{QuestionID: "q1", CustomText: "  padded  inside stays  "},
	}, fixedNow())
	if got, want := resp.Answers[0].CustomText, "padded  inside stays"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

// TestNormalizeAnswered_ByteIdenticalForEquivalentSubmissions proves N3a's
// core promise: two semantically identical submissions in different caller
// order, with a stray reject_reason on one of them (M6, discarded on the
// answered path), produce byte-identical answers/rejected/reject_reason
// payloads once serialized.
func TestNormalizeAnswered_ByteIdenticalForEquivalentSubmissions(t *testing.T) {
	questions := twoQuestionBundle()
	_, a := buildOutcomeResponse("pending-1", questions, Outcome{
		Answers: []Answer{
			{QuestionID: "q1", SelectedOptions: []string{"opt-b", "opt-a"}},
			{QuestionID: "q2"},
		},
	}, fixedNow())
	_, b := buildOutcomeResponse("pending-1", questions, Outcome{
		Answers: []Answer{
			{QuestionID: "q2"},
			{QuestionID: "q1", SelectedOptions: []string{"opt-a", "opt-b"}},
		},
		RejectReason: "stray, should be discarded per M6",
	}, fixedNow())

	sa, err := SerializeResponse(a)
	if err != nil {
		t.Fatalf("SerializeResponse(a): %v", err)
	}
	sb, err := SerializeResponse(b)
	if err != nil {
		t.Fatalf("SerializeResponse(b): %v", err)
	}
	if sa != sb {
		t.Fatalf("expected byte-identical answer payloads:\na=%s\nb=%s", sa, sb)
	}
}
