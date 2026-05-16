package configloader

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnsureBundledSkills(t *testing.T) {
	base := t.TempDir()

	if err := EnsureBundledSkills(base); err != nil {
		t.Fatalf("EnsureBundledSkills() error: %v", err)
	}

	// Verify kandev-protocol skill was written.
	protocolPath := filepath.Join(base, "skills", "kandev-protocol", "SKILL.md")
	data, err := os.ReadFile(protocolPath)
	if err != nil {
		t.Fatalf("read kandev-protocol SKILL.md: %v", err)
	}
	if len(data) == 0 {
		t.Error("kandev-protocol SKILL.md is empty")
	}

	// Verify memory skill was written.
	memoryPath := filepath.Join(base, "skills", "memory", "SKILL.md")
	data, err = os.ReadFile(memoryPath)
	if err != nil {
		t.Fatalf("read memory SKILL.md: %v", err)
	}
	if len(data) == 0 {
		t.Error("memory SKILL.md is empty")
	}

	// Verify kandev-escalation skill was written.
	escalationPath := filepath.Join(base, "skills", "kandev-escalation", "SKILL.md")
	data, err = os.ReadFile(escalationPath)
	if err != nil {
		t.Fatalf("read kandev-escalation SKILL.md: %v", err)
	}
	if len(data) == 0 {
		t.Error("kandev-escalation SKILL.md is empty")
	}
}

func TestEmbeddedSkills_ContainsEscalation(t *testing.T) {
	slugs, err := BundledSkillSlugs()
	if err != nil {
		t.Fatalf("BundledSkillSlugs() error: %v", err)
	}
	found := false
	for _, s := range slugs {
		if s == "kandev-escalation" {
			found = true
			break
		}
	}
	if !found {
		t.Error("kandev-escalation skill not found in bundled skills")
	}

	// Verify the content is meaningful (contains required sections).
	base := t.TempDir()
	if err := EnsureBundledSkills(base); err != nil {
		t.Fatalf("EnsureBundledSkills() error: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(base, "skills", "kandev-escalation", "SKILL.md"))
	if err != nil {
		t.Fatalf("read kandev-escalation SKILL.md: %v", err)
	}
	content := string(data)
	for _, section := range []string{"When to escalate", "Escalation procedure", "task_blockers_resolved"} {
		if !strings.Contains(content, section) {
			t.Errorf("kandev-escalation SKILL.md missing section %q", section)
		}
	}
}

func TestEnsureBundledSkillsIdempotent(t *testing.T) {
	base := t.TempDir()

	// Write twice; second call should overwrite without error.
	if err := EnsureBundledSkills(base); err != nil {
		t.Fatalf("first call: %v", err)
	}
	if err := EnsureBundledSkills(base); err != nil {
		t.Fatalf("second call: %v", err)
	}
}

func TestBundledSkillSlugs(t *testing.T) {
	slugs, err := BundledSkillSlugs()
	if err != nil {
		t.Fatalf("BundledSkillSlugs() error: %v", err)
	}
	if len(slugs) < 3 {
		t.Fatalf("got %d slugs, want at least 3", len(slugs))
	}

	expected := map[string]bool{"kandev-protocol": false, "memory": false, "kandev-escalation": false}
	for _, slug := range slugs {
		if _, ok := expected[slug]; ok {
			expected[slug] = true
		}
	}
	for slug, found := range expected {
		if !found {
			t.Errorf("missing bundled skill: %s", slug)
		}
	}
}
