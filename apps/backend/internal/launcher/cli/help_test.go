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
	if strings.Contains(help, "__backend") {
		t.Fatalf("help exposes hidden backend mode:\n%s", help)
	}
}
