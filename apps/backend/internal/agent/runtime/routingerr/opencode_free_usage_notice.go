package routingerr

// openCodeFreeUsageRules are the OpenCode Free usage-limit signatures.
var openCodeFreeUsageRules = map[string]bool{
	"opencode.stderr.free_usage_unlabelled.v1": true,
	"opencode.stderr.free_usage_prefixed.v1":   true,
}

// HasOpenCodeFreeUsageNotice reports whether a sanitized OpenCode diagnostic
// carries a Free usage-limit line under the same signatures and fence handling
// as classification. A consumer uses it when an earlier OpenCode rule, such as
// a period usage limit, won the classification of the same diagnostic.
func HasOpenCodeFreeUsageNotice(excerpt string) bool {
	for _, r := range providerRules["opencode-acp"] {
		if !openCodeFreeUsageRules[r.id] {
			continue
		}
		text := excerpt
		if r.ignoreMarkdownFences {
			text = withoutMarkdownFencedCode(text)
		}
		if text != "" && r.pattern.MatchString(text) {
			return true
		}
	}
	return false
}
