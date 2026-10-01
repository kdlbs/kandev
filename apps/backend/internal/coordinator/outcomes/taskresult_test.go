package outcomes

import "testing"

func TestTaskResult(t *testing.T) {
	cases := []struct {
		name string
		in   TaskFacts
		want string
	}{
		{"open", TaskFacts{}, ResultOpen},
		{"merged beats everything", TaskFacts{PRMerged: true, StepCompletes: true, Archived: true, LatestSessionFailed: true}, ResultMerged},
		{"done", TaskFacts{StepCompletes: true}, ResultDone},
		{"done beats failed", TaskFacts{StepCompletes: true, LatestSessionFailed: true}, ResultDone},
		{"done beats dropped", TaskFacts{StepCompletes: true, Archived: true}, ResultDone},
		{"failed", TaskFacts{LatestSessionFailed: true}, ResultFailed},
		{"failed beats dropped", TaskFacts{LatestSessionFailed: true, Archived: true}, ResultFailed},
		{"dropped", TaskFacts{Archived: true}, ResultDropped},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := TaskResult(c.in); got != c.want {
				t.Fatalf("TaskResult(%+v) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

func TestIsFinalResult(t *testing.T) {
	for result, want := range map[string]bool{ResultMerged: true, ResultDropped: true, ResultDone: false, ResultFailed: false, ResultOpen: false} {
		if got := IsFinalResult(result); got != want {
			t.Errorf("IsFinalResult(%q) = %v, want %v", result, got, want)
		}
	}
}

func TestReopened(t *testing.T) {
	cases := []struct {
		stored, next string
		want         bool
	}{
		{ResultDone, ResultOpen, true},
		{ResultDone, ResultFailed, true},
		{ResultDone, ResultMerged, false},
		{ResultDone, ResultDropped, false},
		{ResultDone, ResultDone, false},
		{ResultOpen, ResultFailed, false},
		{ResultFailed, ResultOpen, false},
		{"", ResultOpen, false},
	}
	for _, c := range cases {
		if got := Reopened(c.stored, c.next); got != c.want {
			t.Errorf("Reopened(%q,%q) = %v, want %v", c.stored, c.next, got, c.want)
		}
	}
}
