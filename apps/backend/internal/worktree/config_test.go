package worktree

import "testing"

func TestTaskDirSuffix(t *testing.T) {
	const id = "61ccfd2c-1121-4226-99ab-8d9a60a57e6e"

	got := TaskDirSuffix(id)
	if got == "" {
		t.Fatal("TaskDirSuffix returned empty string")
	}
	if len(got) != taskDirSuffixLen {
		t.Fatalf("TaskDirSuffix(%q) length = %d, want %d", id, len(got), taskDirSuffixLen)
	}
	for _, r := range got {
		if !isASCIIAlphaNum(r) {
			t.Errorf("TaskDirSuffix(%q) = %q, contains non-alphanumeric %q", id, got, r)
		}
		if r >= 'A' && r <= 'Z' {
			t.Errorf("TaskDirSuffix(%q) = %q, contains uppercase %q", id, got, r)
		}
	}

	if again := TaskDirSuffix(id); again != got {
		t.Errorf("TaskDirSuffix is not stable: first %q, second %q", got, again)
	}

	other := TaskDirSuffix("a2ac3b48-0000-0000-0000-000000000000")
	if other == got {
		t.Errorf("TaskDirSuffix collided for two different IDs: both %q", got)
	}

	if TaskDirSuffix("") != "" {
		t.Errorf("TaskDirSuffix(\"\") = %q, want empty", TaskDirSuffix(""))
	}
}

func TestSemanticWorktreeNameTaskUnique(t *testing.T) {
	const title = "We need to improve alerting"
	a := SemanticWorktreeName(title, TaskDirSuffix("61ccfd2c-1121-4226-99ab-8d9a60a57e6e"))
	b := SemanticWorktreeName(title, TaskDirSuffix("a2ac3b48-1111-2222-3333-444455556666"))
	if a == b {
		t.Errorf("two tasks with the same title produced the same task-root name %q", a)
	}
}

func TestSanitizeRepoDirName(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"widget-config", "widget-config"},
		{"acme/widget-config", "acme-widget-config"},
		{"acme/widget", "acme-widget"},
		{"owner\\repo", "owner-repo"},
		{"weird:name space", "weird-name-space"},
		{"with..dots", "with..dots"},
		{"trailing/", "trailing"},
		{"/leading", "leading"},
		{"a//b", "a-b"},
		{"-a-b-", "a-b"},
		{".hidden", "hidden"},
		{"!@#$%", ""},
		{"", ""},
		{"under_score.dot-dash", "under_score.dot-dash"},
		{"修复登录问题", ""},
		{"acme/修复", "acme"},
		{"🐛/repo", "repo"},
		{"owner/répó", "owner-r-p"},
	}
	for _, c := range cases {
		if got := SanitizeRepoDirName(c.in); got != c.want {
			t.Errorf("SanitizeRepoDirName(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
