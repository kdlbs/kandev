package backendapp

import (
	"os"
	"strings"
	"testing"
)

func TestBuildHTTPServerWiresAutomationsHostSource(t *testing.T) {
	source, err := os.ReadFile("helpers.go")
	if err != nil {
		t.Fatalf("read helpers.go: %v", err)
	}
	text := string(source)
	wiring := strings.Index(text, "p.services.Plugins.SetAutomationSource(pluginsAutomationSourceAdapter{svc: p.services.Automation.Service})")
	if wiring < 0 {
		t.Fatal("buildHTTPServer must wire its Automation service into the plugin Automations Host API")
	}
}
