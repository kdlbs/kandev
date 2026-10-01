package dream

import (
	"errors"
	"strings"
	"testing"
)

func TestParseAnswerAccepts(t *testing.T) {
	good := `{"items":[{"kind":"note_add","text":"x","target_id":"","cited_turn_ids":["a","b"]}],"considered":["c"]}`
	for name, raw := range map[string]string{
		"bare":       good,
		"fenced":     "```json\n" + good + "\n```",
		"empty":      `{"items":[],"considered":[]}`,
		"no-members": `{}`,
	} {
		t.Run(name, func(t *testing.T) {
			a, err := ParseAnswer(raw)
			if err != nil {
				t.Fatalf("err = %v", err)
			}
			if a.Items == nil || a.Considered == nil {
				t.Fatal("nil slices must be empty")
			}
		})
	}
}

func TestParseAnswerRefuses(t *testing.T) {
	items := func(n int) string {
		it := `{"kind":"note_add","text":"x","cited_turn_ids":["a","b"]}`
		return `{"items":[` + strings.TrimSuffix(strings.Repeat(it+",", n), ",") + `]}`
	}
	cons := func(n int) string {
		return `{"considered":[` + strings.TrimSuffix(strings.Repeat(`"c",`, n), ",") + `]}`
	}
	for name, raw := range map[string]string{
		"prose":         "Here you go: " + `{"items":[]}`,
		"trailing":      `{"items":[]} extra`,
		"two-docs":      `{"items":[]}{"items":[]}`,
		"top-key":       `{"items":[],"other":1}`,
		"item-key":      `{"items":[{"kind":"note_add","text":"x","bogus":1}]}`,
		"bad-kind":      `{"items":[{"kind":"apply","text":"x"}]}`,
		"11-items":      items(11),
		"21-considered": cons(21),
		"not-json":      `nope`,
		"empty":         ``,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseAnswer(raw); !errors.Is(err, ErrBadOutput) {
				t.Fatalf("err = %v, want ErrBadOutput", err)
			}
		})
	}
	if _, err := ParseAnswer(items(10)); err != nil {
		t.Fatalf("10 items: %v", err)
	}
	if _, err := ParseAnswer(cons(20)); err != nil {
		t.Fatalf("20 considered: %v", err)
	}
}
