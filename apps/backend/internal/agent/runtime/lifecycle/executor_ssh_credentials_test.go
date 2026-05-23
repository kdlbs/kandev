package lifecycle

import (
	"strings"
	"testing"
)

func TestWrapLoginShell(t *testing.T) {
	t.Run("empty shell defaults to bash", func(t *testing.T) {
		got := WrapLoginShell("", "echo hi")
		if !strings.HasPrefix(got, "bash -lc ") {
			t.Errorf("WrapLoginShell with empty shell = %q, want bash -lc prefix", got)
		}
	})

	t.Run("custom shell is used verbatim", func(t *testing.T) {
		got := WrapLoginShell("zsh", "echo hi")
		if !strings.HasPrefix(got, "zsh -lc ") {
			t.Errorf("WrapLoginShell with zsh = %q, want zsh -lc prefix", got)
		}
	})

	t.Run("inner command is single-quoted", func(t *testing.T) {
		got := WrapLoginShell("bash", "echo hi")
		if !strings.Contains(got, "'echo hi'") {
			t.Errorf("WrapLoginShell did not single-quote inner cmd: %q", got)
		}
	})

	t.Run("embedded single quote escaped POSIX-safe", func(t *testing.T) {
		// shellQuote's contract is to replace ' with '\'' so a payload
		// like `echo "it's"` becomes 'echo "it'\''s"' — preserving the
		// single quote literally inside the bash -lc argument.
		got := WrapLoginShell("bash", `echo "it's"`)
		if !strings.Contains(got, `'echo "it'\''s"'`) {
			t.Errorf("WrapLoginShell did not escape single quote correctly: %q", got)
		}
	})

	t.Run("multiline scripts survive intact", func(t *testing.T) {
		script := "set -e\nmkdir -p /tmp/x\ncat <<EOF > /tmp/x/f\nhello\nEOF"
		got := WrapLoginShell("bash", script)
		// Newlines inside single-quoted args are valid POSIX shell input.
		if !strings.Contains(got, "set -e\nmkdir -p /tmp/x") {
			t.Errorf("WrapLoginShell mangled multiline script: %q", got)
		}
	})
}

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
