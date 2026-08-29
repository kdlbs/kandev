package clarification

import (
	"encoding/json"
	"time"
)

// serializedAnswer mirrors Answer but declares no `omitempty`: every key is
// always present (M6a).
type serializedAnswer struct {
	QuestionID      string   `json:"question_id"`
	SelectedOptions []string `json:"selected_options"`
	CustomText      string   `json:"custom_text"`
}

// serializedResponse mirrors Response but declares no `omitempty`: every key
// is always present (M6a, N3a rule 4). This is a dedicated wire shape for the
// durable answer record only — clarification.Response's own struct tags
// (`types.go:44-52`) are unchanged, since they are ask_user_question_kandev's
// frozen tool-result shape (M6a, *Out of scope*).
type serializedResponse struct {
	PendingID    string             `json:"pending_id"`
	Answers      []serializedAnswer `json:"answers"`
	Rejected     bool               `json:"rejected"`
	RejectReason string             `json:"reject_reason"`
	RespondedAt  time.Time          `json:"responded_at"`
}

// SerializeResponse produces the M6a/N3a-compliant JSON encoding of resp: the
// durable answer value that gets replayed verbatim to losers (R2, N4).
// `answers`, `selected_options` and
// `reject_reason` are always emitted — as `[]` or `""` when empty, never
// omitted or `null` — independently of clarification.Response's own
// `omitempty` tags, which marshalling the struct directly would honor.
func SerializeResponse(resp *Response) (string, error) {
	out := serializedResponse{
		PendingID:    resp.PendingID,
		Answers:      make([]serializedAnswer, 0, len(resp.Answers)),
		Rejected:     resp.Rejected,
		RejectReason: resp.RejectReason,
		RespondedAt:  resp.RespondedAt,
	}
	for _, a := range resp.Answers {
		out.Answers = append(out.Answers, serializedAnswer{
			QuestionID:      a.QuestionID,
			SelectedOptions: append([]string{}, a.SelectedOptions...),
			CustomText:      a.CustomText,
		})
	}
	b, err := json.Marshal(out)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// DeserializeResponse parses a `response` column value written by
// SerializeResponse back into a clarification.Response, for replay to losers
// (R2, N4) and for reads of an already-resolved bundle.
func DeserializeResponse(data string) (*Response, error) {
	var in serializedResponse
	if err := json.Unmarshal([]byte(data), &in); err != nil {
		return nil, err
	}
	resp := &Response{
		PendingID:    in.PendingID,
		Answers:      make([]Answer, 0, len(in.Answers)),
		Rejected:     in.Rejected,
		RejectReason: in.RejectReason,
		RespondedAt:  in.RespondedAt,
	}
	for _, a := range in.Answers {
		resp.Answers = append(resp.Answers, Answer{
			QuestionID:      a.QuestionID,
			SelectedOptions: append([]string{}, a.SelectedOptions...),
			CustomText:      a.CustomText,
		})
	}
	return resp, nil
}
