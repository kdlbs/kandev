package main

import (
	"strings"
	"testing"
)

func TestReplaceOrAppendSection_append(t *testing.T) {
	body := "Existing PR description."
	section := "### Preview\nURL: https://example.com"
	got := replaceOrAppendSection(body, section)

	if !strings.Contains(got, sectionStart) {
		t.Error("expected sectionStart marker")
	}
	if !strings.Contains(got, sectionEnd) {
		t.Error("expected sectionEnd marker")
	}
	if !strings.HasPrefix(got, body) {
		t.Errorf("expected original body to be preserved at start; got: %q", got)
	}
}

func TestReplaceOrAppendSection_replace(t *testing.T) {
	old := "### Preview\nURL: https://old.example.com"
	body := "Existing body.\n\n" + sectionStart + "\n" + old + "\n" + sectionEnd

	newSection := "### Preview\nURL: https://new.example.com"
	got := replaceOrAppendSection(body, newSection)

	if strings.Contains(got, "old.example.com") {
		t.Error("expected old section to be replaced")
	}
	if !strings.Contains(got, "new.example.com") {
		t.Error("expected new section to be present")
	}
	// Only one start marker.
	if strings.Count(got, sectionStart) != 1 {
		t.Errorf("expected exactly one start marker, got %d", strings.Count(got, sectionStart))
	}
}

func TestReplaceOrAppendSection_emptyBody(t *testing.T) {
	section := "### Preview"
	got := replaceOrAppendSection("", section)
	if !strings.HasPrefix(got, sectionStart) {
		t.Errorf("expected section at start of empty body; got: %q", got)
	}
}

func TestStripSection(t *testing.T) {
	section := "### Preview\nURL: https://example.com"
	body := "My PR description.\n\n" + sectionStart + "\n" + section + "\n" + sectionEnd
	got := stripSection(body)

	if strings.Contains(got, sectionStart) || strings.Contains(got, sectionEnd) {
		t.Error("expected section markers to be removed")
	}
	if !strings.Contains(got, "My PR description.") {
		t.Error("expected original body to be preserved")
	}
	if strings.Contains(got, "https://example.com") {
		t.Error("expected section content to be removed")
	}
}

func TestStripSection_noMarker(t *testing.T) {
	body := "No preview section here."
	got := stripSection(body)
	if got != body {
		t.Errorf("expected no-op; got %q", got)
	}
}

func TestStripSection_otherBotAppended(t *testing.T) {
	// Simulates another bot having appended its own section after ours.
	section := "### Preview"
	otherBot := "<!-- other-bot-start -->\nOther bot content\n<!-- other-bot-end -->"
	body := "PR body.\n\n" + sectionStart + "\n" + section + "\n" + sectionEnd + "\n\n" + otherBot
	got := stripSection(body)

	if strings.Contains(got, sectionStart) {
		t.Error("expected our section to be removed")
	}
	if !strings.Contains(got, "other-bot-start") {
		t.Error("expected other bot section to be preserved")
	}
}
