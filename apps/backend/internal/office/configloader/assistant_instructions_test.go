package configloader

import "testing"

func TestAssistantHasEntryInstructions(t *testing.T) {
	templates, err := LoadRoleTemplates("assistant")
	if err != nil {
		t.Fatal(err)
	}
	for _, template := range templates {
		if template.Filename == "AGENTS.md" && template.Content != "" {
			return
		}
	}
	t.Fatal("assistant creation must receive an editable entry instruction file")
}
