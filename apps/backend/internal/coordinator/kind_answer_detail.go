package coordinator

import "strings"

const answerDetailRunes = 200

// clarificationDetail is the one-line approved detail of a delivered
// clarification: each question's text with the chosen option labels or text.
func clarificationDetail(spec answerSpec, draft answerDraft) string {
	parts := make([]string, 0, len(draft.Answers))
	for _, e := range draft.Answers {
		question := e.QuestionID
		labels := map[string]string{}
		for _, q := range spec.Snapshot.Questions {
			if q.ID != e.QuestionID {
				continue
			}
			question = q.Text
			for _, o := range q.Options {
				labels[o.ID] = o.Label
			}
		}
		chosen := make([]string, 0, len(e.OptionIDs)+1)
		for _, id := range e.OptionIDs {
			if l := labels[id]; l != "" {
				chosen = append(chosen, l)
			} else {
				chosen = append(chosen, id)
			}
		}
		if e.CustomText != "" {
			chosen = append(chosen, e.CustomText)
		}
		parts = append(parts, question+": "+strings.Join(chosen, ", "))
	}
	return truncateRunes(strings.Join(parts, "; "), answerDetailRunes)
}

// permissionDetail is the approved detail of a delivered permission answer.
func permissionDetail(spec answerSpec, draft answerDraft) string {
	if spec.Snapshot.Permission == nil {
		return truncateRunes(draft.OptionID, answerDetailRunes)
	}
	name := draft.OptionID
	for _, o := range spec.Snapshot.Permission.Options {
		if o.ID == draft.OptionID && o.Name != "" {
			name = o.Name
		}
	}
	return truncateRunes(spec.Snapshot.Permission.Title+": "+name, answerDetailRunes)
}
