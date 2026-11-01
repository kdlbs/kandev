package main

import (
	"testing"
)

func TestAsyncQuestionAnswerLabelUsesCanonicalOptionLabel(t *testing.T) {
	listed := `{"questions":[{"question_id":"question-1","status":"answered","questions":[{"id":"db","prompt":"Choose a database","options":[{"option_id":"db-1","label":"PostgreSQL","description":"Relational"}]}],"outcome":{"answers":[{"question_id":"db","selected_options":["db-1"]}]}}]}`
	if got := asyncQuestionAnswerLabel(listed, "question-1"); got != "PostgreSQL" {
		t.Fatalf("asyncQuestionAnswerLabel() = %q, want PostgreSQL", got)
	}
}

func TestPendingAsyncQuestionIDsIgnoresTerminalRequests(t *testing.T) {
	listed := `{"questions":[{"question_id":"answered","status":"answered"},{"question_id":"pending","status":"pending"}]}`
	got := pendingAsyncQuestionIDs(listed)
	if len(got) != 1 || got[0] != "pending" {
		t.Fatalf("pendingAsyncQuestionIDs() = %#v, want [pending]", got)
	}
}

func TestEmitAsyncQuestionDeliveryResult(t *testing.T) {
	tests := []struct {
		name   string
		prompt string
		want   string
	}{
		{
			name: "operator answer",
			prompt: `ASYNC QUESTION ANSWERED
Question ID: question-1
Questions and options:
[{"id":"db","options":[{"option_id":"pg","label":"PostgreSQL"}]}]
Outcome:
{"answers":[{"question_id":"db","selected_options":["pg"]}]}`,
			want: "ASYNC_ANSWER_RECEIVED: PostgreSQL",
		},
		{
			name: "parent answer",
			prompt: `ASYNC CHILD QUESTION RESOLVED
Question ID: question-2
Questions and options:
[{"id":"db","options":[{"option_id":"sqlite","label":"SQLite"}]}]
Outcome:
{"answers":[{"question_id":"db","selected_options":["sqlite"]}]}`,
			want: "ASYNC_PARENT_ANSWER_RECEIVED: SQLite",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := asyncQuestionDeliveryResult(test.prompt); got != test.want {
				t.Fatalf("asyncQuestionDeliveryResult() = %q, want %q", got, test.want)
			}
		})
	}
}
