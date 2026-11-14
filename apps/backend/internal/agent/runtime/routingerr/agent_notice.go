package routingerr

import (
	"regexp"
	"strings"
	"unicode"
)

// ClassifyAgentNotice classifies only signatures opening the assistant output.
// An agent quoting another session's failure is not itself provider evidence.
func ClassifyAgentNotice(in Input) *Error {
	text := strings.TrimLeftFunc(in.Stderr, unicode.IsSpace)
	if text == "" {
		return nil
	}
	r, ok := leadingNoticeRule(in.ProviderID, text)
	if !ok {
		return nil
	}
	result := Classify(Input{Phase: in.Phase, ProviderID: in.ProviderID, Stderr: text, OccurredAt: in.OccurredAt, ResetHint: in.ResetHint})
	if result.ClassifierRule != r.id {
		return nil
	}
	return result
}

var noticeErrorLabel = regexp.MustCompile(`(?i)^[a-z0-9_]*error:\s*`)

func leadingNoticeRule(providerID, text string) (rule, bool) {
	starts := []int{0}
	if label := noticeErrorLabel.FindStringIndex(text); label != nil {
		starts = append(starts, label[1])
	}
	for _, rules := range [][]rule{providerRules[providerID], providerNeutralRules} {
		for _, r := range rules {
			for _, start := range starts {
				if loc := r.pattern.FindStringIndex(text[start:]); loc != nil && loc[0] == 0 {
					return r, true
				}
			}
		}
	}
	return rule{}, false
}
