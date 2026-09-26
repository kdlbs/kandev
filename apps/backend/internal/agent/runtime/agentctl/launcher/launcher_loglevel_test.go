package launcher

import (
	"bytes"
	stdlog "log"
	"log/slog"
	"strings"
	"testing"
)

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
		{
			name: "two-field record is too short to be a level",
			line: "2026-08-12T10:00:00.000Z\tINFO",
			want: "",
		},
		{
			name: "three-field record with empty message still has level",
			line: "2026-08-12T10:00:00.000Z\tINFO\tmain.go:97",
			want: "INFO",
		},
		{
			name: "unterminated ansi escape falls back to no level",
			line: "2026-08-12T10:00:00.000Z\tINFO\x1b[34\tmain.go:97\tstarting agentctl",
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

func TestChildLogLevelSlogTextRecords(t *testing.T) {
	tests := []struct {
		name  string
		level string
		want  string
	}{
		{name: "debug", level: "DEBUG", want: "DEBUG"},
		{name: "info", level: "INFO", want: "INFO"},
		{name: "warn", level: "WARN", want: "WARN"},
		{name: "error", level: "ERROR", want: "ERROR"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			line := "time=2026-08-12T10:00:00.000Z level=" + tt.level +
				" source=main.go:97 msg=\"agentctl record\""
			if got := childLogLevel(line); got != tt.want {
				t.Fatalf("childLogLevel(%q) = %q, want %q", line, got, tt.want)
			}
		})
	}
}

func TestChildLogLevelSlogDefaultRecords(t *testing.T) {
	var output bytes.Buffer
	originalWriter := stdlog.Writer()
	originalFlags := stdlog.Flags()
	stdlog.SetOutput(&output)
	stdlog.SetFlags(stdlog.LstdFlags)
	t.Cleanup(func() {
		stdlog.SetOutput(originalWriter)
		stdlog.SetFlags(originalFlags)
	})

	tests := []struct {
		name  string
		write func()
		want  string
	}{
		{name: "info", write: func() { slog.Default().Info("agentctl record", "component", "acp-conn") }, want: "INFO"},
		{name: "warn", write: func() { slog.Default().Warn("agentctl record", "component", "acp-conn") }, want: "WARN"},
		{name: "error", write: func() { slog.Default().Error("agentctl record", "component", "acp-conn") }, want: "ERROR"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output.Reset()
			tt.write()
			line := strings.TrimSpace(output.String())
			if line == "" {
				t.Fatal("slog.Default produced no output")
			}
			if got := childLogLevel(line); got != tt.want {
				t.Fatalf("childLogLevel(%q) = %q, want %q", line, got, tt.want)
			}
		})
	}
}

func TestChildLogLevelRejectsMalformedSlogTextRecords(t *testing.T) {
	tests := []struct {
		name string
		line string
	}{
		{
			name: "missing time anchor",
			line: "level=INFO source=main.go:97 msg=\"agentctl record\"",
		},
		{
			name: "missing level anchor",
			line: "time=2026-08-12T10:00:00.000Z msg=\"level=INFO\"",
		},
		{
			name: "missing message field",
			line: "time=2026-08-12T10:00:00.000Z level=INFO source=main.go:97",
		},
		{
			name: "level anchor is not second field",
			line: "time=2026-08-12T10:00:00.000Z source=main.go:97 level=INFO msg=\"record\"",
		},
		{
			name: "message-looking key is inside a quoted attribute",
			line: `time=2026-08-12T10:00:00.000Z level=INFO attr="foo msg=bar" component=acp-conn`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := childLogLevel(tt.line); got != "" {
				t.Fatalf("childLogLevel(%q) = %q, want no recognized level", tt.line, got)
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
		{name: "unterminated escape keeps raw byte", in: "INFO\x1b[34", want: "INFO\x1b[34"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := stripANSI(tt.in); got != tt.want {
				t.Fatalf("stripANSI(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
