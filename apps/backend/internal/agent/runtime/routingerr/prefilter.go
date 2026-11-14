package routingerr

import (
	"fmt"
	"strings"
	"time"
)

// keywordDiagnosticText accepts one bounded diagnostic envelope. It rejects
// quoted examples and negated notices even on a terminal error channel.
func keywordDiagnosticText(text string) (string, bool) {
	text = strings.TrimSpace(text)
	if text == "" || len(text) > 1024 || strings.ContainsAny(text, "\n\r`\"<>") {
		return "", false
	}
	lower := strings.ToLower(text)
	words := strings.Fields(normalizeKeywords(text))
	for i, word := range words {
		if word == "without" || ((word == "not" || word == "no") && (i+1 >= len(words) || (words[i+1] != "found" && words[i+1] != "endpoints"))) {
			return "", false
		}
	}
	for _, negation := range []string{"not an error", "no error", "no longer", "not blocked", "not suspended", "not exceeded", "not reached", "example", "quoted"} {
		if strings.Contains(lower, negation) {
			return "", false
		}
	}
	for _, prefix := range []string{"api error:", "internal error:", "ai_apicallerror:", "error:", "streaming response failed:"} {
		if strings.HasPrefix(lower, prefix) {
			return strings.TrimSpace(text[len(prefix):]), true
		}
	}
	return "", false
}

func classifyKeywordContext(in Input, excerpt string) *Error {
	context := in.KeywordContext
	if !context.Enabled || !context.Diagnostic {
		return nil
	}
	diagnostic := strings.TrimSpace(in.Stderr + "\n" + in.Stdout)
	boundedText := ""
	if len(diagnostic) <= 1024 {
		boundedText = SanitizeFullUnbounded(diagnostic)
	}
	code, group, rule := keywordStructuralCode(in, boundedText)
	confidence := ConfHigh
	if code == "" {
		text, envelope := keywordDiagnosticText(boundedText)
		if !envelope {
			return nil
		}
		var count int
		var tied bool
		group, count, tied = matchKeywordGroup(text)
		if count == 0 {
			return nil
		}
		if count < 2 || tied {
			return &Error{Code: CodeAgentRuntime, Confidence: ConfLow, Phase: in.Phase,
				ClassifierRule: "keyword_context.ambiguous.v1", RawExcerpt: excerpt}
		}
		code = keywordCode(group, normalizeKeywords(text), context)
		rule = "keyword_context." + string(group) + ".v1"
	}
	result := &Error{Code: code, Confidence: confidence, Phase: in.Phase,
		ClassifierRule: rule, ExitCode: in.ExitCode, ResetHint: in.ResetHint, RawExcerpt: excerpt}
	result.Keyword = keywordDecision(group, in)
	applyKeywordReset(in, result)
	return result
}

func keywordStructuralCode(in Input, text string) (Code, keywordGroup, string) {
	text = normalizeKeywords(text)
	code, group := Code(""), keywordGroup("")
	status := in.HTTPStatus
	if status == 0 {
		status = in.KeywordContext.HTTPStatus
	}
	switch status {
	case 401, 403:
		code, group = httpStatusToCode(status), groupAccount
	case 404:
		if keywordTermMatches(strings.Fields(text), []string{"model"}) || strings.Contains(text, "endpoint") {
			code, group = CodeModelUnavailable, groupGone
		}
	case 408, 504:
		code, group = CodeProviderUnavailable, groupCapacity
	case 429:
		code, group = CodeRateLimited, groupLimit
	case 500, 502, 503, 529:
		code, group = httpStatusToCode(status), groupCapacity
	case 400:
		if rule := keywordRequestRule(text); rule != "" {
			return CodeAgentRuntime, groupRequest, rule
		}
	}
	if code == "" {
		return "", "", ""
	}
	return code, group, fmt.Sprintf("keyword_context.http.%d.%s.v1", status, group)
}

func keywordRequestRule(text string) string {
	if strings.Contains(text, "context") || strings.Contains(text, "token") || strings.Contains(text, "length") {
		return "keyword_context.http.400.context.v1"
	}
	if strings.Contains(text, "tool") || strings.Contains(text, "schema") || strings.Contains(text, "reasoning content") {
		return "keyword_context.http.400.request.v1"
	}
	return ""
}

func keywordCode(group keywordGroup, text string, context KeywordContext) Code {
	switch group {
	case groupLimit:
		return CodeQuotaLimited
	case groupCapacity:
		return CodeProviderUnavailable
	case groupGone:
		if context.ModelID != "" || strings.Contains(text, "model") || strings.Contains(text, "endpoint") {
			return CodeModelUnavailable
		}
	case groupAccount:
		// Keyword evidence alone never creates an owner-release account hold.
		return CodeAuthRequired
	case groupRequest:
		return CodeAgentRuntime
	}
	return CodeAgentRuntime
}

func applyKeywordReset(in Input, result *Error) {
	if result.Code != CodeQuotaLimited && result.Code != CodeRateLimited {
		return
	}
	now := in.OccurredAt
	if now.IsZero() {
		now = time.Now()
	}
	if result.ResetHint == nil {
		result.ResetHint = parseResetHintAt(in.Stderr+"\n"+in.Stdout, now)
	}
	if result.ResetHint == nil && in.KeywordContext.FreeDaily {
		utc := now.UTC()
		next := time.Date(utc.Year(), utc.Month(), utc.Day()+1, 0, 0, 0, 0, time.UTC)
		result.ResetHint = &next
	}
}
