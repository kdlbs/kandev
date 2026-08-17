package clarification

import (
	"sort"

	taskmodels "github.com/kandev/kandev/internal/task/models"
)

// orderBundleMessages sorts a copy of msgs into D2/L5's total order:
// question_index ascending, then message created_at ascending, then message
// id ascending. questionIndexFromMetadata returns 0 for a missing or
// unparseable index, so a legacy or corrupt bundle can present several
// messages all claiming index 0 — the created_at/id tiebreaks make the
// composite total rather than merely a best effort.
func orderBundleMessages(msgs []*taskmodels.Message) []*taskmodels.Message {
	sorted := make([]*taskmodels.Message, len(msgs))
	copy(sorted, msgs)
	sort.SliceStable(sorted, func(i, j int) bool {
		ii, ij := questionIndexFromMetadata(sorted[i].Metadata), questionIndexFromMetadata(sorted[j].Metadata)
		if ii != ij {
			return ii < ij
		}
		if !sorted[i].CreatedAt.Equal(sorted[j].CreatedAt) {
			return sorted[i].CreatedAt.Before(sorted[j].CreatedAt)
		}
		return sorted[i].ID < sorted[j].ID
	})
	return sorted
}

// bundleQuestions builds the D2/L5-ordered Question list for a bundle,
// parsing each message's full question metadata (id, title, prompt and
// options). Unlike questionsFromMessages (handlers.go), used only for the
// resume-summary path, this parses options too: N8 needs them to validate
// selected_options membership and N3a rule 2 needs their order.
//
// A message with unparseable `question` metadata (L15) yields a Question
// carrying only its question_id, with empty title/prompt and no options. A
// message with no question_id anywhere (L16) yields a Question with an
// empty ID.
func bundleQuestions(msgs []*taskmodels.Message) []Question {
	ordered := orderBundleMessages(msgs)
	out := make([]Question, 0, len(ordered))
	for _, m := range ordered {
		out = append(out, questionFromMessageMetadataFull(m.Metadata))
	}
	return out
}

// QuestionStatus is the spec L4 wire shape for one question inside a listed
// clarification bundle: the same fields as Question but keyed by
// question_id (not id, which list_pending_questions_kandev callers never
// see), plus the question's D3 effective status. Options is never nil (L4a).
type QuestionStatus struct {
	QuestionID string   `json:"question_id"`
	Title      string   `json:"title"`
	Prompt     string   `json:"prompt"`
	Status     string   `json:"status"`
	Options    []Option `json:"options"`
}

// BundleQuestionStatuses builds the D2/L5-ordered, L4-shaped question list
// for a bundle, one entry per durable message, each carrying its D3
// effective status: the message's own recorded status when it is one of
// the five known values, otherwise "pending" — an absent or corrupt status
// must not make a question look unanswerable to a list_pending_questions_kandev
// caller (D3).
func BundleQuestionStatuses(msgs []*taskmodels.Message) []QuestionStatus {
	ordered := orderBundleMessages(msgs)
	out := make([]QuestionStatus, 0, len(ordered))
	for _, m := range ordered {
		q := questionFromMessageMetadataFull(m.Metadata)
		options := q.Options
		if options == nil {
			options = []Option{}
		}
		out = append(out, QuestionStatus{
			QuestionID: q.ID,
			Title:      q.Title,
			Prompt:     q.Prompt,
			Status:     effectiveMessageStatus(m),
			Options:    options,
		})
	}
	return out
}

// effectiveMessageStatus implements D3: a message's status counts as
// pending unless it carries one of the five recognized terminal-or-pending
// status strings.
func effectiveMessageStatus(msg *taskmodels.Message) string {
	status := stringFromMetadata(msg.Metadata, "status")
	switch Status(status) {
	case StatusPending, StatusAnswered, StatusRejected, StatusExpired, StatusCancelled:
		return status
	default:
		return string(StatusPending)
	}
}

func questionFromMessageMetadataFull(meta map[string]any) Question {
	q := Question{ID: questionIDFromMetadata(meta)}
	qData, ok := meta[metaQuestionKey].(map[string]any)
	if !ok {
		return q
	}
	if v, ok := qData["prompt"].(string); ok {
		q.Prompt = v
	}
	if v, ok := qData["title"].(string); ok {
		q.Title = v
	}
	q.Options = optionsFromQuestionData(qData)
	return q
}

// questionIDFromMetadata resolves a message's question_id, falling back to
// the nested metadata.question.id when the flat metadata.question_id key is
// absent or empty (L16). Every reader of a message's question_id — building
// the Question/QuestionStatus list (list_pending_questions_kandev, N6a's
// expected-answer set) and resolver.go's applyMessageUpdates, which matches
// a winning outcome's per-question answers by this same id — SHALL go
// through this one function, so the two paths cannot compute different ids
// for the same message and silently disagree again.
func questionIDFromMetadata(meta map[string]any) string {
	if id := stringFromMetadata(meta, metaQuestionIDKey); id != "" {
		return id
	}
	if qData, ok := meta[metaQuestionKey].(map[string]any); ok {
		if v, ok := qData["id"].(string); ok {
			return v
		}
	}
	return ""
}

// optionsFromQuestionData reads a question's `options` array as stored by
// CreateClarificationRequestMessages (`backendapp/adapters.go`): a
// []interface{} of map[string]interface{} with option_id/label/description
// string keys, the shape json.Unmarshal produces after a database round
// trip. Any entry that does not match this shape is skipped rather than
// aborting the whole question (L15's degrade-gracefully rule).
func optionsFromQuestionData(qData map[string]any) []Option {
	raw, ok := qData["options"].([]interface{})
	if !ok {
		return nil
	}
	out := make([]Option, 0, len(raw))
	for _, item := range raw {
		om, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		var opt Option
		if v, ok := om["option_id"].(string); ok {
			opt.ID = v
		}
		if v, ok := om["label"].(string); ok {
			opt.Label = v
		}
		if v, ok := om["description"].(string); ok {
			opt.Description = v
		}
		out = append(out, opt)
	}
	return out
}
