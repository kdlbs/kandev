package coordinator

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"
)

const (
	maxAnswerSources  = 10
	maxSourceRefRunes = 300
	maxSourceLabelLen = 120
)

var answerTopLevelMembers = map[string]bool{
	fieldTaskID: true, fieldTarget: true, fieldPendingID: true, fieldDraft: true, fieldSources: true,
	fieldConfidence: true, fieldNeedsDecision: true, fieldRationale: true, fieldStandingOrderIDs: true,
}

// answerShape is the stage 1 result: every member decoded and shape-checked
// but not yet checked against a live target.
type answerShape struct {
	TaskID        string
	Target        string
	PendingID     string
	Draft         answerDraft
	Sources       []answerSource
	Confidence    string
	NeedsDecision bool
	Rationale     string
	OrderIDs      []string
}

func fieldErr(field, message string) *FieldError { return &FieldError{Field: field, Message: message} }

// firstUnknown returns the alphabetically first key of members not in allowed.
func firstUnknown(members map[string]json.RawMessage, allowed func(string) bool) (string, bool) {
	var unknown []string
	for name := range members {
		if !allowed(name) {
			unknown = append(unknown, name)
		}
	}
	if len(unknown) == 0 {
		return "", false
	}
	sort.Strings(unknown)
	return unknown[0], true
}

func isNull(raw json.RawMessage, present bool) bool {
	return !present || bytes.Equal(bytes.TrimSpace(raw), []byte("null"))
}

// answerString decodes a string member; an absent or null one is missing.
func answerString(members map[string]json.RawMessage, name string, required bool) (string, error) {
	raw, present := members[name]
	if isNull(raw, present) {
		if required {
			return "", fieldErr(name, name+" is required")
		}
		return "", nil
	}
	var v string
	if err := json.Unmarshal(raw, &v); err != nil {
		return "", fieldErr(name, name+" must be a string")
	}
	v = strings.TrimSpace(v)
	if required && v == "" {
		return "", fieldErr(name, name+" is required")
	}
	return v, nil
}

func decodeObject(raw json.RawMessage, field string) (map[string]json.RawMessage, error) {
	var members map[string]json.RawMessage
	if err := json.Unmarshal(raw, &members); err != nil || members == nil {
		return nil, fieldErr(field, field+" must be an object")
	}
	return members, nil
}

// parseAnswerShape is stage 1: the unknown top-level member, then the first
// failing member in the documented order.
func parseAnswerShape(args json.RawMessage) (*answerShape, error) {
	members, err := decodeObject(args, fieldBody)
	if err != nil {
		return nil, fieldErr(fieldBody, "arguments are malformed")
	}
	if name, bad := firstUnknown(members, func(n string) bool { return answerTopLevelMembers[n] }); bad {
		return nil, fieldErr(name, messageUnknownField)
	}
	shape := &answerShape{}
	for _, step := range []func(map[string]json.RawMessage, *answerShape) error{
		decodeAnswerIdentity, decodeAnswerDraft, decodeAnswerSources, decodeAnswerMeta, decodeAnswerOrderIDs,
	} {
		if err := step(members, shape); err != nil {
			return nil, err
		}
	}
	return shape, nil
}

func decodeAnswerIdentity(members map[string]json.RawMessage, shape *answerShape) error {
	var err error
	if shape.TaskID, err = answerString(members, fieldTaskID, true); err != nil {
		return err
	}
	if shape.Target, err = answerString(members, fieldTarget, true); err != nil {
		return err
	}
	if shape.Target != answerTargetClarification && shape.Target != answerTargetPermission {
		return fieldErr(fieldTarget, "target must be clarification or permission")
	}
	shape.PendingID, err = answerString(members, fieldPendingID, true)
	return err
}

func decodeAnswerDraft(members map[string]json.RawMessage, shape *answerShape) error {
	raw, present := members[fieldDraft]
	if isNull(raw, present) {
		return fieldErr(fieldDraft, "draft is required")
	}
	draft, err := parseAnswerDraft(raw, shape.Target)
	shape.Draft = draft
	return err
}

func decodeAnswerSources(members map[string]json.RawMessage, shape *answerShape) error {
	raw, present := members[fieldSources]
	shape.Sources = []answerSource{}
	if isNull(raw, present) {
		return nil
	}
	var entries []json.RawMessage
	if err := json.Unmarshal(raw, &entries); err != nil {
		return fieldErr(fieldSources, "sources must be a list")
	}
	if len(entries) > maxAnswerSources {
		return fieldErr(fieldSources, fmt.Sprintf("sources may hold at most %d entries", maxAnswerSources))
	}
	for i, entry := range entries {
		src, err := parseAnswerSource(i, entry)
		if err != nil {
			return err
		}
		shape.Sources = append(shape.Sources, src)
	}
	return nil
}

func parseAnswerSource(i int, raw json.RawMessage) (answerSource, error) {
	path := fmt.Sprintf("%s[%d]", fieldSources, i)
	members, err := decodeObject(raw, path)
	if err != nil {
		return answerSource{}, err
	}
	if name, bad := firstUnknown(members, func(n string) bool { return n == "ref" || n == "label" }); bad {
		return answerSource{}, fieldErr(path+"."+name, messageUnknownField)
	}
	ref, err := answerString(members, "ref", false)
	if err != nil || ref == "" || utf8.RuneCountInString(ref) > maxSourceRefRunes {
		return answerSource{}, fieldErr(fieldSources, fmt.Sprintf("%s.ref must be 1 to %d characters", path, maxSourceRefRunes))
	}
	label, err := answerString(members, "label", false)
	if err != nil || utf8.RuneCountInString(label) > maxSourceLabelLen {
		return answerSource{}, fieldErr(fieldSources, fmt.Sprintf("%s.label must be at most %d characters", path, maxSourceLabelLen))
	}
	return answerSource{Ref: ref, Label: label}, nil
}

func decodeAnswerMeta(members map[string]json.RawMessage, shape *answerShape) error {
	var err error
	if shape.Confidence, err = answerString(members, fieldConfidence, true); err != nil {
		return err
	}
	switch shape.Confidence {
	case "low", "med", "high":
	default:
		return fieldErr(fieldConfidence, "confidence must be low, med or high")
	}
	raw, present := members[fieldNeedsDecision]
	if isNull(raw, present) {
		return fieldErr(fieldNeedsDecision, "needs_decision is required")
	}
	if err := json.Unmarshal(raw, &shape.NeedsDecision); err != nil {
		return fieldErr(fieldNeedsDecision, "needs_decision must be a boolean")
	}
	rationale, present := members[fieldRationale]
	if !isNull(rationale, present) {
		if err := json.Unmarshal(rationale, &shape.Rationale); err != nil {
			return fieldErr(fieldRationale, "rationale must be a string")
		}
	}
	return checkRationale(shape.Rationale)
}

func decodeAnswerOrderIDs(members map[string]json.RawMessage, shape *answerShape) error {
	raw, present := members[fieldStandingOrderIDs]
	shape.OrderIDs = []string{}
	if !isNull(raw, present) {
		if err := json.Unmarshal(raw, &shape.OrderIDs); err != nil {
			return fieldErr(fieldStandingOrderIDs, "standing_order_ids must be a list of strings")
		}
	}
	return checkOrderIDsShape(shape.OrderIDs)
}

// parseAnswerDraft shape-checks a draft for a target and returns it
// normalized. Entries are checked in order; an entry left with neither an
// option id nor text is empty_answer.
func parseAnswerDraft(raw json.RawMessage, target string) (answerDraft, error) {
	members, err := decodeObject(raw, fieldDraft)
	if err != nil {
		return answerDraft{}, err
	}
	allowed := "answers"
	if target == answerTargetPermission {
		allowed = "option_id"
	}
	if name, bad := firstUnknown(members, func(n string) bool { return n == allowed }); bad {
		return answerDraft{}, fieldErr(fieldDraft+"."+name, messageUnknownField)
	}
	if target == answerTargetPermission {
		id, err := answerString(members, "option_id", true)
		if err != nil {
			return answerDraft{}, fieldErr(fieldDraft, "option_id must be a non-empty string")
		}
		return answerDraft{OptionID: id}, nil
	}
	rawAnswers, present := members["answers"]
	if isNull(rawAnswers, present) {
		return answerDraft{}, fieldErr(fieldDraft, "answers is required")
	}
	var entries []json.RawMessage
	if err := json.Unmarshal(rawAnswers, &entries); err != nil {
		return answerDraft{}, fieldErr(fieldDraft, "answers must be a list")
	}
	draft := answerDraft{Answers: make([]answerDraftEntry, 0, len(entries))}
	for i, entry := range entries {
		e, err := parseDraftEntry(i, entry)
		if err != nil {
			return answerDraft{}, err
		}
		draft.Answers = append(draft.Answers, e)
	}
	return draft, nil
}

func parseDraftEntry(i int, raw json.RawMessage) (answerDraftEntry, error) {
	path := fmt.Sprintf("%s.answers[%d]", fieldDraft, i)
	members, err := decodeObject(raw, fieldDraft)
	if err != nil {
		return answerDraftEntry{}, fieldErr(fieldDraft, path+" must be an object")
	}
	if name, bad := firstUnknown(members, func(n string) bool {
		return n == "question_id" || n == "option_ids" || n == "custom_text"
	}); bad {
		return answerDraftEntry{}, fieldErr(path+"."+name, messageUnknownField)
	}
	var e answerDraftEntry
	if e.QuestionID, err = answerString(members, "question_id", true); err != nil {
		return answerDraftEntry{}, fieldErr(fieldDraft, path+".question_id must be a non-empty string")
	}
	if ids, present := members["option_ids"]; !isNull(ids, present) {
		if err := json.Unmarshal(ids, &e.OptionIDs); err != nil {
			return answerDraftEntry{}, fieldErr(fieldDraft, path+".option_ids must be a list of strings")
		}
	}
	if text, present := members["custom_text"]; !isNull(text, present) {
		if err := json.Unmarshal(text, &e.CustomText); err != nil {
			return answerDraftEntry{}, fieldErr(fieldDraft, path+".custom_text must be a string")
		}
	}
	e = normalizeEntry(e)
	if len(e.OptionIDs) == 0 && e.CustomText == "" {
		return answerDraftEntry{}, fieldErr(fieldDraft, messageEmptyAnswer)
	}
	return e, nil
}

// normalizeEntry trims the text, de-duplicates the option ids keeping the
// first of each, and drops empty members.
func normalizeEntry(e answerDraftEntry) answerDraftEntry {
	e.CustomText = strings.TrimSpace(e.CustomText)
	seen := make(map[string]bool, len(e.OptionIDs))
	var ids []string
	for _, id := range e.OptionIDs {
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		ids = append(ids, id)
	}
	e.OptionIDs = ids
	return e
}

// normalizeDraft is the one normalization applied before a draft is
// validated, stored or compared.
func normalizeDraft(d answerDraft) answerDraft {
	if d.Answers != nil {
		out := make([]answerDraftEntry, len(d.Answers))
		for i, e := range d.Answers {
			out[i] = normalizeEntry(e)
		}
		d.Answers = out
	}
	return d
}
