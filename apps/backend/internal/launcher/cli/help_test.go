package cli

import (
	"strings"
	"testing"
)

func TestHelpDoesNotExposeHiddenBackendMode(t *testing.T) {
	help := Help()
	if !strings.Contains(help, "kandev run") {
		t.Fatalf("help does not describe public run command:\n%s", help)
	}
	if strings.Contains(help, "kandev dev") || strings.Contains(help, "--dev") {
		t.Fatalf("help exposes unsupported dev mode:\n%s", help)
	}
	if strings.Contains(help, "__backend") {
		t.Fatalf("help exposes hidden backend mode:\n%s", help)
	}
	if !strings.Contains(help, "info logs on stdout") || !strings.Contains(help, "debug logs in the backend file") {
		t.Fatalf("help does not explain file/stdout diagnostics:\n%s", help)
	}
}
