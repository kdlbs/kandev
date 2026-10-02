package stub

import (
	"errors"
	"reflect"
	"testing"
)

func TestParse(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []string
		bad  bool
	}{
		{"empty array", "[]", []string{}, false},
		{"bare", `[{"kind":"move","target_task_id":"t1"}]`, []string{"move|t1"}, false},
		{"fenced", "```json\n[{\"kind\":\"resume\",\"target_task_id\":\"t2\"}]\n```", []string{"resume|t2"}, false},
		{"fenced no lang", "```\n[]\n```", []string{}, false},
		{"create task key normalised", `[{"kind":"create_task","workflow_id":"w","title":"  Fix  THE bug "}]`, []string{"w|fix the bug"}, false},
		{"duplicates count once and sort", `[{"kind":"move","target_task_id":"b"},{"kind":"message","target_task_id":"a"},{"kind":"move","target_task_id":"b"}]`, []string{"message|a", "move|b"}, false},
		{"text outside", `Here: [] done`, nil, true},
		{"text after fence", "```\n[]\n``` thanks", nil, true},
		{"two arrays", `[] []`, nil, true},
		{"unknown kind", `[{"kind":"improvement","target_task_id":"t"}]`, nil, true},
		{"missing target", `[{"kind":"move"}]`, nil, true},
		{"create task missing title", `[{"kind":"create_task","workflow_id":"w"}]`, nil, true},
		{"create task missing workflow", `[{"kind":"create_task","title":"x"}]`, nil, true},
		{"not json", `[nope`, nil, true},
		{"object not array", `{"kind":"move"}`, nil, true},
		{"two fences", "```\n[]\n```\n```\n[]\n```", nil, true},
		{"fence other language", "```python\n[]\n```", nil, true},
		{"unknown field", `[{"kind":"move","target_task_id":"t","extra":1}]`, nil, true},
		{"empty text", "", nil, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Parse(tc.in)
			if tc.bad {
				if !errors.Is(err, ErrUnreadable) {
					t.Fatalf("err = %v, want ErrUnreadable", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error %v", err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
	}
}
