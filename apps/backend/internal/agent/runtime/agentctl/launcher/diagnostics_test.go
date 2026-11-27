package launcher

import (
	"io"
	"strings"
	"testing"
	"time"
)

func TestSanitizeDiagnosticRedactsCredentials(t *testing.T) {
	got := sanitizeDiagnostic("Authorization: Bearer bearer-secret token=api-secret password:open-sesame")
	for _, secret := range []string{"bearer-secret", "api-secret", "open-sesame"} {
		if strings.Contains(got, secret) {
			t.Fatalf("sanitized diagnostic exposes %q: %q", secret, got)
		}
	}
	if strings.Count(got, "[redacted]") != 3 {
		t.Fatalf("sanitized diagnostic = %q, want three redactions", got)
	}
}

func TestPipeOutputDrainsLongLinesAndKeepsBoundedDiagnostics(t *testing.T) {
	launcher := &Launcher{logger: newUnexpectedExitTestLogger(t)}
	reader, writer := io.Pipe()
	finished := make(chan struct{})
	go func() {
		launcher.pipeOutput("stderr", reader)
		close(finished)
	}()

	payload := "2026\tDEBUG\tcomponent\t" + strings.Repeat("x", pipeOutputRecordLimitBytes*3) + "\n" +
		"2026\tINFO\tcomponent\ttoken=secret-value\n"
	writeDone := make(chan error, 1)
	go func() {
		_, err := io.WriteString(writer, payload)
		_ = writer.Close()
		writeDone <- err
	}()
	select {
	case err := <-writeDone:
		if err != nil {
			t.Fatalf("write child output: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("child output writer blocked on a long line")
	}
	select {
	case <-finished:
	case <-time.After(2 * time.Second):
		t.Fatal("output reader did not drain and finish")
	}

	diagnostic := launcher.diagnostic()
	if len(diagnostic) > maxDiagnosticTailBytes {
		t.Fatalf("diagnostic length = %d, exceeds %d bytes", len(diagnostic), maxDiagnosticTailBytes)
	}
	if !strings.Contains(diagnostic, "discarded oversized agentctl log record") {
		t.Fatal("oversized diagnostic record did not retain a content-free discard marker")
	}
	if strings.Contains(diagnostic, strings.Repeat("x", 100)) {
		t.Fatal("oversized diagnostic record retained raw content")
	}
	if strings.Contains(diagnostic, "secret-value") || !strings.Contains(diagnostic, "token=[redacted]") {
		t.Fatalf("diagnostic credential was not sanitized: %q", diagnostic)
	}
}
