package backendapp

import (
	"os"
	"strings"
	"testing"
)

func TestBuildHTTPServerWiresTaskRelationsHostSource(t *testing.T) {
	source, err := os.ReadFile("helpers.go")
	if err != nil {
		t.Fatalf("read helpers.go: %v", err)
	}
	text := string(source)
	construct := strings.Index(text, "handoffSvc := taskservice.NewHandoffService")
	wiring := strings.Index(text, "p.services.Plugins.SetTaskRelationsSource(handoffSvc)")
	if construct < 0 || wiring < 0 || wiring < construct {
		t.Fatal("buildHTTPServer must wire its HandoffService into the plugin task-relations Host API after constructing it")
	}
}
