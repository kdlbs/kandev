package worktree

import "testing"

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
	}
	for _, c := range cases {
		if got := SanitizeRepoDirName(c.in); got != c.want {
			t.Errorf("SanitizeRepoDirName(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
