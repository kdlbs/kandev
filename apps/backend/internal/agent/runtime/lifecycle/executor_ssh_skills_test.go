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
