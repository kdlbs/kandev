package configsync

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestParseAgentFile_Basic(t *testing.T) {
	content := []byte("name: Reviewer\nrole: reviewer\nreports_to: Lead\nbudget_monthly_cents: 500\n")
	a, err := parseAgentFile("cfg/agents/reviewer.yml", content)
	if err != nil {
		t.Fatalf("parseAgentFile() error = %v", err)
	}
	if a.declaredName != "Reviewer" || a.role != "reviewer" || a.reportsTo != "Lead" || a.budgetMonthlyCents != 500 {
		t.Errorf("parseAgentFile() = %+v, unexpected fields", a)
	}
}

func TestParseAgentFile_MissingNameFails(t *testing.T) {
	if _, err := parseAgentFile("cfg/agents/x.yml", []byte("role: reviewer\n")); err == nil {
		t.Fatal("parseAgentFile() error = nil, want error for missing name")
	}
}

func TestParseAgentFile_InvalidYAMLFails(t *testing.T) {
	if _, err := parseAgentFile("cfg/agents/x.yml", []byte("not: [valid yaml")); err == nil {
		t.Fatal("parseAgentFile() error = nil, want parse error")
	}
}

func TestParseProjectFile_Basic(t *testing.T) {
	content := []byte("name: Website\ndescription: Marketing site\nbudget_cents: 1000\n")
	p, err := parseProjectFile("cfg/projects/website.yml", content)
	if err != nil {
		t.Fatalf("parseProjectFile() error = %v", err)
	}
	if p.declaredName != "Website" || p.description != "Marketing site" || p.budgetCents != 1000 {
		t.Errorf("parseProjectFile() = %+v, unexpected fields", p)
	}
}

func TestParseProjectFile_MissingNameFails(t *testing.T) {
	if _, err := parseProjectFile("cfg/projects/x.yml", []byte("description: x\n")); err == nil {
		t.Fatal("parseProjectFile() error = nil, want error for missing name")
	}
}

func TestParseRoutineFile_Basic(t *testing.T) {
	content := []byte("name: Nightly Build\ntask_template: build\nconcurrency_policy: skip\n")
	r, err := parseRoutineFile("cfg/routines/nightly.yml", content)
	if err != nil {
		t.Fatalf("parseRoutineFile() error = %v", err)
	}
	if r.declaredName != "Nightly Build" || r.taskTemplate != "build" || r.concurrencyPolicy != "skip" {
		t.Errorf("parseRoutineFile() = %+v, unexpected fields", r)
	}
}

func TestParseRoutineFile_MissingNameFails(t *testing.T) {
	if _, err := parseRoutineFile("cfg/routines/x.yml", []byte("task_template: build\n")); err == nil {
		t.Fatal("parseRoutineFile() error = nil, want error for missing name")
	}
}

func TestParseSkill_NoSkillMDReturnsNilNil(t *testing.T) {
	sf := skillFiles{dirName: "empty", dirPath: "cfg/skills/empty"}
	skill, err := parseSkill(sf)
	if err != nil {
		t.Fatalf("parseSkill() error = %v, want nil", err)
	}
	if skill != nil {
		t.Fatalf("parseSkill() = %+v, want nil (no SKILL.md)", skill)
	}
}

func TestParseSkill_NoFrontmatterFallsBackToDirName(t *testing.T) {
	sf := skillFiles{
		dirName: "reviewer",
		dirPath: "cfg/skills/reviewer",
		skillMD: &fetchedFile{path: "cfg/skills/reviewer/SKILL.md", content: []byte("# Reviewer\n\nDoes review things.\n")},
	}
	skill, err := parseSkill(sf)
	if err != nil {
		t.Fatalf("parseSkill() error = %v", err)
	}
	if skill.name != "reviewer" {
		t.Errorf("name = %q, want fallback to dir name %q", skill.name, "reviewer")
	}
	if skill.description != "" {
		t.Errorf("description = %q, want empty (no frontmatter)", skill.description)
	}
	if skill.content != "# Reviewer\n\nDoes review things.\n" {
		t.Errorf("content = %q, want full raw file", skill.content)
	}
	if skill.fileInventory != "[]" {
		t.Errorf("fileInventory = %q, want empty array for no references", skill.fileInventory)
	}
}

func TestParseSkill_FrontmatterNameAndDescription(t *testing.T) {
	raw := "---\nname: Code Reviewer\ndescription: Reviews pull requests\n---\n# Body\n"
	sf := skillFiles{
		dirName: "reviewer",
		dirPath: "cfg/skills/reviewer",
		skillMD: &fetchedFile{path: "cfg/skills/reviewer/SKILL.md", content: []byte(raw)},
	}
	skill, err := parseSkill(sf)
	if err != nil {
		t.Fatalf("parseSkill() error = %v", err)
	}
	if skill.name != "Code Reviewer" {
		t.Errorf("name = %q, want frontmatter name", skill.name)
	}
	if skill.description != "Reviews pull requests" {
		t.Errorf("description = %q, want frontmatter description", skill.description)
	}
	if skill.content != raw {
		t.Errorf("content = %q, want full raw file including frontmatter", skill.content)
	}
}

func TestParseSkill_ReferencesBuildRelativePathInventory(t *testing.T) {
	sf := skillFiles{
		dirName: "reviewer",
		dirPath: "cfg/skills/reviewer",
		skillMD: &fetchedFile{path: "cfg/skills/reviewer/SKILL.md", content: []byte("# Reviewer\n")},
		references: []fetchedFile{
			{path: "cfg/skills/reviewer/references/checklist.md", content: []byte("checklist")},
			{path: "cfg/skills/reviewer/references/style.md", content: []byte("style")},
		},
	}
	skill, err := parseSkill(sf)
	if err != nil {
		t.Fatalf("parseSkill() error = %v", err)
	}
	var files []inventoryFile
	if err := json.Unmarshal([]byte(skill.fileInventory), &files); err != nil {
		t.Fatalf("fileInventory is not valid JSON: %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("fileInventory = %+v, want 2 entries", files)
	}
	if files[0].Path != "references/checklist.md" || files[1].Path != "references/style.md" {
		t.Errorf("inventory paths = %+v, want paths relative to the skill directory", files)
	}
	if files[0].Content != "checklist" {
		t.Errorf("inventory content = %q, want file content preserved", files[0].Content)
	}
}

func TestSplitFrontmatter_NoDelimiterReturnsFalse(t *testing.T) {
	if _, ok := splitFrontmatter("# Just a heading\n"); ok {
		t.Error("splitFrontmatter() ok = true, want false without a --- delimiter")
	}
}

func TestSplitFrontmatter_UnterminatedBlockReturnsFalse(t *testing.T) {
	if _, ok := splitFrontmatter("---\nname: x\n"); ok {
		t.Error("splitFrontmatter() ok = true, want false without a closing ---")
	}
}

func TestSplitFrontmatter_ExtractsYAMLPart(t *testing.T) {
	yamlPart, ok := splitFrontmatter("---\nname: x\ndescription: y\n---\nbody\n")
	if !ok {
		t.Fatal("splitFrontmatter() ok = false, want true")
	}
	if strings.TrimSpace(yamlPart) != "name: x\ndescription: y" {
		t.Errorf("splitFrontmatter() yamlPart = %q", yamlPart)
	}
}
