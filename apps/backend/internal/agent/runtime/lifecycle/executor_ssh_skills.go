package lifecycle

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"strings"

	"golang.org/x/crypto/ssh"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle/skill"
)

type sshSkillWorkspace interface {
	WriteFile(context.Context, string, []byte, os.FileMode) error
	RemoveAll(context.Context, string) error
}

// uploadSSHSkillManifest materializes launch-scoped skills after the remote
// task workspace exists and before the remote agent starts. The lifecycle
// deployer cannot write the host worktree because SSH agents run elsewhere.
func uploadSSHSkillManifest(
	ctx context.Context,
	client *ssh.Client,
	taskDir string,
	metadata map[string]interface{},
) error {
	raw := getMetadataString(metadata, MetadataKeySkillManifestJSON)
	if raw == "" {
		return nil
	}
	return materializeSSHSkillManifest(ctx, &sshFileUploader{client: client}, taskDir, raw)
}

func materializeSSHSkillManifest(
	ctx context.Context,
	workspace sshSkillWorkspace,
	taskDir string,
	raw string,
) error {
	if workspace == nil {
		return fmt.Errorf("SSH skill workspace is required")
	}
	taskRoot := path.Clean(strings.TrimSpace(taskDir))
	if taskRoot == "." || !path.IsAbs(taskRoot) {
		return fmt.Errorf("SSH task directory must be absolute")
	}
	var manifest skill.Manifest
	if err := json.Unmarshal([]byte(raw), &manifest); err != nil {
		return fmt.Errorf("unmarshal SSH skill manifest: %w", err)
	}
	projectDir := manifest.ProjectSkillDir
	if projectDir == "" {
		projectDir = skill.DefaultProjectSkillDir
	}
	projectDir, ok := sshSafeRelativePath(projectDir)
	if !ok {
		return fmt.Errorf("SSH skill manifest project directory is unsafe")
	}
	projectRoot := path.Join(taskRoot, projectDir)
	decisionRoot := path.Join(projectRoot, skill.DirName(skill.ReservedDecisionSkillSlug))
	if err := workspace.RemoveAll(ctx, decisionRoot); err != nil {
		return fmt.Errorf("remove stale SSH decision skill: %w", err)
	}
	return materializeSSHManifestSkills(ctx, workspace, projectRoot, manifest.Skills)
}

func materializeSSHManifestSkills(
	ctx context.Context,
	workspace sshSkillWorkspace,
	projectRoot string,
	skills []skill.Skill,
) error {
	claimed := make(map[string]string, len(skills))
	for _, item := range skills {
		if !validSlugRe.MatchString(item.Slug) {
			continue
		}
		dirName := skill.DirName(item.Slug)
		if _, exists := claimed[dirName]; exists {
			continue
		}
		claimed[dirName] = item.Slug
		if err := writeSSHManifestSkill(ctx, workspace, projectRoot, item); err != nil {
			return err
		}
	}
	return nil
}

func writeSSHManifestSkill(
	ctx context.Context,
	workspace sshSkillWorkspace,
	projectRoot string,
	item skill.Skill,
) error {
	root := path.Join(projectRoot, skill.DirName(item.Slug))
	if err := workspace.WriteFile(ctx, path.Join(root, "SKILL.md"), []byte(item.Content), 0o644); err != nil {
		return fmt.Errorf("write SSH skill %q: %w", item.Slug, err)
	}
	for _, support := range item.Files {
		relative, safe := sshSafeRelativePath(support.Path)
		if !safe || relative == "SKILL.md" {
			continue
		}
		if err := workspace.WriteFile(ctx, path.Join(root, relative), []byte(support.Content), 0o644); err != nil {
			return fmt.Errorf("write SSH skill support file %q: %w", support.Path, err)
		}
	}
	return nil
}

func sshSafeRelativePath(value string) (string, bool) {
	if value == "" || strings.Contains(value, "\\") || strings.ContainsRune(value, '\x00') || path.IsAbs(value) {
		return "", false
	}
	cleaned := path.Clean(value)
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", false
	}
	return cleaned, true
}
