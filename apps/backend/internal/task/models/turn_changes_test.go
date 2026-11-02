package models

import "testing"

func TestTurnChangeSetPartialProjectsCompleteness(t *testing.T) {
	for _, test := range []struct {
		name            string
		complete        bool
		summaryComplete bool
		contentComplete bool
		partial         bool
	}{
		{name: "complete", complete: true, summaryComplete: true, contentComplete: true},
		{name: "interval partial", summaryComplete: true, contentComplete: true, partial: true},
		{name: "summary partial", complete: true, contentComplete: true, partial: true},
		{name: "content partial", complete: true, summaryComplete: true, partial: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			changeSet := TurnChangeSet{
				Complete: test.complete, SummaryComplete: test.summaryComplete, ContentComplete: test.contentComplete,
			}
			if got := changeSet.Partial(); got != test.partial {
				t.Fatalf("Partial() = %t, want %t", got, test.partial)
			}
		})
	}
}
