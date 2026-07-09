package skill

import "testing"

func TestCleanRelativeSkillFilePath(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		want      string
		wantAllow bool
	}{
		{name: "reject empty", input: ""},
		{name: "reject whitespace", input: "  "},
		{name: "reject current dir", input: "."},
		{name: "reject cleaned current dir", input: "a/.."},
		{name: "reject backslash", input: `a\b`},
		{name: "reject parent", input: ".."},
		{name: "reject nested parent", input: "a/../.."},
		{name: "reject sibling", input: "../sibling"},
		{name: "reject absolute", input: "/absolute"},
		{name: "reject skill md", input: "SKILL.md"},
		{name: "allow reference", input: "references/tasks.md", want: "references/tasks.md", wantAllow: true},
		{name: "allow nested", input: "deep/nested/file.md", want: "deep/nested/file.md", wantAllow: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := cleanRelativeSkillFilePath(tt.input)
			if ok != tt.wantAllow || got != tt.want {
				t.Fatalf("cleanRelativeSkillFilePath(%q) = %q, %v; want %q, %v", tt.input, got, ok, tt.want, tt.wantAllow)
			}
		})
	}
}
