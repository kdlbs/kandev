package startupdiag

import (
	"strings"
	"testing"
)

func TestStartupDiagnosticMiseTrustProjection(t *testing.T) {
	tests := []struct {
		name   string
		line   string
		reason string
	}{
		{
			name:   "reported workspace path",
			line:   "mise ERROR Config files in /home/alice/work/acme/mise.toml are not trusted.",
			reason: ReasonMiseConfigUntrusted,
		},
		{
			name:   "path containing spaces",
			line:   "mise ERROR Config files in /home/alice/team workspace/mise.toml are not trusted.",
			reason: ReasonMiseConfigUntrusted,
		},
		{
			name:   "windows path",
			line:   `mise ERROR Config files in C:\Users\alice\work\mise.toml are not trusted.`,
			reason: ReasonMiseConfigUntrusted,
		},
		{
			name:   "ansi decorated line",
			line:   "\x1b[31mmise ERROR Config files in /tmp/mise.toml are not trusted.\x1b[0m",
			reason: ReasonMiseConfigUntrusted,
		},
		{
			name:   "carriage return suffix",
			line:   "mise ERROR Config files in /tmp/mise.toml are not trusted.\r",
			reason: ReasonMiseConfigUntrusted,
		},
		{
			name:   "unrelated prose",
			line:   "the workspace mise.toml is not trusted",
			reason: "",
		},
		{
			name:   "missing prefix",
			line:   "ERROR Config files in /tmp/mise.toml are not trusted.",
			reason: "",
		},
		{
			name:   "malformed prefix",
			line:   "mise  ERROR Config files in /tmp/mise.toml are not trusted.",
			reason: "",
		},
		{
			name:   "missing suffix",
			line:   "mise ERROR Config files in /tmp/mise.toml are not",
			reason: "",
		},
		{
			name:   "path without mise config",
			line:   "mise ERROR Config files in /tmp/other.toml are not trusted.",
			reason: "",
		},
		{
			name:   "empty path",
			line:   "mise ERROR Config files in  are not trusted.",
			reason: "",
		},
		{
			name:   "prefix and suffix share their single space",
			line:   "mise ERROR Config files in are not trusted.",
			reason: "",
		},
		{
			name:   "prefix only",
			line:   "mise ERROR Config files in ",
			reason: "",
		},
		{
			name:   "suffix only",
			line:   " are not trusted.",
			reason: "",
		},
		{
			name:   "overlapping prefix and suffix fragments",
			line:   "mise ERROR Config files in are not trust",
			reason: "",
		},
		{
			name:   "embedded newline",
			line:   "mise ERROR Config files in /tmp/mise.toml\nare not trusted.",
			reason: "",
		},
		{
			name:   "oversized line",
			line:   "mise ERROR Config files in /tmp/" + strings.Repeat("a", maxDiagnosticLineBytes) + "/mise.toml are not trusted.",
			reason: "",
		},
		{
			name:   "secret and url path stay unreturned",
			line:   "mise ERROR Config files in https://token@example.test/private/mise.toml are not trusted.",
			reason: ReasonMiseConfigUntrusted,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			marker, reason := ProjectLine(tt.line)
			if reason != tt.reason {
				t.Fatalf("ProjectLine(%q) reason = %q, want %q", tt.line, reason, tt.reason)
			}
			if tt.reason == "" {
				if marker != "" {
					t.Fatalf("ProjectLine(%q) marker = %q, want empty", tt.line, marker)
				}
				return
			}
			if marker != MiseConfigUntrustedDetail {
				t.Fatalf("ProjectLine(%q) marker = %q, want the fixed detail", tt.line, marker)
			}
			for _, sensitive := range []string{"/tmp", "token", "example.test", "mise.toml"} {
				if strings.Contains(marker, sensitive) {
					t.Fatalf("safe marker leaked %q: %q", sensitive, marker)
				}
			}
		})
	}
}
