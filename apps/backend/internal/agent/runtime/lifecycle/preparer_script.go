package lifecycle

import (
	"bytes"
	"context"
	"os/exec"
	"path/filepath"
	"strings"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/executor"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/scriptengine"
)

// prepareScriptLogger is a package-level logger for setup script resolution.
var prepareScriptLogger = logger.Default().WithFields(zap.String("component", "prepare-script"))

func resolvePreparerSetupScript(req *EnvPrepareRequest, workspacePath string) string {
	script := strings.TrimSpace(req.SetupScript)
	if script == "" {
		script = defaultPreparerSetupScript(req)
	}
	if script == "" {
		return ""
	}

	metadata := map[string]any{
		MetadataKeyRepositoryPath:  req.RepositoryPath,
		MetadataKeyBaseBranch:      req.BaseBranch,
		MetadataKeyRepoSetupScript: req.RepoSetupScript,
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

	resolved := resolver.Resolve(script)
	if isScriptEffectivelyEmpty(resolved) {
		prepareScriptLogger.Warn("setup script is comment-only after resolution, skipping",
			zap.String("task_id", req.TaskID),
			zap.String("executor_type", string(req.ExecutorType)),
			zap.Bool("use_worktree", req.UseWorktree),
			zap.Bool("has_explicit_script", strings.TrimSpace(req.SetupScript) != ""))
		return ""
	}
	return resolved
}

// isScriptEffectivelyEmpty returns true when a script contains only a shebang,
// comments, and/or blank lines — i.e. no executable commands. This avoids
// running (and displaying) the default template scripts that are pure comments.
func isScriptEffectivelyEmpty(script string) bool {
	for _, line := range strings.Split(script, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		return false
	}
	return true
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

// runSetupScriptStep executes the resolved setup script as a named prepare step,
// appends the completed step to steps, and returns the updated slice.
// Setup script failures are non-fatal: the error is recorded in the step and
// logged as a warning, but preparation continues.
func runSetupScriptStep(
	ctx context.Context,
	req *EnvPrepareRequest,
	workspacePath, resolvedScript string,
	stepIdx, totalSteps int,
	onProgress PrepareProgressCallback,
	steps []PrepareStep,
	log *logger.Logger,
) []PrepareStep {
	step := beginStep("Run setup script")
	step.Command = setupScriptDisplayCommand(req)
	reportProgress(onProgress, step, stepIdx, totalSteps)
	output, err := runSetupScript(ctx, resolvedScript, workspacePath, req.Env, func(current string) {
		step.Output = current
		reportProgress(onProgress, step, stepIdx, totalSteps)
	})
	step.Output = output
	if err != nil {
		completeStepError(&step, err.Error())
		steps = append(steps, step)
		reportProgress(onProgress, step, stepIdx, totalSteps)
		log.Warn("setup script failed", zap.String("task_id", req.TaskID), zap.Error(err))
	} else {
		completeStepSuccess(&step)
		steps = append(steps, step)
		reportProgress(onProgress, step, stepIdx, totalSteps)
	}
	return steps
}
