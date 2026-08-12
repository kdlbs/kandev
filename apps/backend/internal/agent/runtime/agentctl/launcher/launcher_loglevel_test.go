package launcher

import "testing"

func TestChildLogLevel(t *testing.T) {
	tests := []struct {
		name string
		line string
		want string
	}{
		{
			name: "plain info line",
			line: "2026-08-12T10:00:00.000Z\tINFO\tmain.go:97\tstarting agentctl",
			want: "INFO",
		},
		{
			name: "plain warn line",
			line: "2026-08-12T10:00:00.000Z\tWARN\tserver.go:12\tslow request",
			want: "WARN",
		},
		{
			name: "plain error line",
			line: "2026-08-12T10:00:00.000Z\tERROR\tmain.go:213\tHTTP server failed to bind",
			want: "ERROR",
		},
		{
			name: "plain debug line",
			line: "2026-08-12T10:00:00.000Z\tDEBUG\tx.go:1\tnoise",
			want: "DEBUG",
		},
		{
			name: "ansi-wrapped level token",
			line: "2026-08-12T10:00:00.000Z\t\x1b[34mINFO\x1b[0m\tmain.go:97\tstarting agentctl",
			want: "INFO",
		},
		{
			name: "lowercase level normalizes to upper",
			line: "2026-08-12T10:00:00.000Z\tinfo\tmain.go:97\tstarting agentctl",
			want: "INFO",
		},
		{
			name: "unstructured line has no level",
			line: "panic: runtime error: invalid memory address",
			want: "",
		},
		{
			name: "single field line has no level",
			line: "just-one-field",
			want: "",
		},
		{
			name: "unknown token is not a level",
			line: "2026-08-12T10:00:00.000Z\tTRACE\tx.go:1\tsomething",
			want: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := childLogLevel(tt.line); got != tt.want {
				t.Fatalf("childLogLevel(%q) = %q, want %q", tt.line, got, tt.want)
			}
		})
	}
}

func TestStripANSI(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "no escapes", in: "INFO", want: "INFO"},
		{name: "color wrapped", in: "\x1b[34mINFO\x1b[0m", want: "INFO"},
		{name: "multiple sequences", in: "\x1b[1m\x1b[31mERROR\x1b[0m", want: "ERROR"},
		{name: "unterminated escape drops tail", in: "INFO\x1b[34", want: "INFO"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := stripANSI(tt.in); got != tt.want {
				t.Fatalf("stripANSI(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
