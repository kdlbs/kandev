package skills

import (
	"errors"
	"testing"
)

// TestErrorsAreClassifiable verifies that the file-accessor helper wraps its
// not-found error with the package sentinel so HTTP callers
// (handler.getSkillFile) can classify via errors.Is. The GetSkillFromConfig
// wrap is covered separately in service_test.go's external-package test.
func TestErrorsAreClassifiable(t *testing.T) {
	t.Run("readUserHomeSkillInventoryFile returns ErrSkillFileNotFound", func(t *testing.T) {
		_, err := readUserHomeSkillInventoryFile(`[]`, "missing.md")
		if err == nil {
			t.Fatal("expected error for missing file in empty inventory")
		}
		if !errors.Is(err, ErrSkillFileNotFound) {
			t.Errorf("error not classifiable as ErrSkillFileNotFound: %v", err)
		}
	})
}
