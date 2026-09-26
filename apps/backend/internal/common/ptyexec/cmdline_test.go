package ptyexec

import (
	"os/exec"
	"reflect"
	"strings"
	"testing"
)

func TestEscapeArg(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", `""`},
		{"simple", "hello", "hello"},
		{"with space", "hello world", `"hello world"`},
		{"with tab", "hello\tworld", `"hello` + "\t" + `world"`},
		{"with quote", `say "hi"`, `"say \"hi\""`},
		{"quote only", `"`, `\"`},
		{"backslash no quote", `a\b`, `a\b`},
		{"backslash before quote", `a\"`, `a\\\"`},
		// A trailing backslash only needs doubling when the argument gets
		// wrapped in quotes — otherwise it cannot run into the closing quote.
		{"trailing backslash no space", `a\`, `a\`},
		{"trailing backslash with space", `a b\`, `"a b\\"`},
		{"multiple trailing backslashes with space", `a b\\`, `"a b\\\\"`},
		{"only backslashes", `\\`, `\\`},
		{"backslash then quote", `\\"`, `\\\\\"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := escapeArg(tt.in)
			if got != tt.want {
				t.Errorf("escapeArg(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestBuildCmdLine(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"single", []string{"cmd.exe"}, "cmd.exe"},
		{"simple args", []string{"git", "status"}, "git status"},
		{"arg with space", []string{`C:\Program Files\app.exe`, "-f"}, `"C:\Program Files\app.exe" -f`},
		{"arg with quote", []string{"echo", `"hello"`}, `echo \"hello\"`},
		{"empty arg is preserved", []string{"app.exe", "", "-x"}, `app.exe "" -x`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildCmdLine(tt.args)
			if got != tt.want {
				t.Errorf("buildCmdLine(%v) = %q, want %q", tt.args, got, tt.want)
			}
		})
	}
}

// TestBuildCmdLineRoundTrip is the property that actually matters: the string we
// hand to ConPTY must be parsed back into the exact argv we started with by
// CommandLineToArgvW. Golden strings pin today's output, but they are
// hand-computed and so cannot prove the escaping is *correct* — this decodes with
// an independent implementation of the documented parsing rules instead.
func TestBuildCmdLineRoundTrip(t *testing.T) {
	// args[0] is always a plain program path: CommandLineToArgvW parses argv[0]
	// under special rules (backslashes are never escapes there), and a path with
	// no backslash-before-quote sequence decodes identically under both.
	cases := [][]string{
		{"cmd.exe"},
		{"git", "status"},
		{`C:\Program Files\app.exe`, "-f"},
		{"echo", `"hello"`},
		{"app.exe", ""},
		{"app.exe", "", "", "-x"},
		{"app.exe", `a\`},
		{"app.exe", `a b\`},
		{"app.exe", `a b\\`},
		{"app.exe", `a\"`},
		{"app.exe", `a b\"`},
		{"app.exe", `\\"`},
		{"app.exe", `say "hi"`},
		{"app.exe", "tab\there"},
		{"app.exe", `C:\path with spaces\`, `plain`},
		{"app.exe", `--flag="quoted value"`},
		{"app.exe", `\\server\share`},
		{"app.exe", `"`, `\`, `""`},
	}
	for _, want := range cases {
		t.Run(strings.Join(want, "|"), func(t *testing.T) {
			line := buildCmdLine(want)
			got := commandLineToArgv(line)
			if !reflect.DeepEqual(got, want) {
				t.Errorf("round trip failed\n  argv in:  %q\n  cmdline:  %s\n  argv out: %q", want, line, got)
			}
		})
	}
}

// commandLineToArgv decodes a Windows command line following the documented
// CommandLineToArgvW rules:
//
//   - whitespace separates arguments unless it appears inside quotes;
//   - 2n backslashes followed by a quote emit n backslashes and toggle the
//     quoted state;
//   - 2n+1 backslashes followed by a quote emit n backslashes and a literal
//     quote, leaving the quoted state unchanged;
//   - backslashes not followed by a quote are literal.
//
// This is test-only: it exists purely as an independent oracle for buildCmdLine.
func commandLineToArgv(line string) []string {
	var (
		argv    []string
		current []byte
		started bool
		inQuote bool
	)
	flush := func() {
		if started {
			argv = append(argv, string(current))
			current = nil
			started = false
		}
	}
	for i := 0; i < len(line); {
		c := line[i]
		switch {
		case (c == ' ' || c == '\t') && !inQuote:
			flush()
			i++
		case c == '\\':
			slashes := 0
			for i < len(line) && line[i] == '\\' {
				slashes++
				i++
			}
			started = true
			if i < len(line) && line[i] == '"' {
				for n := 0; n < slashes/2; n++ {
					current = append(current, '\\')
				}
				if slashes%2 == 1 {
					current = append(current, '"')
					i++
				} else {
					inQuote = !inQuote
					i++
				}
			} else {
				for n := 0; n < slashes; n++ {
					current = append(current, '\\')
				}
			}
		case c == '"':
			started = true
			inQuote = !inQuote
			i++
		default:
			started = true
			current = append(current, c)
			i++
		}
	}
	flush()
	return argv
}

// TestResolveConPtyCmdLine_CmdShimIsWrapped guards against the regression that
// broke passthrough and login sessions for every npm-installed CLI on Windows:
// CreateProcessW (used by ConPTY) doesn't apply PATHEXT and can't execute
// .cmd / .bat scripts directly. exec.Command resolves the binary into cmd.Path;
// we must (a) use that resolved path instead of the bare Args[0] and (b) wrap
// batch scripts with cmd.exe /c so the interpreter runs them.
func TestResolveConPtyCmdLine_CmdShimIsWrapped(t *testing.T) {
	tests := []struct {
		name string
		cmd  *exec.Cmd
		want string
	}{
		{
			name: "cmd shim with args",
			cmd: &exec.Cmd{
				Path: `C:\Users\test\AppData\Roaming\npm\opencode.cmd`,
				Args: []string{"opencode", "--model", "big-pickle"},
			},
			want: `cmd.exe /c C:\Users\test\AppData\Roaming\npm\opencode.cmd --model big-pickle`,
		},
		{
			name: "cmd shim without args",
			cmd: &exec.Cmd{
				Path: `C:\Users\test\AppData\Roaming\npm\claude.cmd`,
				Args: []string{"claude"},
			},
			want: `cmd.exe /c C:\Users\test\AppData\Roaming\npm\claude.cmd`,
		},
		{
			name: "bat shim is wrapped too",
			cmd: &exec.Cmd{
				Path: `C:\tools\thing.BAT`,
				Args: []string{"thing"},
			},
			want: `cmd.exe /c C:\tools\thing.BAT`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := resolveConPtyCmdLine(tt.cmd); got != tt.want {
				t.Errorf("resolveConPtyCmdLine:\n  got:  %s\n  want: %s", got, tt.want)
			}
		})
	}
}

// TestResolveConPtyCmdLine_ExePassesThrough confirms .exe binaries (and any
// non-batch path) skip the cmd.exe wrapper — wrapping would lose access to
// signals and add a needless intermediate process for the common case.
func TestResolveConPtyCmdLine_ExePassesThrough(t *testing.T) {
	cmd := &exec.Cmd{
		Path: `C:\Program Files\Git\bin\bash.exe`,
		Args: []string{"bash", "-l"},
	}
	got := resolveConPtyCmdLine(cmd)
	if strings.HasPrefix(got, "cmd.exe /c ") {
		t.Errorf("resolveConPtyCmdLine wrapped a non-batch executable: %s", got)
	}
	if !strings.Contains(got, cmd.Path) {
		t.Errorf("resolveConPtyCmdLine should substitute resolved cmd.Path %q for Args[0], got: %s", cmd.Path, got)
	}
}

// TestResolveConPtyCmdLine_PathSubstitutedForArgs0 confirms cmd.Path is what
// CreateProcessW receives, not the unresolved cmd.Args[0]. Without this,
// "opencode" reaches CreateProcessW and fails because the loader can't resolve
// a bare name without PATHEXT.
func TestResolveConPtyCmdLine_PathSubstitutedForArgs0(t *testing.T) {
	cmd := &exec.Cmd{
		Path: `C:\Tools\foo.exe`,
		Args: []string{"foo", "bar"},
	}
	got := resolveConPtyCmdLine(cmd)
	if !strings.HasPrefix(got, `C:\Tools\foo.exe `) {
		t.Errorf("expected cmd.Path as first token; got: %s", got)
	}
	if strings.HasPrefix(got, "foo ") {
		t.Errorf("first token is unresolved Args[0]; got: %s", got)
	}
}

// TestResolveConPtyCmdLine_Degenerate covers the empty-Cmd guards, which exist so
// a misconfigured caller gets an empty command line rather than an index panic.
func TestResolveConPtyCmdLine_Degenerate(t *testing.T) {
	if got := resolveConPtyCmdLine(&exec.Cmd{}); got != "" {
		t.Errorf("empty Cmd: got %q, want empty string", got)
	}
	if got := resolveConPtyCmdLine(&exec.Cmd{Path: `C:\a\b.exe`}); got != `C:\a\b.exe` {
		t.Errorf("Path-only Cmd: got %q, want %q", got, `C:\a\b.exe`)
	}
}
