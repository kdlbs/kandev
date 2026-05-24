package lifecycle

import (
	"context"
	"fmt"
	"strings"

	"go.uber.org/zap"

	client "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/worktree/copyfiles"
)

// shipRemoteCopyfilesForLaunch fires runRemoteCopyfiles once for the
// single-repo top-level spec, or once per RepoLaunchSpec for multi-repo
// launches. Each invocation gets its own progressRecorder.Callback so
// the step ends up at the recorder's current tail without the
// double-offset bug that prompted CodeRabbit's "double-offset in remote
// copy-files progress indexing" feedback.
func shipRemoteCopyfilesForLaunch(
	ctx context.Context,
	log *logger.Logger,
	req *LaunchRequest,
	cli *client.Client,
	runtimeProgress PrepareProgressCallback,
	progressRecorder *prepareProgressRecorder,
) {
	type repoCopy struct {
		sourceRepoPath string
		copyFilesSpec  string
		repoSubpath    string
	}
	var jobs []repoCopy
	if specs := req.Repositories; len(specs) > 0 {
		for _, spec := range specs {
			jobs = append(jobs, repoCopy{
				sourceRepoPath: spec.RepositoryPath,
				copyFilesSpec:  spec.CopyFiles,
				repoSubpath:    spec.RepoName,
			})
		}
	} else if req.RepositoryPath != "" {
		jobs = append(jobs, repoCopy{
			sourceRepoPath: req.RepositoryPath,
			copyFilesSpec:  req.CopyFiles,
			repoSubpath:    "", // single-repo: write at workspace root
		})
	}
	for _, j := range jobs {
		if strings.TrimSpace(j.copyFilesSpec) == "" {
			continue
		}
		// Create a fresh recorder callback at the current tail per job so
		// passing StepIndex=0 / TotalSteps=1 lands at the right absolute
		// index regardless of how many steps the previous job appended.
		var jobProgress PrepareProgressCallback
		if runtimeProgress != nil {
			jobProgress = progressRecorder.Callback(progressRecorder.Len())
		}
		runRemoteCopyfiles(ctx, log, remoteCopyfilesRequest{
			SourceRepoPath: j.sourceRepoPath,
			CopyFilesSpec:  j.copyFilesSpec,
			RepoSubpath:    j.repoSubpath,
			Client:         cli,
			OnProgress:     jobProgress,
			StepIndex:      0,
			TotalSteps:     1,
		})
	}
}

// remoteCopyfilesRequest carries the inputs runRemoteCopyfiles needs.
// Bundled so the call site in manager_launch.go stays readable —
// remote-copy is a side action after CreateInstance, not part of the
// executor's prepare pipeline.
type remoteCopyfilesRequest struct {
	SourceRepoPath string
	CopyFilesSpec  string
	RepoSubpath    string // optional: per-repo subdir under workspace (multi-repo)
	Client         *client.Client
	OnProgress     PrepareProgressCallback
	StepIndex      int
	TotalSteps     int
}

// runRemoteCopyfiles plans copy_files entries from the host source repo,
// ships them via agentctl, and emits a single "Copy N ignored files"
// prepare step matching the worktree path's naming. Best-effort: a
// failure logs + emits a warning on the step but never aborts the
// launch (parallels worktree.Manager.copyConfiguredFiles).
//
// Used by remote executors (Docker, Sprites) whose containers clone
// their own workspace and therefore can't receive copy_files seeding
// via the host-side worktree path.
func runRemoteCopyfiles(ctx context.Context, log *logger.Logger, req remoteCopyfilesRequest) {
	if req.Client == nil || strings.TrimSpace(req.CopyFilesSpec) == "" || req.SourceRepoPath == "" {
		return
	}
	patterns := copyfiles.Parse(req.CopyFilesSpec)
	if len(patterns) == 0 {
		return
	}

	entries, planWarnings, err := copyfiles.Plan(ctx, req.SourceRepoPath, patterns, zapOrNil(log))
	if err != nil {
		log.Warn("remote-copyfiles: plan failed",
			zap.String("source", req.SourceRepoPath),
			zap.Error(err))
		if req.OnProgress != nil {
			req.OnProgress(buildCopyFilesStep(nil,
				append(planWarnings, "plan failed: "+err.Error()), req.RepoSubpath),
				req.StepIndex, req.TotalSteps)
		}
		return
	}

	var copied []string
	var shipWarnings []string
	if len(entries) > 0 {
		resp, err := req.Client.CopyFiles(ctx, req.RepoSubpath, entries)
		if err != nil {
			log.Warn("remote-copyfiles: ship failed",
				zap.String("repo_subpath", req.RepoSubpath),
				zap.Error(err))
			shipWarnings = append(shipWarnings, "ship failed: "+err.Error())
		}
		if resp != nil {
			copied = resp.Copied
			shipWarnings = append(shipWarnings, resp.Warnings...)
		}
	}

	allWarnings := append(planWarnings, shipWarnings...) //nolint:gocritic // intentional new slice
	if len(copied) == 0 && len(allWarnings) == 0 {
		return
	}
	if req.OnProgress == nil {
		return
	}
	req.OnProgress(buildCopyFilesStep(copied, allWarnings, req.RepoSubpath), req.StepIndex, req.TotalSteps)
}

// buildCopyFilesStep returns the "Copy N ignored files" prepare step
// shared by the worktree (host-side) and remote (Docker/Sprites) paths.
// Centralised so the user-facing wording stays in lockstep — the only
// difference between the two callers is when in the launch pipeline the
// step gets emitted.
func buildCopyFilesStep(copied []string, warnings []string, repoLabel string) PrepareStep {
	const (
		nameZero = "Copy ignored files"
		nameOne  = "Copy 1 ignored file"
	)
	var name string
	switch len(copied) {
	case 0:
		name = nameZero
	case 1:
		name = nameOne
	default:
		name = fmt.Sprintf("Copy %d ignored files", len(copied))
	}
	if repoLabel != "" {
		name = fmt.Sprintf("%s (%s)", name, repoLabel)
	}
	step := beginStep(name)
	if len(copied) > 0 {
		step.Output = strings.Join(copied, "\n")
	}
	if len(warnings) > 0 {
		step.Warning = warnings[0]
		if len(warnings) > 1 {
			step.WarningDetail = strings.Join(warnings, "\n")
		}
	}
	completeStepSuccess(&step)
	return step
}

// zapOrNil returns the underlying *zap.Logger or nil when log is nil.
// Plan/WriteEntries accept a nil logger.
func zapOrNil(log *logger.Logger) *zap.Logger {
	if log == nil {
		return nil
	}
	return log.Zap()
}
