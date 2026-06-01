package worktree

import "testing"

func TestSanitizeBranchSlug(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"main", "main"},
		{"feature/foo", "feature-foo"},
		{"feature/foo-bar", "feature-foo-bar"},
		{"release/v1.2.3", "release-v1.2.3"},
		{"user/jane/wip", "user-jane-wip"},
		{"-leading", "leading"},
		{"trailing-", "trailing"},
		{"slash/", "slash"},
		{"a//b", "a-b"},
		{"weird:name space", "weird-name-space"},
		{"!@#$%", ""},
		{"", ""},
		{".hidden", "hidden"},
		{"修复登录问题", ""},
	}
	for _, c := range cases {
		if got := SanitizeBranchSlug(c.in); got != c.want {
			t.Errorf("SanitizeBranchSlug(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestTaskWorktreePath_BranchSlugNesting(t *testing.T) {
	cfg := &Config{TasksBasePath: "/tmp/kandev/tasks"}

	flat, err := cfg.TaskWorktreePath("task-1_abc", "frontend", "")
	if err != nil {
		t.Fatalf("flat path: %v", err)
	}
	want := "/tmp/kandev/tasks/task-1_abc/frontend"
	if flat != want {
		t.Errorf("flat path = %q, want %q", flat, want)
	}

	sibling, err := cfg.TaskWorktreePath("task-1_abc", "frontend", "feature-foo")
	if err != nil {
		t.Fatalf("sibling path: %v", err)
	}
	want = "/tmp/kandev/tasks/task-1_abc/frontend-feature-foo"
	if sibling != want {
		t.Errorf("sibling path = %q, want %q", sibling, want)
	}
}
