package automation

import "testing"

// AC-42: automation name and trigger type are escaped by four disjoint rules
// before being interpolated into a warning message, so the rendered
// `<name>: <message>` line always contains no newline as `## Definitions`
// defines it (LF, CR, NEL, LS, PS) and is always valid UTF-8.

func TestEscapeWarningText_LFAndCR(t *testing.T) {
	got := escapeWarningText("Daily\nSync\r")
	want := `Daily\nSync\r`
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestEscapeWarningText_OtherC0AndDELAndNEL(t *testing.T) {
	got := escapeWarningText("a\x01b\x7fc\u0085d")
	want := `a\x01b\x7fc\x85d`
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestEscapeWarningText_LineSeparatorAndParagraphSeparator(t *testing.T) {
	got := escapeWarningText("Daily   Sync ")
	want := "Daily\\u2028  Sync\\u2029"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestEscapeWarningText_InvalidUTF8Byte(t *testing.T) {
	got := escapeWarningText("abc\xffdef")
	want := `abc\xffdef`
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestEscapeWarningText_OrdinaryTextUnchanged(t *testing.T) {
	got := escapeWarningText("plain name")
	if got != "plain name" {
		t.Errorf("got %q, want unchanged", got)
	}
}

func TestEscapeWarningText_EmptyString(t *testing.T) {
	if got := escapeWarningText(""); got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

// AC-20/21/42: buildWarningsList dedups per its scope key, orders by
// automation name asc / automations.id asc / message asc, and renders each
// surviving warning as "<escaped automation name>: <message>".

func TestBuildWarningsList_Empty(t *testing.T) {
	got := buildWarningsList(nil)
	if len(got) != 0 {
		t.Errorf("got %v, want empty", got)
	}
}

func TestBuildWarningsList_OrdersByNameThenIDThenMessage(t *testing.T) {
	warnings := []exportWarning{
		{AutomationName: "Zeta", AutomationID: "z1", DedupKey: "z1", Message: "unresolved workflow"},
		{AutomationName: "Alpha", AutomationID: "a2", DedupKey: "a2", Message: "unresolved workflow"},
		{AutomationName: "Alpha", AutomationID: "a1", DedupKey: "a1", Message: "unresolved agent profile"},
	}
	got := buildWarningsList(warnings)
	want := []string{
		"Alpha: unresolved agent profile",
		"Alpha: unresolved workflow",
		"Zeta: unresolved workflow",
	}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("index %d: got %q, want %q", i, got[i], want[i])
		}
	}
}

func TestBuildWarningsList_SameAutomationMultipleMessagesOrderedByText(t *testing.T) {
	warnings := []exportWarning{
		{AutomationName: "A", AutomationID: "a1", DedupKey: "a1", Message: "unresolved workflow"},
		{AutomationName: "A", AutomationID: "a1", DedupKey: "a1", Message: "unresolved agent profile"},
	}
	got := buildWarningsList(warnings)
	want := []string{"A: unresolved agent profile", "A: unresolved workflow"}
	if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestBuildWarningsList_DedupsWithinSameScopeKey(t *testing.T) {
	warnings := []exportWarning{
		{AutomationName: "A", AutomationID: "a1", DedupKey: "a1", Message: "unresolved workflow"},
		{AutomationName: "A", AutomationID: "a1", DedupKey: "a1", Message: "unresolved workflow"},
	}
	got := buildWarningsList(warnings)
	if len(got) != 1 {
		t.Errorf("got %v, want 1 entry after dedup", got)
	}
}

func TestBuildWarningsList_KeepsIdenticalMessagesAcrossDifferentScopeKeys(t *testing.T) {
	// Two different automations (or two different triggers on the same
	// automation) that produce byte-identical messages each keep their own
	// warning: dedup is scoped to DedupKey, never global.
	warnings := []exportWarning{
		{AutomationName: "Daily Sync", AutomationID: "a1", DedupKey: "a1", Message: "unresolved agent profile"},
		{AutomationName: "Daily Sync", AutomationID: "a2", DedupKey: "a2", Message: "unresolved agent profile"},
	}
	got := buildWarningsList(warnings)
	if len(got) != 2 {
		t.Errorf("got %v, want 2 entries (dedup must not cross scope keys)", got)
	}
}

func TestBuildWarningsList_EscapesAutomationNameOnRender(t *testing.T) {
	warnings := []exportWarning{
		{AutomationName: "Daily\nSync", AutomationID: "a1", DedupKey: "a1", Message: "unresolved workflow"},
	}
	got := buildWarningsList(warnings)
	want := `Daily\nSync: unresolved workflow`
	if len(got) != 1 || got[0] != want {
		t.Errorf("got %v, want [%q]", got, want)
	}
}
