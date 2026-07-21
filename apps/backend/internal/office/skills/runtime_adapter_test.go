package skills

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/office/models"
)

type stubOfficeSkillReader struct {
	skill *models.Skill
}

func (s stubOfficeSkillReader) GetSkillFromConfig(context.Context, string) (*models.Skill, error) {
	return s.skill, nil
}

func TestSkillReaderAdapterMapsInventoryFiles(t *testing.T) {
	adapter := NewSkillReaderAdapter(stubOfficeSkillReader{skill: &models.Skill{
		Slug:          "office-admin",
		Content:       "# Office Admin\n",
		FileInventory: `[{"path":"references/tasks.md","content":"# Tasks\n"},{"path":"empty.md"}]`,
	}})

	got, err := adapter.GetSkillFromConfig(context.Background(), "office-admin")
	if err != nil {
		t.Fatalf("GetSkillFromConfig: %v", err)
	}
	if got == nil {
		t.Fatal("expected skill")
	}
	if len(got.Files) != 1 {
		t.Fatalf("files = %+v, want one content-bearing file", got.Files)
	}
	if got.Files[0].Path != "references/tasks.md" || got.Files[0].Content != "# Tasks\n" {
		t.Errorf("file = %+v", got.Files[0])
	}
}
