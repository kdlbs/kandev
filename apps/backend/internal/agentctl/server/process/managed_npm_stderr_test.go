package process

import (
	"io"
	"strings"
	"testing"
)

func TestSafeManagedNpmStderrLineKeepsOnlyCanonicalReleaseAgeMarker(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{
			name: "RFC3339 date",
			raw:  "npm error notarget No matching version found for opencode-ai@1.18.18 with a date before 2026-09-24T11:00:00.000Z.",
			want: "npm error notarget No matching version found for opencode-ai@1.18.18 with a date before <release-date>",
		},
		{
			name: "npm 11.16 issue date",
			raw:  "npm error notarget No matching version found for @agentclientprotocol/claude-agent-acp@0.81.0 with a date before 9/22/2026, 12:28:47 PM.",
			want: "npm error notarget No matching version found for @agentclientprotocol/claude-agent-acp@0.81.0 with a date before <release-date>",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, keep := safeManagedNpmStderrLine(tt.raw)
			if !keep || got != tt.want {
				t.Fatalf("safeManagedNpmStderrLine() = (%q, %v), want (%q, true)", got, keep, tt.want)
			}
			if strings.Contains(got, "9/22/2026") || strings.Contains(got, "12:28:47 PM") {
				t.Fatalf("sanitized diagnostic leaked the raw date: %q", got)
			}
		})
	}
}

func TestSafeManagedNpmStderrLineRejectsMalformedReleaseAgeDiagnostics(t *testing.T) {
	for _, raw := range []string{
		"npm error notarget No matching version found for opencode-ai@1.18.18 with a date before yesterday.",
		"npm error notarget No matching version found for opencode-ai@1.18.18 with a date before 2026-02-30T11:00:00Z.",
		"npm error notarget No matching version found for opencode-ai@1.18.18 with a date before 9/31/2026, 12:28:47 PM.",
		"npm error notarget No matching version found for opencode-ai@1.18.18 with a date before 9/22/2026, 12:88:47 PM.",
		"npm error notarget No matching version found for ../opencode@1.18.18 with a date before 2026-09-24T11:00:00Z.",
	} {
		if got, keep := safeManagedNpmStderrLine(raw); keep || got != "" {
			t.Fatalf("safeManagedNpmStderrLine(%q) = (%q, %v), want empty and false", raw, got, keep)
		}
	}
}

func TestSafeManagedNpmStderrLineProjectsTransientStartupCodes(t *testing.T) {
	for _, code := range []string{"ECONNRESET", "ECONNREFUSED", "ETIMEDOUT", "EAI_AGAIN", "E502", "E503", "E504", "EBUSY", "ENOTEMPTY", "EINTEGRITY"} {
		t.Run(code, func(t *testing.T) {
			got, keep := safeManagedNpmStderrLine("npm error code " + code)
			want := "npm error code " + code
			if !keep || got != want {
				t.Fatalf("safeManagedNpmStderrLine() = (%q, %v), want (%q, true)", got, keep, want)
			}
		})
	}
}

func TestSafeManagedNpmStderrLineMarksUnclassifiedCanonicalCodes(t *testing.T) {
	for _, code := range []string{"EUSAGE", "EBADPLATFORM"} {
		t.Run(code, func(t *testing.T) {
			got, keep := safeManagedNpmStderrLine("npm error code " + code)
			if !keep || got != npmUnknownCodeMarker {
				t.Fatalf("safeManagedNpmStderrLine() = (%q, %v), want bounded unknown-code marker", got, keep)
			}
			if strings.Contains(got, code) {
				t.Fatalf("safe diagnostic exposed an unclassified npm code: %q", got)
			}
		})
	}
}

func TestReadStderrProjectsUnclassifiedNpmCodesBeforeProviderSanitizer(t *testing.T) {
	m := &Manager{
		stderr: io.NopCloser(strings.NewReader("npm error code EBADPLATFORM\n")),
		stderrSanitizer: stderrLineSanitizerFunc(func(string) (string, bool) {
			return "provider failure", true
		}),
		logger: newTestLogger(t),
	}
	m.wg.Add(1)
	m.readStderr(make(chan stderrReadResult, 1))

	if got := m.GetRecentStderr(); len(got) != 1 || got[0] != npmUnknownCodeMarker {
		t.Fatalf("retained stderr = %#v, want one bounded unknown npm-code marker", got)
	}
}
