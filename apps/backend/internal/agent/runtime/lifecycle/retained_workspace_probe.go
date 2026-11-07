package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/common/subproc"
)

const retainedProbeResultPrefix = "kandev-retained-result-"

func retainedProbeResult(output []agentctl.ProcessOutputChunk, marker string) *int {
	var stdout strings.Builder
	for _, chunk := range output {
		if chunk.Stream == "stdout" {
			stdout.WriteString(chunk.Data)
		}
	}
	for _, line := range strings.Split(stdout.String(), "\n") {
		if raw, ok := strings.CutPrefix(line, marker); ok {
			code, err := strconv.Atoi(raw)
			if err == nil && code >= 0 && code <= 255 {
				return &code
			}
		}
	}
	return nil
}

// Retained checkouts may contain unpublished commits. Verify their identity
// without cloning, fetching, checking out, or changing any Git refs.
func retainedWorkspaceProbeCommand(root string, repositories []WorkspaceRepositoryMaterialization) string {
	checks := []string{"set -eu"}
	for _, repository := range repositories {
		directory := path.Join(root, repository.Destination)
		git := "git --no-optional-locks -C " + shellQuote(directory)
		checks = append(checks,
			"test -e "+shellQuote(path.Join(directory, ".git")),
			git+" rev-parse --verify "+shellQuote("HEAD^{commit}")+" >/dev/null",
			"origin=$("+git+" config --local --get remote.origin.url)",
			`[ "${origin%.git}" = `+shellQuote(strings.TrimSuffix(repository.RepositoryURL, ".git"))+" ]")
		if binding := repository.RemoteContribution; binding != nil {
			remoteName := binding.ContributionRemoteName()
			remoteURL := shellQuote(strings.TrimSuffix(binding.SourceRepository.RemoteURL, ".git"))
			upstream := shellQuote(remoteName + "/" + binding.HeadBranch)
			checks = append(checks,
				"branch=$("+git+" symbolic-ref --short HEAD)",
				`[ -n "$branch" ]`,
				"contribution_origin=$("+git+" config --local --get "+shellQuote("remote."+remoteName+".url")+")",
				`[ "${contribution_origin%.git}" = `+remoteURL+" ]",
				"upstream=$("+git+" rev-parse --abbrev-ref --symbolic-full-name "+shellQuote("@{upstream}")+")",
				`[ "$upstream" = `+upstream+" ]",
				git+" merge-base --is-ancestor "+shellQuote(binding.HeadSHA)+" HEAD")
		} else {
			branch := repository.CheckoutBranch
			if branch == "" {
				branch = repository.BaseBranch
			}
			if branch != "" {
				checks = append(checks, "branch=$("+git+" symbolic-ref HEAD)", `[ "$branch" = `+shellQuote("refs/heads/"+branch)+" ]")
			}
		}
	}
	return strings.Join(checks, "\n")
}

func verifyRetainedLaunchWorkspace(ctx context.Context, sessionID string, instance *ExecutorInstance, repositories []WorkspaceRepositoryMaterialization) (resultErr error) {
	if len(repositories) == 0 {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	releaseGit, err := subproc.AcquireGit(ctx, subproc.GitLifecycle)
	if err != nil {
		return fmt.Errorf("admit retained repository verification: %w", err)
	}
	defer releaseGit()
	root := instance.WorkspacePath
	if root == "" {
		root = dockerWorkspacePath
	}
	marker := retainedProbeResultPrefix + uuid.NewString() + ":"
	// Agentctl removes exited processes immediately. Keep our probe alive long
	// enough to read its explicit result, then stop only this owned process.
	command := "set +e\n(\n" + retainedWorkspaceProbeCommand(root, repositories) + "\n)\nkandev_probe_exit=$?\nprintf '\\n%s%s\\n' " + shellQuote(marker) + " \"$kandev_probe_exit\"\nsleep 30"
	probe, err := instance.Client.StartProcess(ctx, agentctl.StartProcessRequest{SessionID: sessionID, Kind: "custom", ScriptName: "verify-retained-workspace", WorkingDir: root, Command: command, BufferMaxBytes: 1024})
	if err != nil {
		return fmt.Errorf("start retained repository verification: %w", err)
	}
	id := probe.ID
	defer func() {
		if id != "" {
			cleanup, cancelCleanup := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
			defer cancelCleanup()
			if stopErr := instance.Client.StopProcess(cleanup, id); stopErr != nil {
				resultErr = errors.Join(resultErr, fmt.Errorf("stop retained repository verification: %w", stopErr))
			}
		}
	}()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		if result := retainedProbeResult(probe.Output, marker); result != nil {
			if *result != 0 {
				return fmt.Errorf("retained repositories are missing, corrupt, or differ in origin/branch; workspace preserved for repair")
			}
			return nil
		}
		if probe.ExitCode != nil {
			return fmt.Errorf("retained repository verification ended without a result; workspace preserved for repair")
		}
		if id == "" {
			return fmt.Errorf("retained repository verification returned no process identity")
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("retained repository verification: %w", ctx.Err())
		case <-ticker.C:
			probe, err = instance.Client.GetProcess(ctx, id, true)
			if err != nil {
				return fmt.Errorf("read retained repository verification: %w", err)
			}
		}
	}
}
