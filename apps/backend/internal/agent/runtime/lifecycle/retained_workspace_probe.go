package lifecycle

import (
	"context"
	"fmt"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
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
			git+" rev-parse --verify HEAD >/dev/null",
			"origin=$("+git+" remote get-url origin)",
			`[ "${origin%.git}" = `+shellQuote(strings.TrimSuffix(repository.RepositoryURL, ".git"))+" ]")
		branch := repository.CheckoutBranch
		if branch == "" {
			branch = repository.BaseBranch
		}
		if branch != "" {
			checks = append(checks, "branch=$("+git+" symbolic-ref --short HEAD)", `[ "$branch" = `+shellQuote(branch)+" ]")
		}
	}
	return strings.Join(checks, "\n")
}

func verifyRetainedLaunchWorkspace(ctx context.Context, sessionID string, instance *ExecutorInstance, repositories []WorkspaceRepositoryMaterialization) error {
	if len(repositories) == 0 {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
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
			_ = instance.Client.StopProcess(cleanup, id)
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
