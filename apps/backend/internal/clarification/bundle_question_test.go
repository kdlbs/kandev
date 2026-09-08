package clarification

import (
	"testing"
	"time"

	taskmodels "github.com/kandev/kandev/internal/task/models"
)

func questionMetadata(pendingID, questionID string, index int, question map[string]any) map[string]any {
	meta := map[string]any{
		"pending_id":     pendingID,
		"question_index": index,
		"status":         "pending",
	}
	if questionID != "" {
		meta["question_id"] = questionID
	}
	if question != nil {
		meta["question"] = question
	}
	return meta
}

// TestOrderBundleMessages_ByQuestionIndex proves the primary D2/L5 sort key.
func TestOrderBundleMessages_ByQuestionIndex(t *testing.T) {
	msgs := []*taskmodels.Message{
		{ID: "m2", Metadata: questionMetadata("p1", "q2", 1, nil)},
		{ID: "m1", Metadata: questionMetadata("p1", "q1", 0, nil)},
	}
	ordered := orderBundleMessages(msgs)
	if ordered[0].ID != "m1" || ordered[1].ID != "m2" {
		t.Fatalf("got order %s, %s; want m1, m2", ordered[0].ID, ordered[1].ID)
	}
}

// TestOrderBundleMessages_TiebreaksByCreatedAtThenID proves D2's tiebreak: a
// legacy or corrupt bundle can have several messages all claiming
// question_index 0 (questionIndexFromMetadata returns 0 for a missing or
// unparseable index), and the order must still be total and reproducible.
func TestOrderBundleMessages_TiebreaksByCreatedAtThenID(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	msgs := []*taskmodels.Message{
		{ID: "z", CreatedAt: base, Metadata: questionMetadata("p1", "q3", 0, nil)},
		{ID: "a", CreatedAt: base, Metadata: questionMetadata("p1", "q1", 0, nil)},
		{ID: "m", CreatedAt: base.Add(-time.Second), Metadata: questionMetadata("p1", "q2", 0, nil)},
	}
	ordered := orderBundleMessages(msgs)
	got := []string{ordered[0].ID, ordered[1].ID, ordered[2].ID}
	want := []string{"m", "a", "z"} // earlier created_at first, then id ascending among ties
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got order %v, want %v", got, want)
		}
	}
}

// TestOrderBundleMessages_DoesNotMutateInput proves the sort works on a copy,
// since callers (resolver.go) pass the same slice to multiple helpers.
func TestOrderBundleMessages_DoesNotMutateInput(t *testing.T) {
	msgs := []*taskmodels.Message{
		{ID: "m2", Metadata: questionMetadata("p1", "q2", 1, nil)},
		{ID: "m1", Metadata: questionMetadata("p1", "q1", 0, nil)},
	}
	_ = orderBundleMessages(msgs)
	if msgs[0].ID != "m2" || msgs[1].ID != "m1" {
		t.Fatalf("input slice was mutated: %s, %s", msgs[0].ID, msgs[1].ID)
	}
}

// TestBundleQuestions_ParsesFullQuestionIncludingOptions proves this parser
// extracts options (unlike questionsFromMessages, which the resume-summary
// path uses and which never needs option identifiers for N8/N3a).
func TestBundleQuestions_ParsesFullQuestionIncludingOptions(t *testing.T) {
	msgs := []*taskmodels.Message{
		{
			ID: "m1",
			Metadata: questionMetadata("p1", "q1", 0, map[string]any{
				"id":     "q1",
				"title":  "Color",
				"prompt": "Pick a color",
				"options": []interface{}{
					map[string]interface{}{"option_id": "opt-a", "label": "Red", "description": "Red option"},
					map[string]interface{}{"option_id": "opt-b", "label": "Blue", "description": "Blue option"},
				},
			}),
		},
	}
	questions := bundleQuestions(msgs)
	if len(questions) != 1 {
		t.Fatalf("expected 1 question, got %d", len(questions))
	}
	q := questions[0]
	if q.ID != "q1" || q.Title != "Color" || q.Prompt != "Pick a color" {
		t.Fatalf("unexpected question: %+v", q)
	}
	if len(q.Options) != 2 || q.Options[0].ID != "opt-a" || q.Options[1].ID != "opt-b" {
		t.Fatalf("unexpected options: %+v", q.Options)
	}
	if q.Options[0].Label != "Red" || q.Options[0].Description != "Red option" {
		t.Fatalf("unexpected option fields: %+v", q.Options[0])
	}
}

// TestBundleQuestions_UnparseableQuestionMetadataDegradesGracefully proves
// L15: a message with unparseable `question` metadata still yields a
// Question carrying only its question_id, with no options and empty
// title/prompt rather than a panic or nil options causing a crash downstream.
func TestBundleQuestions_UnparseableQuestionMetadataDegradesGracefully(t *testing.T) {
	msgs := []*taskmodels.Message{
		{ID: "m1", Metadata: questionMetadata("p1", "q1", 0, nil)},
	}
	questions := bundleQuestions(msgs)
	if len(questions) != 1 {
		t.Fatalf("expected 1 question, got %d", len(questions))
	}
	if questions[0].ID != "q1" || questions[0].Prompt != "" || len(questions[0].Options) != 0 {
		t.Fatalf("unexpected degraded question: %+v", questions[0])
	}
}

// TestBundleQuestions_NoQuestionIDAnywhereYieldsEmptyID proves L16: a message
// with no question_id in either metadata.question_id or metadata.question.id
// yields a Question with an empty ID rather than an error.
func TestBundleQuestions_NoQuestionIDAnywhereYieldsEmptyID(t *testing.T) {
	msgs := []*taskmodels.Message{
		{ID: "m1", Metadata: questionMetadata("p1", "", 0, map[string]any{"prompt": "no id here"})},
	}
	questions := bundleQuestions(msgs)
	if len(questions) != 1 {
		t.Fatalf("expected 1 question, got %d", len(questions))
	}
	if questions[0].ID != "" {
		t.Fatalf("expected empty question_id, got %q", questions[0].ID)
	}
}

// TestBundleQuestions_OrdersBeforeParsing proves bundleQuestions applies D2
// ordering before building the Question list, so the result reflects L5's
// order rather than the caller's slice order.
func TestBundleQuestions_OrdersBeforeParsing(t *testing.T) {
	msgs := []*taskmodels.Message{
		{ID: "m2", Metadata: questionMetadata("p1", "q2", 1, map[string]any{"id": "q2"})},
		{ID: "m1", Metadata: questionMetadata("p1", "q1", 0, map[string]any{"id": "q1"})},
	}
	questions := bundleQuestions(msgs)
	if questions[0].ID != "q1" || questions[1].ID != "q2" {
		t.Fatalf("got %q, %q; want q1, q2", questions[0].ID, questions[1].ID)
	}
}

// TestBundleQuestionStatuses_L4Shape proves the L4 wire shape: question_id
// (not id), title, prompt, status and options, with options carrying
// option_id/label/description per the existing Option JSON tags.
func TestBundleQuestionStatuses_L4Shape(t *testing.T) {
	msgs := []*taskmodels.Message{
		{
			ID: "m1",
			Metadata: questionMetadata("p1", "q1", 0, map[string]any{
				"id":     "q1",
				"title":  "Color",
				"prompt": "Pick a color",
				"options": []interface{}{
					map[string]interface{}{"option_id": "opt-a", "label": "Red", "description": "Red option"},
				},
			}),
		},
	}
	statuses := BundleQuestionStatuses(msgs)
	if len(statuses) != 1 {
		t.Fatalf("expected 1 question, got %d", len(statuses))
	}
	got := statuses[0]
	if got.QuestionID != "q1" || got.Title != "Color" || got.Prompt != "Pick a color" {
		t.Fatalf("unexpected question: %+v", got)
	}
	if got.Status != "pending" {
		t.Fatalf("expected status pending, got %q", got.Status)
	}
	if len(got.Options) != 1 || got.Options[0].ID != "opt-a" || got.Options[0].Label != "Red" {
		t.Fatalf("unexpected options: %+v", got.Options)
	}
}

// TestBundleQuestionStatuses_D3AbsentStatusCountsAsPending proves D3: a
// message with no recognized status metadata is reported as "pending"
// rather than an empty string or the raw unrecognized value.
func TestBundleQuestionStatuses_D3AbsentStatusCountsAsPending(t *testing.T) {
	msgs := []*taskmodels.Message{
		{ID: "m1", Metadata: map[string]any{"pending_id": "p1", "question_index": 0, "question_id": "q1"}},
	}
	statuses := BundleQuestionStatuses(msgs)
	if statuses[0].Status != "pending" {
		t.Fatalf("expected pending for absent status, got %q", statuses[0].Status)
	}
}

// TestBundleQuestionStatuses_D3UnrecognizedStatusCountsAsPending proves the
// other half of D3: a corrupt/legacy status string that isn't one of the
// five known values also degrades to "pending" rather than passing through
// verbatim.
func TestBundleQuestionStatuses_D3UnrecognizedStatusCountsAsPending(t *testing.T) {
	msgs := []*taskmodels.Message{
		{ID: "m1", Metadata: map[string]any{"pending_id": "p1", "question_index": 0, "question_id": "q1", "status": "bogus"}},
	}
	statuses := BundleQuestionStatuses(msgs)
	if statuses[0].Status != "pending" {
		t.Fatalf("expected pending for unrecognized status, got %q", statuses[0].Status)
	}
}

// TestBundleQuestionStatuses_RecognizedTerminalStatusPassesThrough proves a
// recognized terminal status (e.g. answered) is reported as-is, not
// collapsed to pending.
func TestBundleQuestionStatuses_RecognizedTerminalStatusPassesThrough(t *testing.T) {
	msgs := []*taskmodels.Message{
		{ID: "m1", Metadata: map[string]any{"pending_id": "p1", "question_index": 0, "question_id": "q1", "status": "answered"}},
	}
	statuses := BundleQuestionStatuses(msgs)
	if statuses[0].Status != "answered" {
		t.Fatalf("expected answered to pass through, got %q", statuses[0].Status)
	}
}

// TestBundleQuestionStatuses_OptionsNeverNil proves L4a: a degraded
// question with no parseable options still yields an empty slice, not nil,
// so JSON marshaling emits [] rather than null.
func TestBundleQuestionStatuses_OptionsNeverNil(t *testing.T) {
	msgs := []*taskmodels.Message{
		{ID: "m1", Metadata: questionMetadata("p1", "q1", 0, nil)},
	}
	statuses := BundleQuestionStatuses(msgs)
	if statuses[0].Options == nil {
		t.Fatalf("expected non-nil empty options slice, got nil")
	}
	if len(statuses[0].Options) != 0 {
		t.Fatalf("expected empty options, got %+v", statuses[0].Options)
	}
}
