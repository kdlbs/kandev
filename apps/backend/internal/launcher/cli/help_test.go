package cli

import (
	"strings"
	"testing"
)

func TestHelpDescribesPublicCommands(t *testing.T) {
	help := Help()
	for _, want := range []string{
		"kandev run",
		"kandev dev",
		"kandev start",
		"--dev",
		"--web-internal-port",
	} {
		if !strings.Contains(help, want) {
			t.Fatalf("help does not describe %q:\n%s", want, help)
		}
	}
	if strings.Contains(help, "__backend") {
		t.Fatalf("help exposes hidden backend mode:\n%s", help)
	}
	if !strings.Contains(help, "info logs on stdout") || !strings.Contains(help, "debug logs in the backend file") {
		t.Fatalf("help does not explain file/stdout diagnostics:\n%s", help)
	}
}
