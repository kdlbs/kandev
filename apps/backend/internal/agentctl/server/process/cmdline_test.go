package process

import "testing"

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
		{"backslash no quote", `a\b`, `a\b`},
		{"backslash before quote", `a\"`, `a\\\"`},
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
		{"arg with space", []string{"C:\\Program Files\\app.exe", "-f"}, `"C:\Program Files\app.exe" -f`},
		{"arg with quote", []string{"echo", `"hello"`}, `echo \"hello\"`},
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
