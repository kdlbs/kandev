package coordinator

import (
	"strings"
	"testing"
)

func TestBuildWakeMessage_ListsWakesOldestFirstWithSanitisedTitles(t *testing.T) {
	long := strings.Repeat("é", 100)
	got := buildWakeMessage([]wakeLine{
		{Kind: WakeKindQuestion, Ref: "KAN-1", Title: "Line one\nline\ttwo \"quoted\""},
		{Kind: WakeKindStall, Ref: "task-uuid", Title: ""},
		{Kind: WakeKindStall, Ref: "KAN-3", Title: long},
	})
	want := "Unattended turn. No person started this turn or is watching it.\n" +
		"Events since your last turn (3):\n" +
		"- question on KAN-1 \"Line one line two 'quoted'\"\n" +
		"- stall on task-uuid \"\"\n" +
		"- stall on KAN-3 \"" + strings.Repeat("é", 80) + "\"\n" +
		"These were current when this turn started and may have changed since; read\n" +
		"current state before acting. Propose what should happen. Proposals wait for a\n" +
		"manager unless one has allowed automatic creation of tasks.\n"
	if got != want {
		t.Fatalf("message =\n%s\nwant\n%s", got, want)
	}
}
