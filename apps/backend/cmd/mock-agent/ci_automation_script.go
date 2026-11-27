package main

import "strings"

// CI automation wraps custom instructions separately from visible PR feedback.
// Only an explicit script block on an auto-fix prompt selects this mock scenario.
func ciAutomationScript(prompt, visible string) string {
	if !strings.HasPrefix(visible, "@ci-auto-fix\n") {
		return ""
	}
	for _, block := range kandevSystemRegex.FindAllString(prompt, -1) {
		content := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(block, "<kandev-system>"), "</kandev-system>"))
		if isScriptMode(content) {
			return content
		}
	}
	return ""
}
