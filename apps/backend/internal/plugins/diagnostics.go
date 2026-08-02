package plugins

import (
	"os"
	"regexp"
	"strings"
	"unicode/utf8"
)

// maxPluginErrorBytes bounds the diagnostic persisted into a plugin record.
// Failure messages can originate in subprocess/transport errors, so keeping
// them bounded prevents an unexpectedly verbose error from bloating the
// record or the settings API response.
const maxPluginErrorBytes = 2048

var (
	pluginBearerTokenPattern   = regexp.MustCompile(`(?i)\bBearer\s+[A-Za-z0-9._~+/=-]+`)
	pluginLabeledSecretPattern = regexp.MustCompile(`(?i)\b(password|passwd|passphrase|secret|token|api[_ -]?(?:key|token|secret)|access[_ -]?token|refresh[_ -]?token|pat)\b(\s*[:=]\s*)(?:"[^"]*"|'[^']*'|[^\s,;}]+)`)
	pluginPATPattern           = regexp.MustCompile(`\b(?:github_pat_[A-Za-z0-9_]+|gh[pousr]_[A-Za-z0-9]+|glpat-[A-Za-z0-9_-]+)\b`)
	pluginHomePathPattern      = regexp.MustCompile(`(/Users/[^/\s]+|/home/[^/\s]+|/root)(/|$)`)
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
	message = redactPluginError(message)
	if len([]byte(message)) <= maxPluginErrorBytes {
		return message
	}

	const marker = "…"
	budget := maxPluginErrorBytes - len([]byte(marker))
	runes := []rune(message)
	for len([]byte(string(runes))) > budget {
		runes = runes[:len(runes)-1]
	}
	// The loop above is rune-safe, but keep the invariant explicit if the
	// limit is ever changed below the marker's byte width.
	result := string(runes) + marker
	if len([]byte(result)) > maxPluginErrorBytes {
		result = string([]byte(result)[:maxPluginErrorBytes])
		for !utf8.ValidString(result) {
			result = result[:len(result)-1]
		}
	}
	return result
}

func redactPluginError(message string) string {
	if home, err := os.UserHomeDir(); err == nil && home != "" && home != "/" {
		homePattern := regexp.MustCompile(regexp.QuoteMeta(home) + `([/\\]|$)`)
		message = homePattern.ReplaceAllString(message, "~$1")
	}
	message = pluginHomePathPattern.ReplaceAllString(message, "~$2")
	message = pluginBearerTokenPattern.ReplaceAllString(message, "Bearer [REDACTED]")
	message = pluginLabeledSecretPattern.ReplaceAllString(message, "${1}${2}[REDACTED]")
	message = pluginPATPattern.ReplaceAllString(message, "[REDACTED]")
	return message
}
