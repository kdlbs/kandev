package routingerr

import (
	"strings"
	"unicode"
)

type keywordGroup string

const (
	groupLimit    keywordGroup = "limit"
	groupCapacity keywordGroup = "capacity"
	groupGone     keywordGroup = "gone"
	groupAccount  keywordGroup = "account"
	groupRequest  keywordGroup = "request"
)

// Entries are semantic evidence terms, not provider-specific message copies.
// A multiword entry counts once; repeating a term never increases confidence.
var keywordGroups = []struct {
	group keywordGroup
	terms []string
}{
	{groupAccount, []string{"blocked", "forbidden", "unauthorized", "access has been", "suspended", "invalid api key", "requires global", "trains on"}},
	{groupRequest, []string{"context length", "maximum context", "tokens", "invalid tools", "invalid request", "reasoning content", "schema"}},
	{groupGone, []string{"not found", "no endpoints", "deprecated", "retired", "model is unavailable", "unknown model"}},
	{groupLimit, []string{"limit", "quota", "exceeded", "reached", "usage", "credits", "balance", "insufficient", "subscribe", "upgrade", "plan", "renewal", "reset", "try again later"}},
	{groupCapacity, []string{"capacity", "overloaded", "busy", "unavailable", "endpoint is unavailable", "upstream", "idle", "timeout", "timed out"}},
}

func normalizeKeywords(text string) string {
	return strings.Join(strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	}), " ")
}

func keywordStem(word string) string {
	for _, suffix := range []string{"ing", "ed", "es", "s"} {
		if len(word) > len(suffix)+3 && strings.HasSuffix(word, suffix) {
			word = strings.TrimSuffix(word, suffix)
			break
		}
	}
	return strings.TrimSuffix(word, "e")
}

func keywordTermMatches(words, term []string) bool {
	for start := 0; start+len(term) <= len(words); start++ {
		matched := true
		for i, word := range term {
			if keywordStem(words[start+i]) != keywordStem(word) {
				matched = false
				break
			}
		}
		if matched {
			return true
		}
	}
	return false
}

func matchKeywordGroup(text string) (keywordGroup, int, bool) {
	words := strings.Fields(normalizeKeywords(text))
	var best keywordGroup
	bestCount := 0
	tied := false
	accountHit, goneHit := false, false
	for _, group := range keywordGroups {
		count := 0
		for _, term := range group.terms {
			if keywordTermMatches(words, strings.Fields(term)) {
				count++
			}
		}
		accountHit = accountHit || (group.group == groupAccount && count > 0)
		goneHit = goneHit || (group.group == groupGone && count > 0)
		if count > bestCount {
			best, bestCount, tied = group.group, count, false
		} else if count > 0 && count == bestCount {
			tied = true
		}
	}
	// Generic recovery terms cannot resolve conflicting account or removal
	// evidence. Such evidence alone also cannot authorize a stop or hold.
	if (accountHit && best != groupAccount) || (goneHit && best != groupGone) {
		tied = true
	}
	return best, bestCount, tied
}
