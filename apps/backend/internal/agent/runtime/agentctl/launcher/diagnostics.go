package launcher

import (
	"regexp"
	"strings"
	"unicode"
)

const maxDiagnosticTailBytes = 8 * 1024

var diagnosticSecretPattern = regexp.MustCompile(`(?i)(authorization\s*:\s*bearer\s+|bearer\s+|(?:api[_-]?key|token|secret|credential|password)\s*[:=]\s*)[^\s,;]+`)

func (l *Launcher) captureDiagnostic(line string) {
	line = sanitizeDiagnostic(line)
	if line == "" {
		return
	}
	l.diagnosticMu.Lock()
	l.diagnosticTail += line + "\n"
	if len(l.diagnosticTail) > maxDiagnosticTailBytes {
		l.diagnosticTail = l.diagnosticTail[len(l.diagnosticTail)-maxDiagnosticTailBytes:]
		l.diagnosticTail = strings.ToValidUTF8(l.diagnosticTail, "")
	}
	l.diagnosticMu.Unlock()
}

func (l *Launcher) diagnostic() string {
	l.diagnosticMu.Lock()
	defer l.diagnosticMu.Unlock()
	return l.diagnosticTail
}

func sanitizeDiagnostic(line string) string {
	line = stripANSI(line)
	line = diagnosticSecretPattern.ReplaceAllString(line, "$1[redacted]")
	line = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) && r != '\t' {
			return ' '
		}
		return r
	}, line)
	return strings.TrimSpace(line)
}
