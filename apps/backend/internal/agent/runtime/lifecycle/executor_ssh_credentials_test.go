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

func TestBuildSSHEnvInitScript(t *testing.T) {
	t.Run("empty map returns empty string", func(t *testing.T) {
		if got := buildSSHEnvInitScript(nil); got != "" {
			t.Errorf("buildSSHEnvInitScript(nil) = %q, want \"\"", got)
		}
		if got := buildSSHEnvInitScript(map[string]string{}); got != "" {
			t.Errorf("buildSSHEnvInitScript(empty) = %q, want \"\"", got)
		}
	})

	t.Run("single env var is shell-quoted on its own line", func(t *testing.T) {
		got := buildSSHEnvInitScript(map[string]string{"FOO": "bar baz"})
		// Each line is a POSIX shell assignment; the line break separates
		// entries so `. /dev/stdin` under `set -a` exports each one.
		if got != "FOO='bar baz'\n" {
			t.Errorf("buildSSHEnvInitScript = %q, want \"FOO='bar baz'\\n\"", got)
		}
	})

	t.Run("values with embedded single quotes are escaped", func(t *testing.T) {
		got := buildSSHEnvInitScript(map[string]string{"TOKEN": "it's-a-secret"})
		// shellQuote replaces ' with '\'' for POSIX-safe escaping.
		if !strings.Contains(got, `TOKEN='it'\''s-a-secret'`) {
			t.Errorf("buildSSHEnvInitScript did not escape single quote: %q", got)
		}
		if !strings.HasSuffix(got, "\n") {
			t.Errorf("buildSSHEnvInitScript missing trailing newline: %q", got)
		}
	})
}
