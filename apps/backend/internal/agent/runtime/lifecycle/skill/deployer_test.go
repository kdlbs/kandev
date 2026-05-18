package skill_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle/skill"
	settingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	"github.com/kandev/kandev/internal/common/logger"
)

// fakeSkillReader returns canned skill records.
type fakeSkillReader struct {
	skills map[string]*skill.Skill
}

func (f *fakeSkillReader) GetSkillFromConfig(_ context.Context, key string) (*skill.Skill, error) {
	return f.skills[key], nil
}

// fakeInstructionLister returns a canned slice for the configured agent.
type fakeInstructionLister struct {
	files map[string][]*skill.InstructionFile
}

func (f *fakeInstructionLister) ListInstructions(_ context.Context, agentID string) ([]*skill.InstructionFile, error) {
	return f.files[agentID], nil
}

func newTestLogger(t *testing.T) *logger.Logger {
	t.Helper()
	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "console"})
	return log
}

func newDeployer(t *testing.T, basePath string, reader skill.SkillReader, lister skill.InstructionLister) *skill.Deployer {
	t.Helper()
	d, err := skill.New(skill.Config{
		Logger:            newTestLogger(t),
		BasePath:          basePath,
		SkillReader:       reader,
		InstructionLister: lister,
		ProjectSkillDirResolver: func(agentTypeID string) string {
			if agentTypeID == "claude-acp" {
				return ".claude/skills"
			}
			return ""
		},
	})
	if err != nil {
		t.Fatalf("skill.New: %v", err)
	}
	return d
}

// TestDeploy_Local_WritesSkillsAndInstructions verifies the local
// strategy writes SKILL.md to the worktree under the agent's project
// skill dir and instruction files to the host runtime tree.
func TestDeploy_Local_WritesSkillsAndInstructions(t *testing.T) {
	base := t.TempDir()
	worktree := t.TempDir()
	reader := &fakeSkillReader{skills: map[string]*skill.Skill{
		"sk-foo": {Slug: "sk-foo", Content: "# foo skill", SourceType: "inline"},
	}}
	lister := &fakeInstructionLister{files: map[string][]*skill.InstructionFile{
		"profile-1": {{Filename: "AGENTS.md", Content: "# instructions", IsEntry: true}},
	}}
	d := newDeployer(t, base, reader, lister)

	res, err := d.Deploy(context.Background(), skill.Request{
		Profile: &settingsmodels.AgentProfile{
			ID:       "profile-1",
			AgentID:  "claude-acp",
			SkillIDs: `["sk-foo"]`,
		},
		ExecutorType:  "local_pc",
		WorkspaceID:   "ws-1",
		WorkspacePath: worktree,
	})
	if err != nil {
		t.Fatalf("Deploy: %v", err)
	}

	wantDir := filepath.Join(base, "runtime", "default", "instructions", "profile-1")
	if res.InstructionsDir != wantDir {
		t.Errorf("instructions dir = %q, want %q", res.InstructionsDir, wantDir)
	}
	if data, err := os.ReadFile(filepath.Join(wantDir, "AGENTS.md")); err != nil {
		t.Fatalf("read AGENTS.md: %v", err)
	} else if string(data) != "# instructions" {
		t.Errorf("AGENTS.md content = %q", string(data))
	}
	// Skill lands inside the worktree under the claude project skill dir,
	// prefixed kandev- per spec.
	skillPath := filepath.Join(worktree, ".claude", "skills", "kandev-sk-foo", "SKILL.md")
	if data, err := os.ReadFile(skillPath); err != nil {
		t.Fatalf("read SKILL.md: %v", err)
	} else if string(data) != "# foo skill" {
		t.Errorf("SKILL.md content = %q", string(data))
	}
	// The legacy host runtime layout for skills must NOT be populated.
	if _, err := os.Stat(filepath.Join(base, "runtime", "default", "skills")); !os.IsNotExist(err) {
		t.Errorf("legacy host skills tree should not be created")
	}
}

// TestDeploy_Local_RewritesSiblingRefs verifies relative `./X.md`
// references inside instruction file content are rewritten to
// absolute paths under the materialised instructions dir, so the
// agent can act on them without resolving manually.
func TestDeploy_Local_RewritesSiblingRefs(t *testing.T) {
	base := t.TempDir()
	worktree := t.TempDir()
	lister := &fakeInstructionLister{files: map[string][]*skill.InstructionFile{
		"p1": {
			{Filename: "AGENTS.md", Content: "Read `./HEARTBEAT.md` first.", IsEntry: true},
			{Filename: "HEARTBEAT.md", Content: "Heartbeat checklist."},
		},
	}}
	d := newDeployer(t, base, &fakeSkillReader{}, lister)

	res, err := d.Deploy(context.Background(), skill.Request{
		Profile: &settingsmodels.AgentProfile{
			ID: "p1", AgentID: "claude-acp",
		},
		ExecutorType:  "local_pc",
		WorkspacePath: worktree,
	})
	if err != nil {
		t.Fatalf("Deploy: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(res.InstructionsDir, "AGENTS.md"))
	if err != nil {
		t.Fatalf("read AGENTS.md: %v", err)
	}
	got := string(data)
	wantRef := res.InstructionsDir + "/HEARTBEAT.md"
	if !strings.Contains(got, wantRef) {
		t.Errorf("AGENTS.md should contain absolute sibling ref %q, got %q", wantRef, got)
	}
	if strings.Contains(got, "./HEARTBEAT.md") {
		t.Errorf("AGENTS.md should not contain relative sibling ref, got %q", got)
	}
}

// TestDeploy_Sprites_RewritesSiblingRefsInManifest verifies the
// sprite-bound manifest also has its sibling refs rewritten to the
// sprite-side absolute path before serialisation.
func TestDeploy_Sprites_RewritesSiblingRefsInManifest(t *testing.T) {
	base := t.TempDir()
	lister := &fakeInstructionLister{files: map[string][]*skill.InstructionFile{
		"p1": {
			{Filename: "AGENTS.md", Content: "See ./HEARTBEAT.md."},
			{Filename: "HEARTBEAT.md", Content: "checklist"},
		},
	}}
	d := newDeployer(t, base, &fakeSkillReader{}, lister)

	res, err := d.Deploy(context.Background(), skill.Request{
		Profile: &settingsmodels.AgentProfile{
			ID: "p1", AgentID: "claude-acp",
		},
		ExecutorType: "sprites",
	})
	if err != nil {
		t.Fatalf("Deploy: %v", err)
	}
	raw := res.Metadata[skill.MetadataKeySkillManifestJSON].(string)
	var decoded skill.Manifest
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	var agentsContent string
	for _, instr := range decoded.Instructions {
		if instr.Filename == "AGENTS.md" {
			agentsContent = instr.Content
			break
		}
	}
	wantRef := res.InstructionsDir + "/HEARTBEAT.md"
	if !strings.Contains(agentsContent, wantRef) {
		t.Errorf("manifest AGENTS.md should contain %q, got %q", wantRef, agentsContent)
	}
	if strings.Contains(agentsContent, "./HEARTBEAT.md") {
		t.Errorf("manifest AGENTS.md should not contain relative sibling ref, got %q", agentsContent)
	}
}

// TestDeploy_Docker_WritesSkillsToWorktree verifies the Docker strategy
// writes skills directly into the bind-mounted worktree (no separate
// runtime-dir bind-mount) and emits no kandev_runtime_dir metadata.
func TestDeploy_Docker_WritesSkillsToWorktree(t *testing.T) {
	base := t.TempDir()
	worktree := t.TempDir()
	reader := &fakeSkillReader{skills: map[string]*skill.Skill{
		"sk-foo": {Slug: "sk-foo", Content: "# foo", SourceType: "inline"},
	}}
	d := newDeployer(t, base, reader, &fakeInstructionLister{})

	res, err := d.Deploy(context.Background(), skill.Request{
		Profile: &settingsmodels.AgentProfile{
			ID: "p1", AgentID: "claude-acp",
			SkillIDs: `["sk-foo"]`,
		},
		ExecutorType:  "local_docker",
		WorkspacePath: worktree,
	})
	if err != nil {
		t.Fatalf("Deploy: %v", err)
	}

	if _, ok := res.Metadata["kandev_runtime_dir"]; ok {
		t.Errorf("metadata should not contain kandev_runtime_dir under CWD-injection model")
	}
	skillPath := filepath.Join(worktree, ".claude", "skills", "kandev-sk-foo", "SKILL.md")
	if data, err := os.ReadFile(skillPath); err != nil {
		t.Fatalf("skill file missing in worktree: %v", err)
	} else if string(data) != "# foo" {
		t.Errorf("SKILL.md content = %q", string(data))
	}
}

// TestDeploy_Sprites_SetsManifestJSONMetadata verifies the Sprites
// strategy returns the manifest JSON in metadata, does not write to
// the host, and returns a sprite-side instructions path.
func TestDeploy_Sprites_SetsManifestJSONMetadata(t *testing.T) {
	base := t.TempDir()
	reader := &fakeSkillReader{skills: map[string]*skill.Skill{
		"sk-foo": {Slug: "sk-foo", Content: "# foo", SourceType: "inline"},
	}}
	lister := &fakeInstructionLister{files: map[string][]*skill.InstructionFile{
		"p1": {{Filename: "AGENTS.md", Content: "# inst"}},
	}}
	d := newDeployer(t, base, reader, lister)

	res, err := d.Deploy(context.Background(), skill.Request{
		Profile: &settingsmodels.AgentProfile{
			ID: "p1", AgentID: "claude-acp",
			SkillIDs: `["sk-foo"]`,
		},
		ExecutorType: "sprites",
	})
	if err != nil {
		t.Fatalf("Deploy: %v", err)
	}
	if !strings.HasPrefix(res.InstructionsDir, "/root/.kandev/runtime/") {
		t.Errorf("instructions dir = %q, want sprite-side", res.InstructionsDir)
	}
	raw, ok := res.Metadata[skill.MetadataKeySkillManifestJSON].(string)
	if !ok || raw == "" {
		t.Fatalf("metadata missing skill_manifest_json: %#v", res.Metadata)
	}
	var decoded skill.Manifest
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(decoded.Skills) != 1 || decoded.Skills[0].Slug != "sk-foo" {
		t.Errorf("manifest skills = %+v", decoded.Skills)
	}
	if len(decoded.Instructions) != 1 || decoded.Instructions[0].Filename != "AGENTS.md" {
		t.Errorf("manifest instructions = %+v", decoded.Instructions)
	}
	// Host runtime path must not be populated for sprites.
	if _, err := os.Stat(filepath.Join(base, "runtime")); !os.IsNotExist(err) {
		t.Errorf("host runtime tree should not exist for sprites delivery")
	}
}

// TestDeploy_KanbanProfile_NoSkillsNoOp verifies that a profile
// without SkillIDs / DesiredSkills produces an empty result with no
// filesystem side effects — kanban launches today.
func TestDeploy_KanbanProfile_NoSkillsNoOp(t *testing.T) {
	base := t.TempDir()
	d := newDeployer(t, base, &fakeSkillReader{}, &fakeInstructionLister{})

	res, err := d.Deploy(context.Background(), skill.Request{
		Profile:      &settingsmodels.AgentProfile{ID: "p1", AgentID: "claude-acp"},
		ExecutorType: "local_pc",
	})
	if err != nil {
		t.Fatalf("Deploy: %v", err)
	}
	// instructionsDir is still computed (deterministic path); the
	// runtime caller decides whether to use it. The on-host tree must
	// not contain any files because the manifest is empty.
	if entries, err := os.ReadDir(filepath.Join(base, "runtime")); err == nil && len(entries) > 0 {
		t.Errorf("host runtime tree should be untouched, got %v", entries)
	}
	if res.InstructionsDir == "" {
		t.Errorf("instructionsDir should be deterministic even for empty manifest")
	}
}

// TestDeploy_KanbanProfileWithSkill_DeploysFiles verifies that a
// kanban-flavoured profile (no DesiredSkills, no Role) that the user
// later enriched with SkillIDs gets the same delivery treatment as
// office profiles. This is the "kanban launches deploy too" guarantee
// Wave E ships.
func TestDeploy_KanbanProfileWithSkill_DeploysFiles(t *testing.T) {
	base := t.TempDir()
	worktree := t.TempDir()
	reader := &fakeSkillReader{skills: map[string]*skill.Skill{
		"sk-quality": {Slug: "sk-quality", Content: "# quality skill"},
	}}
	d := newDeployer(t, base, reader, &fakeInstructionLister{})

	_, err := d.Deploy(context.Background(), skill.Request{
		Profile: &settingsmodels.AgentProfile{
			ID:       "kanban-p1",
			AgentID:  "claude-acp",
			SkillIDs: `["sk-quality"]`,
		},
		ExecutorType:  "local_pc",
		WorkspacePath: worktree,
	})
	if err != nil {
		t.Fatalf("Deploy: %v", err)
	}
	skillPath := filepath.Join(worktree, ".claude", "skills", "kandev-sk-quality", "SKILL.md")
	if _, err := os.Stat(skillPath); err != nil {
		t.Errorf("kanban-flavoured profile skill should be deployed: %v", err)
	}
}

// TestDeploy_NilProfile_Errors guards the public surface against
// callers passing nil profiles.
func TestDeploy_NilProfile_Errors(t *testing.T) {
	d := newDeployer(t, t.TempDir(), &fakeSkillReader{}, &fakeInstructionLister{})
	if _, err := d.Deploy(context.Background(), skill.Request{}); err == nil {
		t.Error("expected error for nil profile")
	}
}

// TestDeploy_MergesSkillIDsAndDesiredSkills verifies that both fields
// are consulted and duplicates collapse — preserves office's legacy
// DesiredSkills column while supporting the new merged SkillIDs.
func TestDeploy_MergesSkillIDsAndDesiredSkills(t *testing.T) {
	base := t.TempDir()
	worktree := t.TempDir()
	reader := &fakeSkillReader{skills: map[string]*skill.Skill{
		"sk-a": {Slug: "sk-a", Content: "# a"},
		"sk-b": {Slug: "sk-b", Content: "# b"},
	}}
	d := newDeployer(t, base, reader, &fakeInstructionLister{})

	_, err := d.Deploy(context.Background(), skill.Request{
		Profile: &settingsmodels.AgentProfile{
			ID:            "p1",
			AgentID:       "claude-acp",
			SkillIDs:      `["sk-a", "sk-b"]`,
			DesiredSkills: `["sk-a"]`, // duplicate of SkillIDs[0]
		},
		ExecutorType:  "local_pc",
		WorkspacePath: worktree,
	})
	if err != nil {
		t.Fatalf("Deploy: %v", err)
	}
	for _, slug := range []string{"sk-a", "sk-b"} {
		path := filepath.Join(worktree, ".claude", "skills", "kandev-"+slug, "SKILL.md")
		if _, err := os.Stat(path); err != nil {
			t.Errorf("expected skill %q in worktree: %v", slug, err)
		}
	}
}
