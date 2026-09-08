package clarification

import (
	"encoding/json"
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

// TestValidateOutcome_N8b_CustomTextAtCapAccepted proves N8b's positive
// boundary: exactly answerTextRuneCap runes, counted over code points with a
// multi-byte fixture (not bytes), is accepted.
func TestValidateOutcome_N8b_CustomTextAtCapAccepted(t *testing.T) {
	exact := make([]rune, answerTextRuneCap)
	for i := range exact {
		exact[i] = '本' // multi-byte rune: proves the cap counts runes, not bytes
	}
	err := validateOutcome(twoQuestionBundle(), Outcome{
		Answers: []Answer{{QuestionID: "q1", CustomText: string(exact)}, {QuestionID: "q2"}},
	})
	if err != nil {
		t.Fatalf("expected no error at exactly %d runes, got %v", answerTextRuneCap, err)
	}
}

// TestValidateOutcome_N8b_ReasonAtCapAccepted proves N8b's positive boundary
// on a rejection's reason: exactly answerTextRuneCap runes, counted over
// code points with a multi-byte fixture, is accepted.
func TestValidateOutcome_N8b_ReasonAtCapAccepted(t *testing.T) {
	exact := make([]rune, answerTextRuneCap)
	for i := range exact {
		exact[i] = '本' // multi-byte rune: proves the cap counts runes, not bytes
	}
	err := validateOutcome(twoQuestionBundle(), Outcome{Rejected: true, RejectReason: string(exact)})
	if err != nil {
		t.Fatalf("expected no error at exactly %d runes, got %v", answerTextRuneCap, err)
	}
}

// TestValidateOutcome_N3aRule3_CustomTextOverCapWithTrailingWhitespaceRejected
// proves N3a rule 3's ordering requirement: validation runs on the caller's
// RAW, untrimmed input, before normalizeAnswer's trimming. A value that is
// over the cap only counting trailing whitespace must still be rejected, even
// though trimming it would bring it under the cap — trimming happens strictly
// after validation, never before.
func TestValidateOutcome_N3aRule3_CustomTextOverCapWithTrailingWhitespaceRejected(t *testing.T) {
	long := make([]rune, answerTextRuneCap-1)
	for i := range long {
		long[i] = 'a'
	}
	overCapWithTrailingSpace := string(long) + "  " // answerTextRuneCap+1 runes, only trims to answerTextRuneCap-1

	err := validateOutcome(twoQuestionBundle(), Outcome{
		Answers: []Answer{{QuestionID: "q1", CustomText: overCapWithTrailingSpace}, {QuestionID: "q2"}},
	})
	if err == nil || !IsValidationError(err) {
		t.Fatalf("expected a validation error for a raw value one rune over the cap (even though trimming would bring it under), got %v", err)
	}
}

// TestValidateOutcome_L16_BundleUnanswerableByAnyOutcome proves L16: a
// bundle whose sole question carries an empty question_id cannot be
// resolved by ANY outcome -- not by an answer, and not by a rejection.
// Upstream's completeClaimedClarificationMessages aborts its transaction on
// the first such message it meets, so every path here must fail validation
// pre-claim rather than let a rejection through to that 500.
func TestValidateOutcome_L16_BundleUnanswerableByAnyOutcome(t *testing.T) {
	l16 := []Question{{ID: ""}}

	if err := validateOutcome(l16, Outcome{Answers: []Answer{{QuestionID: ""}}}); err == nil || !IsValidationError(err) {
		t.Fatalf("expected a validation error for an answer with an empty question_id, got %v", err)
	}
	if err := validateOutcome(l16, Outcome{Answers: []Answer{{QuestionID: "anything"}}}); err == nil || !IsValidationError(err) {
		t.Fatalf("expected a validation error for an answer naming an unknown question_id, got %v", err)
	}
	if err := validateOutcome(l16, Outcome{Rejected: true}); err == nil || !IsValidationError(err) {
		t.Fatalf("expected L16 to reject a rejection outcome too, got %v", err)
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
// payloads once serialized — even when they are NOT byte-identical overall,
// because responded_at legitimately differs (the spec explicitly forbids
// proving this by freezing the clock: two real submissions never share a
// wall-clock timestamp). pending_id and responded_at are the two fields the
// server sets independently of caller input, so this test excludes exactly
// those two before comparing, rather than relying on them coincidentally
// matching.
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
	}, fixedNow().Add(37*time.Second))

	if a.RespondedAt.Equal(b.RespondedAt) {
		t.Fatalf("test setup bug: a and b must have different responded_at, both got %v", a.RespondedAt)
	}

	sa, err := SerializeResponse(a)
	if err != nil {
		t.Fatalf("SerializeResponse(a): %v", err)
	}
	sb, err := SerializeResponse(b)
	if err != nil {
		t.Fatalf("SerializeResponse(b): %v", err)
	}
	if sa == sb {
		t.Fatalf("a and b have different responded_at, so the full payloads must NOT be byte-identical (a stale assertion here would mask a bug that made responded_at stop varying):\na=%s\nb=%s", sa, sb)
	}

	fa := stripServerSetFields(t, sa)
	fb := stripServerSetFields(t, sb)
	if fa != fb {
		t.Fatalf("expected byte-identical answers/rejected/reject_reason once pending_id/responded_at are excluded:\na=%s\nb=%s", fa, fb)
	}
}

// stripServerSetFields removes the two fields SerializeResponse sets from
// server-side state (pending_id, responded_at) rather than from caller
// input, and re-serializes the rest with json.Marshal's deterministic
// alphabetical key ordering for a stable comparison.
func stripServerSetFields(t *testing.T, serialized string) string {
	t.Helper()
	var m map[string]json.RawMessage
	if err := json.Unmarshal([]byte(serialized), &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	delete(m, "pending_id")
	delete(m, "responded_at")
	out, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return string(out)
}
