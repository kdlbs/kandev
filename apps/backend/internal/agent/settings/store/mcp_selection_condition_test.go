package store

import (
	"errors"
	"slices"
	"testing"

	"github.com/kandev/kandev/internal/agent/mcpconfig"
)

// @covers AC-AGENTS-MCP-ASSIGNMENT-003.1
func TestMCPSelectionConditionUsesMembershipNotDatabaseOrder(t *testing.T) {
	tenID := "10a00000-0000-4000-8000-000000000000"
	twoID := "2a000000-0000-4000-8000-000000000000"
	for _, tc := range []struct {
		name     string
		expected []string
		conflict bool
	}{
		{name: "same membership in another order", expected: []string{tenID, twoID}},
		{name: "removed member", expected: []string{twoID}, conflict: true},
		{name: "different member", expected: []string{twoID, "other"}, conflict: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			saved := []string{twoID, tenID}
			original := slices.Clone(saved)
			err := checkMCPSelectionCondition(mcpconfig.SelectionSnapshot{DefinitionIDs: saved}, mcpconfig.SelectionCondition{ExpectedDefinitionIDs: &tc.expected})
			if errors.Is(err, mcpconfig.ErrMCPSelectionConflict) != tc.conflict {
				t.Fatalf("condition returned %v; conflict wanted %v", err, tc.conflict)
			}
			if err != nil && !tc.conflict {
				t.Fatal(err)
			}
			if !slices.Equal(saved, original) {
				t.Fatalf("condition mutated saved snapshot: %v", saved)
			}
		})
	}
}
