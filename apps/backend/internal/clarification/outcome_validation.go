package clarification

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

// Outcome is the caller-supplied resolution ResolveBundle is asked to
// record: either an answers submission or a rejection. Exactly one of
// Rejected or a non-empty Answers submission applies; N8a additionally
// forbids combining Rejected with a non-empty Answers. A cancel (A9) does
// not go through ResolveBundle at all, so it has no representation here.
type Outcome struct {
	Answers      []Answer
	Rejected     bool
	RejectReason string
}

// answerTextRuneCap is N8b's shared limit on both an answer's custom_text
// and a rejection's reason, counted in UTF-8 runes rather than bytes so a
// non-Latin answer is not cut short at a fraction of its visible length.
const answerTextRuneCap = 2000

// validationError marks an outcome that failed N6-N8b validation. It SHALL
// NOT claim the bundle (N8c). REST/MCP layers map it to a 400 (N6-N8b's
// R10 row); it must never be confused with ErrBundleNotFound (404) or a
// partialApplicationError (500).
type validationError struct{ msg string }

func (e *validationError) Error() string { return e.msg }

func newValidationError(format string, args ...any) error {
	return &validationError{msg: fmt.Sprintf(format, args...)}
}

// IsValidationError reports whether err is a validateOutcome failure
// (N6-N8b), for REST/MCP layers deciding a 400 response.
func IsValidationError(err error) bool {
	var ve *validationError
	return errors.As(err, &ve)
}

// validateOutcome applies L16, N6, N6a, N7, N8, N8a and N8b against
// questions, the bundle's D2/L5-ordered question list. Callers SHALL NOT
// call this for a cancel outcome (X5 skips step 3 entirely).
func validateOutcome(questions []Question, o Outcome) error {
	if err := validateNoMissingQuestionID(questions); err != nil {
		return err
	}
	if o.Rejected {
		return validateRejection(o)
	}
	if utf8.RuneCountInString(o.RejectReason) > answerTextRuneCap {
		// Reachable even on the answered path: RejectReason is a stray
		// value M6 discards from the stored response, but N8b's cap still
		// binds the caller-supplied field itself before that discard.
		return newValidationError("reason exceeds %d characters", answerTextRuneCap)
	}
	return validateAnswers(questions, o.Answers)
}

// validateNoMissingQuestionID applies L16 ahead of both the answer and the
// rejection branch: a bundle in which any message carries an empty or
// absent question_id cannot be resolved by any outcome, because upstream's
// completeClaimedClarificationMessages aborts its transaction on the first
// such message it meets. Checking this before dispatching to either branch
// is what makes the rejection path name the condition pre-claim instead of
// surfacing that abort as a 500 (R4a).
func validateNoMissingQuestionID(questions []Question) error {
	for _, q := range questions {
		if q.ID == "" {
			return newValidationError("bundle contains a message with a missing question_id")
		}
	}
	return nil
}

func validateRejection(o Outcome) error {
	if len(o.Answers) > 0 {
		return newValidationError("rejected may not be combined with a non-empty answers array") // N8a
	}
	if utf8.RuneCountInString(o.RejectReason) > answerTextRuneCap {
		return newValidationError("reason exceeds %d characters", answerTextRuneCap) // N8b
	}
	return nil
}

func validateAnswers(questions []Question, answers []Answer) error {
	if len(answers) != len(questions) {
		// N8a's count-mismatch case (rejected:false, answers shorter or
		// longer than the bundle) also lands here, including the empty
		// array against a single-question bundle N8a calls out explicitly.
		return newValidationError("expected exactly %d answer(s), got %d", len(questions), len(answers)) // N6 condition 1
	}

	byID := make(map[string]Question, len(questions))
	for _, q := range questions {
		byID[q.ID] = q
	}

	seen := make(map[string]bool, len(answers))
	for i, a := range answers {
		if a.QuestionID == "" {
			return newValidationError("answer %d is missing question_id", i+1) // N6 condition 2
		}
		q, ok := byID[a.QuestionID]
		if !ok {
			return newValidationError("answer %d references unknown question_id %q", i+1, a.QuestionID) // N6 condition 3
		}
		if seen[a.QuestionID] {
			return newValidationError("answer %d repeats question_id %q", i+1, a.QuestionID) // N6 condition 4
		}
		seen[a.QuestionID] = true

		if err := validateSelectedOptions(i, a, q); err != nil {
			return err
		}
		if utf8.RuneCountInString(a.CustomText) > answerTextRuneCap {
			return newValidationError("answer %d custom_text exceeds %d characters", i+1, answerTextRuneCap) // N8b
		}
	}
	return nil
}

// validateSelectedOptions applies N8: membership only, never cardinality —
// a question is not marked single- or multi-select, so naming several valid
// option IDs is accepted.
func validateSelectedOptions(i int, a Answer, q Question) error {
	if len(a.SelectedOptions) == 0 {
		return nil
	}
	valid := make(map[string]bool, len(q.Options))
	for _, opt := range q.Options {
		valid[opt.ID] = true
	}
	for _, optID := range a.SelectedOptions {
		if !valid[optID] {
			return newValidationError("answer %d references unknown option_id %q", i+1, optID)
		}
	}
	return nil
}

// buildOutcomeResponse computes the resolution's status and its M6/N3a
// response payload from a validated outcome.
func buildOutcomeResponse(pendingID string, questions []Question, o Outcome, now time.Time) (string, *Response) {
	if o.Rejected {
		return string(StatusRejected), &Response{
			PendingID:    pendingID,
			Answers:      []Answer{},
			Rejected:     true,
			RejectReason: strings.TrimSpace(o.RejectReason),
			RespondedAt:  now,
		}
	}
	return string(StatusAnswered), normalizeAnswered(pendingID, questions, o.Answers, now)
}

// normalizeAnswered builds the N3a-normalized Response for an answered
// outcome: answers ordered by the bundle's own question order (rule 1), not
// the caller's submission order.
func normalizeAnswered(pendingID string, questions []Question, answers []Answer, now time.Time) *Response {
	byID := make(map[string]Answer, len(answers))
	for _, a := range answers {
		byID[a.QuestionID] = a
	}
	out := make([]Answer, 0, len(questions))
	for _, q := range questions {
		a, ok := byID[q.ID]
		if !ok {
			continue
		}
		out = append(out, normalizeAnswer(q, a))
	}
	return &Response{
		PendingID:    pendingID,
		Answers:      out,
		Rejected:     false,
		RejectReason: "", // M6: discarded even if the caller supplied one
		RespondedAt:  now,
	}
}

// normalizeAnswer applies N3a rules 2 and 3 to a single answer entry:
// selected_options ordered by the option's position in the question's
// options array with exact duplicates removed, and custom_text trimmed of
// leading/trailing whitespace with no other transformation.
func normalizeAnswer(q Question, a Answer) Answer {
	position := make(map[string]int, len(q.Options))
	for i, opt := range q.Options {
		position[opt.ID] = i
	}

	deduped := make([]string, 0, len(a.SelectedOptions))
	seen := make(map[string]bool, len(a.SelectedOptions))
	for _, id := range a.SelectedOptions {
		if seen[id] {
			continue
		}
		seen[id] = true
		deduped = append(deduped, id)
	}
	sort.SliceStable(deduped, func(i, j int) bool {
		return position[deduped[i]] < position[deduped[j]]
	})

	return Answer{
		QuestionID:      q.ID,
		SelectedOptions: deduped,
		CustomText:      strings.TrimSpace(a.CustomText),
	}
}
