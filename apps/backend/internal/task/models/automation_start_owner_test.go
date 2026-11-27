package models

import "testing"

func TestAutomationStartOwnedRecognizesExactOwner(t *testing.T) {
	for _, tc := range []struct {
		name  string
		value interface{}
		want  bool
	}{
		{"exact run owner", "run-original", true},
		{"legacy marker", true, true},
		{"empty owner", "", false},
		{"released marker", false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := AutomationStartOwned(map[string]interface{}{MetaKeyAutomationStartOwned: tc.value}); got != tc.want {
				t.Fatalf("owned = %v, want %v", got, tc.want)
			}
		})
	}
}
