package skills

import "testing"

func TestDefaultProjectSkillDir_IsAgentsSkills(t *testing.T) {
	if DefaultProjectSkillDir != ".agents/skills" {
		t.Errorf("DefaultProjectSkillDir = %q, want %q", DefaultProjectSkillDir, ".agents/skills")
	}
}

func TestParseDesiredSlugs_JSON(t *testing.T) {
	slugs := ParseDesiredSlugs(`["code-review","memory"]`)
	if len(slugs) != 2 || slugs[0] != "code-review" || slugs[1] != "memory" {
		t.Errorf("unexpected slugs: %v", slugs)
	}
}

func TestParseDesiredSlugs_Comma(t *testing.T) {
	slugs := ParseDesiredSlugs("code-review,memory")
	if len(slugs) != 2 {
		t.Errorf("unexpected slugs: %v", slugs)
	}
}

func TestParseDesiredSlugs_Empty(t *testing.T) {
	if slugs := ParseDesiredSlugs(""); slugs != nil {
		t.Errorf("expected nil, got %v", slugs)
	}
	if slugs := ParseDesiredSlugs("[]"); slugs != nil {
		t.Errorf("expected nil, got %v", slugs)
	}
}
