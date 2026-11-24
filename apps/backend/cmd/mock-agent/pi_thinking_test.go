package main

import "testing"

func TestPiThinkingFixture(t *testing.T) {
	t.Setenv("KANDEV_MOCK_PI_THINKING", "true")
	options := mockSessionConfigOptionsForModel("mock-fast")
	found := false
	for _, option := range options {
		if option.Select != nil && string(option.Select.Id) == "thought_level" {
			found = true
			if option.Select.CurrentValue != "off" || option.Select.Options.Ungrouped == nil || len(*option.Select.Options.Ungrouped) != 1 {
				t.Fatalf("wrong off-only snapshot: %+v", option.Select)
			}
		}
	}
	if !found {
		t.Fatal("missing Pi thinking config")
	}
}
