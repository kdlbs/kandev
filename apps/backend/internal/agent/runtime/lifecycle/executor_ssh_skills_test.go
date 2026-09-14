package lifecycle

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle/skill"
)

func TestUploadSSHSkillManifest_WritesDecisionSkillToRemoteWorkspace(t *testing.T) {
	remoteWorkspace := t.TempDir()
	server := newFakeSSHServer(t, nil)
	server.enableSFTP()
	manifest, err := json.Marshal(skill.Manifest{
		ProjectSkillDir: ".claude/skills",
		Skills: []skill.Skill{{
			Slug:    skill.ReservedDecisionSkillSlug,
			Content: "---\nname: kandev-step-decision\ndescription: decision\n---\n# decision",
		}},
	})
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}

	if err := uploadSSHSkillManifest(context.Background(), server.dial(t), remoteWorkspace,
		map[string]interface{}{MetadataKeySkillManifestJSON: string(manifest)}); err != nil {
		t.Fatalf("uploadSSHSkillManifest: %v", err)
	}

	path := filepath.Join(remoteWorkspace, ".claude", "skills", "kandev-step-decision", "SKILL.md")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read remote decision skill: %v", err)
	}
	if string(data) != "---\nname: kandev-step-decision\ndescription: decision\n---\n# decision" {
		t.Fatalf("remote decision skill = %q", string(data))
	}
}

func TestUploadSSHSkillManifest_NoSeatRemovesStaleDecisionSkill(t *testing.T) {
	remoteWorkspace := t.TempDir()
	server := newFakeSSHServer(t, nil)
	server.enableSFTP()
	client := server.dial(t)

	seatManifest, err := json.Marshal(skill.Manifest{
		ProjectSkillDir: ".claude/skills",
		Skills:          []skill.Skill{{Slug: skill.ReservedDecisionSkillSlug, Content: "# decision"}},
	})
	if err != nil {
		t.Fatalf("marshal seat manifest: %v", err)
	}
	metadata := map[string]interface{}{MetadataKeySkillManifestJSON: string(seatManifest)}
	if err := uploadSSHSkillManifest(context.Background(), client, remoteWorkspace, metadata); err != nil {
		t.Fatalf("seat uploadSSHSkillManifest: %v", err)
	}
	decisionPath := filepath.Join(remoteWorkspace, ".claude", "skills", "kandev-step-decision", "SKILL.md")
	if _, err := os.Stat(decisionPath); err != nil {
		t.Fatalf("seat decision skill missing: %v", err)
	}

	noSeatManifest, err := json.Marshal(skill.Manifest{ProjectSkillDir: ".claude/skills"})
	if err != nil {
		t.Fatalf("marshal no-seat manifest: %v", err)
	}
	metadata[MetadataKeySkillManifestJSON] = string(noSeatManifest)
	if err := uploadSSHSkillManifest(context.Background(), client, remoteWorkspace, metadata); err != nil {
		t.Fatalf("no-seat uploadSSHSkillManifest: %v", err)
	}
	if _, err := os.Stat(decisionPath); !os.IsNotExist(err) {
		t.Fatalf("no-seat upload left stale decision skill at %s", decisionPath)
	}
}

func TestUploadSSHSkillManifest_NoSeatDoesNotFollowDecisionSkillSymlink(t *testing.T) {
	remoteWorkspace := t.TempDir()
	outsideWorkspace := t.TempDir()
	marker := filepath.Join(outsideWorkspace, "keep.txt")
	if err := os.WriteFile(marker, []byte("must survive"), 0o600); err != nil {
		t.Fatalf("seed outside marker: %v", err)
	}
	decisionRoot := filepath.Join(remoteWorkspace, ".claude", "skills", skill.ReservedDecisionSkillSlug)
	if err := os.MkdirAll(filepath.Dir(decisionRoot), 0o755); err != nil {
		t.Fatalf("create decision skill parent: %v", err)
	}
	if err := os.Symlink(outsideWorkspace, decisionRoot); err != nil {
		t.Skipf("symlinks are unavailable: %v", err)
	}

	server := newFakeSSHServer(t, nil)
	server.enableSFTP()
	manifest, err := json.Marshal(skill.Manifest{ProjectSkillDir: ".claude/skills"})
	if err != nil {
		t.Fatalf("marshal no-seat manifest: %v", err)
	}
	if err := uploadSSHSkillManifest(context.Background(), server.dial(t), remoteWorkspace,
		map[string]interface{}{MetadataKeySkillManifestJSON: string(manifest)}); err != nil {
		t.Fatalf("uploadSSHSkillManifest: %v", err)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("outside marker was removed: %v", err)
	}
	if _, err := os.Lstat(decisionRoot); !os.IsNotExist(err) {
		t.Fatalf("decision symlink still exists or returned unexpected error: %v", err)
	}
}
