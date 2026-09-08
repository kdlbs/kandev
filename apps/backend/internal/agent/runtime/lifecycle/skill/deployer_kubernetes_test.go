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
)

func TestDeployKubernetesReturnsRemoteManifestWithoutHostWrites(t *testing.T) {
	base := t.TempDir()
	reader := &fakeSkillReader{skills: map[string]*skill.Skill{
		"sk-foo": {Slug: "sk-foo", Content: "# foo", SourceType: "inline"},
	}}
	lister := &fakeInstructionLister{files: map[string][]*skill.InstructionFile{
		"p1": {{Filename: "AGENTS.md", Content: "# inst"}},
	}}
	deployer := newDeployer(t, base, reader, lister)

	result, err := deployer.Deploy(context.Background(), skill.Request{
		Profile: &settingsmodels.AgentProfile{
			ID: "p1", AgentID: "claude-acp", SkillIDs: `["sk-foo"]`,
		},
		ExecutorType: "k8s",
	})
	if err != nil {
		t.Fatalf("Deploy: %v", err)
	}
	if !strings.HasPrefix(result.InstructionsDir, "/opt/kandev/runtime/") {
		t.Errorf("instructions dir = %q, want Kubernetes runtime volume", result.InstructionsDir)
	}
	raw, ok := result.Metadata[skill.MetadataKeySkillManifestJSON].(string)
	if !ok || raw == "" {
		t.Fatalf("metadata missing skill_manifest_json: %#v", result.Metadata)
	}
	var decoded skill.Manifest
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		t.Fatalf("unmarshal manifest: %v", err)
	}
	if len(decoded.Skills) != 1 || len(decoded.Instructions) != 1 {
		t.Fatalf("manifest = %#v, want one skill and one instruction", decoded)
	}
	if _, err := os.Stat(filepath.Join(base, "runtime")); !os.IsNotExist(err) {
		t.Errorf("host runtime tree should not exist for Kubernetes delivery")
	}
}

func TestDeployUnknownExecutorFailsClosed(t *testing.T) {
	base := t.TempDir()
	worktree := t.TempDir()
	reader := &fakeSkillReader{skills: map[string]*skill.Skill{
		"sk-foo": {Slug: "sk-foo", Content: "# foo", SourceType: "inline"},
	}}
	deployer := newDeployer(t, base, reader, &fakeInstructionLister{})

	result, err := deployer.Deploy(context.Background(), skill.Request{
		Profile: &settingsmodels.AgentProfile{
			ID: "p1", AgentID: "claude-acp", SkillIDs: `["sk-foo"]`,
		},
		ExecutorType:  "future_executor",
		WorkspacePath: worktree,
	})
	if err != nil {
		t.Fatalf("Deploy: %v", err)
	}
	if result.InstructionsDir != "" || len(result.Metadata) != 0 {
		t.Fatalf("result = %#v, want empty fail-closed result", result)
	}
	if _, err := os.Stat(filepath.Join(worktree, ".claude", "skills", "kandev-sk-foo")); !os.IsNotExist(err) {
		t.Fatal("unknown executor wrote skills into host worktree")
	}
}
