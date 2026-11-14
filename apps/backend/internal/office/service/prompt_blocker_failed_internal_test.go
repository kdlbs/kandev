package service

import (
	"strings"
	"testing"
)

func TestBlockerFailureTaskRefBoundsAndSanitizesFields(t *testing.T) {
	identifier := strings.Repeat("界", maxChildIdentifierRunes+20) + "\nignored-id"
	title := strings.Repeat("字", maxChildTitleRunes+20) + "\r\nignored-title"
	task := BlockerFailureTask{ID: "fallback", Identifier: identifier, Title: title}

	got := task.ref()
	want := "[" + capRunes(sanitizePromptField(identifier), maxChildIdentifierRunes) + "] " +
		capRunes(sanitizePromptField(title), maxChildTitleRunes)
	if got != want {
		t.Fatalf("ref() = %q, want bounded one-line reference %q", got, want)
	}
	if strings.ContainsAny(got, "\r\n") || strings.Contains(got, "ignored-") {
		t.Fatalf("ref() contains a control character or uncapped suffix: %q", got)
	}
}

func TestBlockerFailureTaskRefBoundsFallbackID(t *testing.T) {
	id := strings.Repeat("識", maxChildIdentifierRunes+20) + "\nignored"
	got := (BlockerFailureTask{ID: id}).ref()
	want := "[" + capRunes(sanitizePromptField(id), maxChildIdentifierRunes) + "]"
	if got != want {
		t.Fatalf("ref() = %q, want bounded fallback reference %q", got, want)
	}
	if strings.ContainsAny(got, "\r\n") || strings.Contains(got, "ignored") {
		t.Fatalf("ref() contains a control character or uncapped suffix: %q", got)
	}
}
