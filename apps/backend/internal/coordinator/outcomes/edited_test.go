package outcomes

import (
	"reflect"
	"testing"
)

func TestEditedFields(t *testing.T) {
	cases := []struct {
		name        string
		spec, final string
		want        []string
	}{
		{"nil final means no edits", `{"title":"a"}`, "", []string{}},
		{"identical", `{"title":"a","step_id":"s"}`, `{"title":"a","step_id":"s"}`, []string{}},
		{"changed keys sorted", `{"title":"a","step_id":"s","description":"d"}`, `{"title":"b","step_id":"t","description":"d"}`, []string{"step_id", "title"}},
		{"key added by the edit", `{"title":"a"}`, `{"title":"a","repository_id":"r"}`, []string{"repository_id"}},
		{"key removed by the edit", `{"title":"a","repository_id":"r"}`, `{"title":"a"}`, []string{"repository_id"}},
		{"key order does not matter", `{"a":1,"b":2}`, `{"b":2,"a":1}`, []string{}},
		{"nested value compared structurally", `{"x":{"k":[1,2]}}`, `{"x":{"k":[1,2]}}`, []string{}},
		{"unparseable final reads as no edits", `{"title":"a"}`, `not json`, []string{}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := EditedFields([]byte(c.spec), []byte(c.final))
			if !reflect.DeepEqual(got, c.want) {
				t.Fatalf("EditedFields = %#v, want %#v", got, c.want)
			}
		})
	}
}

func TestEditedFieldsNeverReturnsValues(t *testing.T) {
	got := EditedFields([]byte(`{"title":"secret one"}`), []byte(`{"title":"secret two"}`))
	if !reflect.DeepEqual(got, []string{"title"}) {
		t.Fatalf("got %#v", got)
	}
}
