package lifecycle

import (
	"strings"
	"testing"
)

func TestParentDir(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"/home/zeval/.claude/credentials.json", "/home/zeval/.claude"},
		{"/etc/hosts", "/etc"},
		{"creds.json", ""},
		{"/foo", ""},
		{"", ""},
		{"/", ""},
		{"/a/b/c/d", "/a/b/c"},
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			if got := parentDir(c.in); got != c.want {
				t.Errorf("parentDir(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

func TestBuildSSHEnvPrefix(t *testing.T) {
	t.Run("empty map returns empty string", func(t *testing.T) {
		if got := buildSSHEnvPrefix(nil); got != "" {
			t.Errorf("buildSSHEnvPrefix(nil) = %q, want \"\"", got)
		}
		if got := buildSSHEnvPrefix(map[string]string{}); got != "" {
			t.Errorf("buildSSHEnvPrefix(empty) = %q, want \"\"", got)
		}
	})

	t.Run("single env var is shell-quoted", func(t *testing.T) {
		got := buildSSHEnvPrefix(map[string]string{"FOO": "bar baz"})
		// shellQuote wraps in single quotes; the prefix should end with a
		// trailing space so the next token (sh -c '...') doesn't run into
		// the value.
		if !strings.HasPrefix(got, "FOO='bar baz' ") {
			t.Errorf("buildSSHEnvPrefix = %q, want prefix \"FOO='bar baz' \"", got)
		}
	})

	t.Run("values with embedded single quotes are escaped", func(t *testing.T) {
		got := buildSSHEnvPrefix(map[string]string{"TOKEN": "it's-a-secret"})
		// shellQuote replaces ' with '\'' for POSIX-safe escaping.
		if !strings.Contains(got, `TOKEN='it'\''s-a-secret' `) {
			t.Errorf("buildSSHEnvPrefix did not escape single quote: %q", got)
		}
	})
}
