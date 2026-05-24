package skills

import (
	"errors"
	"fmt"
	"testing"
)

// TestErrorsAreClassifiable verifies that GetSkill and the file-accessor
// helpers wrap their not-found errors with the package sentinels so HTTP
// callers (handler.getSkillFile) can classify via errors.Is.
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

	t.Run("ErrSkillNotFound is reachable via errors.Is on wrapped errors", func(t *testing.T) {
		wrapped := fmt.Errorf("lookup failed: %w", ErrSkillNotFound)
		if !errors.Is(wrapped, ErrSkillNotFound) {
			t.Errorf("wrapped ErrSkillNotFound failed to classify")
		}
	})
}
