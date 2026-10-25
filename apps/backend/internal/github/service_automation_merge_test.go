package github

import (
	"context"
	"strings"
	"testing"
)

// @covers AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-002.1
func TestMergePRForAutomationRequiresExpectedHeadSHA(t *testing.T) {
	service := &Service{}
	err := service.MergePRForAutomation(
		context.Background(), "workspace-1", "acme", "widget", 42, "squash", "",
	)
	if err == nil || !strings.Contains(err.Error(), "expected head SHA") {
		t.Fatalf("error = %v, want expected-head validation", err)
	}
}
