package lifecycle

import (
	"bytes"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/kandev/kandev/internal/agent/executor"
	"github.com/kandev/kandev/internal/scriptengine"
)

func resolvePreparerSetupScript(req *EnvPrepareRequest, workspacePath string) string {
	script := strings.TrimSpace(req.SetupScript)
	if script == "" {
		script = defaultPreparerSetupScript(req)
	}
	if script == "" {
		return ""
	}

	metadata := map[string]any{
		MetadataKeyRepositoryPath: req.RepositoryPath,
		MetadataKeyBaseBranch:     req.BaseBranch,
	}
	if req.WorktreeID != "" {
		metadata[MetadataKeyWorktreeID] = req.WorktreeID
	}
	if req.WorktreeBranch != "" {
		metadata[MetadataKeyWorktreeBranch] = req.WorktreeBranch
	}

	worktreeBasePath := ""
	if workspacePath != "" {
		worktreeBasePath = filepath.Dir(workspacePath)
	}

	resolver := scriptengine.NewResolver().
		WithProvider(scriptengine.WorkspaceProvider(workspacePath)).
		WithProvider(scriptengine.RepositoryProvider(metadata, req.Env, getGitRemoteURL, nil)).
		WithProvider(scriptengine.WorktreeProvider(
			worktreeBasePath,
			workspacePath,
			req.WorktreeID,
			req.WorktreeBranch,
			req.BaseBranch,
		))

	return resolver.Resolve(script)
}

func defaultPreparerSetupScript(req *EnvPrepareRequest) string {
	if req.UseWorktree {
		return DefaultPrepareScript("worktree")
	}
	execType := req.ExecutorType
	switch execType {
	case executor.NameStandalone, executor.NameLocal:
		return DefaultPrepareScript("local")
	default:
		return ""
	}
}

func getGitRemoteURL(repoPath string) (string, error) {
	cmd := exec.Command("git", "-C", repoPath, "remote", "get-url", "origin")
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return "", err
	}
	return strings.TrimSpace(out.String()), nil
}
