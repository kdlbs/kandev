package clarification

import (
	"encoding/json"
	"time"

	"github.com/kandev/kandev/internal/clarification/protocol"
)

// serializedAttachment is the durable/browser projection of one answer
// attachment. It deliberately has no executor path: paths are generated only
// for live agent delivery and must never be persisted or returned to browsers.
type serializedAttachment struct {
	AttachmentID string `json:"attachment_id"`
	Type         string `json:"type"`
	Name         string `json:"name"`
	MimeType     string `json:"mime_type"`
	SizeBytes    int64  `json:"size_bytes"`
	DeliveryMode string `json:"delivery_mode"`
}

// serializedAnswer mirrors Answer but declares no `omitempty` for the legacy
// fields: every one is always present (M6a). Attachments stay omitted when
// empty so the existing file-free encoding is byte-compatible.
type serializedAnswer struct {
	QuestionID      string                 `json:"question_id"`
	SelectedOptions []string               `json:"selected_options"`
	CustomText      string                 `json:"custom_text"`
	Attachments     []serializedAttachment `json:"attachments,omitempty"`
	// Annotations stay omitted when empty for the same byte compatibility.
	Annotations []protocol.AnswerAnnotation `json:"annotations,omitempty"`
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
			Attachments:     serializeAnswerAttachments(a.Attachments),
			Annotations:     a.Annotations,
		})
	}
	b, err := json.Marshal(out)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func serializeAnswerAttachments(attachments []AnswerAttachment) []serializedAttachment {
	if len(attachments) == 0 {
		return nil
	}
	out := make([]serializedAttachment, 0, len(attachments))
	for _, attachment := range attachments {
		out = append(out, serializedAttachment{
			AttachmentID: attachment.AttachmentID,
			Type:         attachment.Type,
			Name:         attachment.Name,
			MimeType:     attachment.MimeType,
			SizeBytes:    attachment.SizeBytes,
			DeliveryMode: attachment.DeliveryMode,
		})
	}
	return out
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
			Attachments:     deserializeAnswerAttachments(a.Attachments),
			Annotations:     a.Annotations,
		})
	}
	return resp, nil
}

func deserializeAnswerAttachments(attachments []serializedAttachment) []AnswerAttachment {
	if len(attachments) == 0 {
		return nil
	}
	out := make([]AnswerAttachment, 0, len(attachments))
	for _, attachment := range attachments {
		out = append(out, AnswerAttachment{
			AttachmentID: attachment.AttachmentID,
			Type:         attachment.Type,
			Name:         attachment.Name,
			MimeType:     attachment.MimeType,
			SizeBytes:    attachment.SizeBytes,
			DeliveryMode: attachment.DeliveryMode,
		})
	}
	return out
}
