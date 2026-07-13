package lifecycle

import "testing"

func TestCleanSpriteSkillFilePathRejectsTraversal(t *testing.T) {
	for _, input := range []string{"..", "../x", "a/../..", "a/.."} {
		t.Run(input, func(t *testing.T) {
			if got, ok := cleanSpriteSkillFilePath(input); ok {
				t.Fatalf("cleanSpriteSkillFilePath(%q) = %q, true; want reject", input, got)
			}
		})
	}
}
