package plugins

import (
	"os"
	"regexp"
	"strings"
	"sync"
	"unicode/utf8"
)

// maxPluginErrorBytes bounds the diagnostic persisted into a plugin record.
// Failure messages can originate in subprocess/transport errors, so keeping
// them bounded prevents an unexpectedly verbose error from bloating the
// record or the settings API response.
const maxPluginErrorBytes = 2048

var (
	pluginBearerTokenPattern   = regexp.MustCompile(`(?i)\bBearer\s+[A-Za-z0-9._~+/=-]+`)
	pluginLabeledSecretPattern = regexp.MustCompile(`(?i)\b((?:[A-Za-z0-9]+[_-])*(?:password|passwd|passphrase|secret|token|api[_ -]?(?:key|token|secret)|access[_ -]?token|refresh[_ -]?token|pat))\b(\s*[:=]\s*)(?:"[^"]*"|'[^']*'|[^\s,;}]+)`)
	pluginPATPattern           = regexp.MustCompile(`\b(?:github_pat_[A-Za-z0-9_]+|gh[pousr]_[A-Za-z0-9]+|glpat-[A-Za-z0-9_-]+)\b`)
	pluginHomePathPattern      = regexp.MustCompile(`(/Users/[^/\s]+|/home/[^/\s]+|/root)(/|$)`)
	pluginWindowsHomePattern   = regexp.MustCompile(`(?i)([A-Za-z]:[/\\]Users[/\\][^/\\\s]+)([/\\]|$)`)
	pluginURLCredentialPattern = regexp.MustCompile(`(?i)(https?|ftp)://[^:@/\s]+:[^@/\s]+@`)

	pluginHomePatternOnce   sync.Once
	cachedPluginHomePattern *regexp.Regexp
)

// normalizePluginError turns a runtime failure into a compact, single-line
// diagnostic suitable for persistence and display. Runtime errors can include
// arbitrary go-plugin subprocess output, so credential-like values and home
// paths are removed before the bounded message is stored.
func normalizePluginError(err error) string {
	if err == nil {
		return ""
	}
	message := strings.Join(strings.Fields(err.Error()), " ")
	message = strings.ToValidUTF8(message, "�")
	message = redactPluginError(message)
	if len([]byte(message)) <= maxPluginErrorBytes {
		return message
	}

	return truncatePluginError(message)
}

func truncatePluginError(message string) string {
	const marker = "…"
	content := []byte(message)
	cut := maxPluginErrorBytes - len([]byte(marker))
	for cut > 0 && cut < len(content) && !utf8.RuneStart(content[cut]) {
		cut--
	}
	return string(content[:cut]) + marker
}

func redactPluginError(message string) string {
	pluginHomePatternOnce.Do(func() {
		if home, err := os.UserHomeDir(); err == nil && home != "" && home != "/" {
			cachedPluginHomePattern = regexp.MustCompile(regexp.QuoteMeta(home) + `([/\\]|$)`)
		}
	})
	if cachedPluginHomePattern != nil {
		message = cachedPluginHomePattern.ReplaceAllString(message, "~$1")
	}
	message = pluginHomePathPattern.ReplaceAllString(message, "~$2")
	message = pluginWindowsHomePattern.ReplaceAllString(message, "~$2")
	message = pluginURLCredentialPattern.ReplaceAllString(message, "${1}://[REDACTED]@")
	message = pluginBearerTokenPattern.ReplaceAllString(message, "Bearer [REDACTED]")
	message = pluginLabeledSecretPattern.ReplaceAllString(message, "${1}${2}[REDACTED]")
	message = pluginPATPattern.ReplaceAllString(message, "[REDACTED]")
	return message
}
