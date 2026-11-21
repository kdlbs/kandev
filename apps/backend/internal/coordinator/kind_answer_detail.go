package coordinator

import (
	"fmt"
	"strings"
)

const answerDetailRunes = 200

// clarificationDetail is the one-line approved detail of a delivered
// clarification: "Q<n>: <text>" per answered question, where n is the
// question's position in the bundle and text is the chosen option labels or,
// for an entry without options, its free text.
func clarificationDetail(target *ClarificationTarget, draft answerDraft) string {
	parts := make([]string, 0, len(draft.Answers))
	for _, e := range draft.Answers {
		n, labels := questionPosition(target, e.QuestionID)
		text := e.CustomText
		if len(e.OptionIDs) > 0 {
			chosen := make([]string, len(e.OptionIDs))
			for i, id := range e.OptionIDs {
				chosen[i] = optionLabel(labels, id)
			}
			text = strings.Join(chosen, ", ")
		}
		parts = append(parts, fmt.Sprintf("Q%d: %s", n, text))
	}
	return truncateRunes(strings.Join(parts, "; "), answerDetailRunes)
}

// questionPosition returns a question's 1-based position in the bundle and its
// option labels by id.
func questionPosition(target *ClarificationTarget, questionID string) (int, map[string]string) {
	for i, q := range target.Questions {
		if q.ID != questionID {
			continue
		}
		labels := make(map[string]string, len(q.Options))
		for _, o := range q.Options {
			labels[o.ID] = o.Label
		}
		return i + 1, labels
	}
	return 0, nil
}

func optionLabel(labels map[string]string, id string) string {
	if l := labels[id]; l != "" {
		return l
	}
	return id
}

// permissionDetail is the approved detail of a delivered permission answer:
// the chosen option's name.
func permissionDetail(target *PermissionTarget, draft answerDraft) string {
	name := draft.OptionID
	for _, o := range target.Options {
		if o.ID == draft.OptionID && o.Name != "" {
			name = o.Name
		}
	}
	return truncateRunes(name, answerDetailRunes)
}
