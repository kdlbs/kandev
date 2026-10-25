package skill_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle/skill"
	settingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	"github.com/kandev/kandev/internal/office/configloader"
)

// TestDeploy_OfficeRuntimeGatesSystemSkills is the regression test for
// the heavy-routine defect: a launch whose finalized env carries none
// of the Office runtime variables (KANDEV_CLI, KANDEV_API_KEY, ...)
// must not receive bundled system (Office) skills, since every one of
// their instructions depends on those variables. A launch that does
// have Office runtime env (the office scheduler path) keeps receiving
// them. A user-authored (non-system) skill lands either way.
func TestDeploy_OfficeRuntimeGatesSystemSkills(t *testing.T) {
	slugs, err := configloader.BundledSkillSlugs()
	if err != nil || len(slugs) == 0 {
		t.Fatalf("BundledSkillSlugs: %v (len=%d)", err, len(slugs))
	}

	skills := make(map[string]*skill.Skill, len(slugs)+1)
	skillIDs := make([]string, 0, len(slugs)+1)
	for _, slug := range slugs {
		content, err := configloader.BundledSkillContent(slug)
		if err != nil {
			t.Fatalf("BundledSkillContent(%s): %v", slug, err)
		}
		skills[slug] = &skill.Skill{Slug: slug, Content: string(content), IsSystem: true}
		skillIDs = append(skillIDs, slug)
	}
	// Control: a user-authored skill must land regardless of OfficeRuntime.
	skills["sk-user"] = &skill.Skill{Slug: "sk-user", Content: "# user skill"}
	skillIDs = append(skillIDs, "sk-user")

	skillIDsJSON, err := json.Marshal(skillIDs)
	if err != nil {
		t.Fatalf("marshal skill ids: %v", err)
	}
	reader := &fakeSkillReader{skills: skills}

	deployedDirs := func(t *testing.T, officeRuntime bool) map[string]bool {
		t.Helper()
		base := t.TempDir()
		worktree := t.TempDir()
		d := newDeployer(t, base, reader, &fakeInstructionLister{})
		if _, err := d.Deploy(context.Background(), skill.Request{
			Profile: &settingsmodels.AgentProfile{
				ID:       "routine-agent",
				SkillIDs: string(skillIDsJSON),
			},
			ExecutorType:         "worktree",
			WorkspacePath:        worktree,
			AdditionalSkillSlugs: []string{skill.ReservedDecisionSkillSlug},
			OfficeRuntime:        officeRuntime,
		}); err != nil {
			t.Fatalf("Deploy: %v", err)
		}
		root := filepath.Join(worktree, skill.DefaultProjectSkillDir)
		entries, err := os.ReadDir(root)
		if err != nil {
			if os.IsNotExist(err) {
				return map[string]bool{}
			}
			t.Fatalf("ReadDir: %v", err)
		}
		got := make(map[string]bool, len(entries))
		for _, e := range entries {
			got[e.Name()] = true
		}
		return got
	}

	t.Run("no office runtime env: zero system skills land", func(t *testing.T) {
		got := deployedDirs(t, false)
		for _, s := range slugs {
			if got[skill.DirName(s)] {
				t.Errorf("system skill %q should not be deployed without office runtime env", s)
			}
		}
		if !got[skill.DirName("sk-user")] {
			t.Errorf("user skill should still be deployed without office runtime env")
		}
	})

	t.Run("office runtime env present: all skills land", func(t *testing.T) {
		got := deployedDirs(t, true)
		for _, s := range slugs {
			if !got[skill.DirName(s)] {
				t.Errorf("system skill %q should be deployed with office runtime env", s)
			}
		}
		if !got[skill.DirName("sk-user")] {
			t.Errorf("user skill should be deployed with office runtime env")
		}
	})
}
