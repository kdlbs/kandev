package storeconformance

import (
	"testing"

	"github.com/kandev/kandev/internal/coordinator"
)

func TestCoordinatorConformanceRejectsUnpersistedNamePatch(t *testing.T) {
	action := coordinatorAction()
	if action.assertUpdated == nil {
		t.Fatal("coordinator patch lacks a persistence assertion")
	}
	before := &coordinator.Coordinator{ID: "id", Name: "original"}
	if err := action.assertUpdated(before, before, "id"); err == nil {
		t.Fatal("unchanged coordinator passed update conformance")
	}
	if err := action.assertUpdated(before, &coordinator.Coordinator{ID: "id", Name: "Updated id"}, "id"); err != nil {
		t.Fatalf("persisted update refused: %v", err)
	}
}
