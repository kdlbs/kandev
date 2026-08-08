package lifecycle

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"go.uber.org/zap"
	"golang.org/x/crypto/ssh"

	"github.com/kandev/kandev/internal/scriptengine"
)

const (
	sshPrepareTimeout        = 10 * time.Minute
	sshCleanupTimeout        = 60 * time.Second
	sshPrepareOutputMaxLines = 20
)

func cloneSSHMetadata(metadata map[string]interface{}) map[string]interface{} {
	if len(metadata) == 0 {
		return nil
	}
	clone := make(map[string]interface{}, len(metadata))
	for key, value := range metadata {
		clone[key] = value
	}
	return clone
}

func mergeSSHMetadata(base, overlay map[string]interface{}) map[string]interface{} {
	merged := cloneSSHMetadata(base)
	if merged == nil {
		merged = make(map[string]interface{})
	}
	for key, value := range overlay {
		merged[key] = value
	}
	return merged
}

func (r *SSHExecutor) resolvePrepareScript(req *ExecutorCreateRequest, workspacePath, agentctlBin string) (string, error) {
	if req == nil {
		return "", fmt.Errorf("ssh: prepare request is required")
	}
	script := strings.TrimSpace(getMetadataString(req.Metadata, MetadataKeySetupScript))
	if script == "" {
		script = DefaultPrepareScript("ssh")
	}
	if script == "" {
		return "", nil
	}
	script += KandevBranchCheckoutPostlude()
	if binding, ok := req.RemoteContributions[""]; ok {
		targetURL := getMetadataString(req.Metadata, "repository_clone_url")
		if targetURL == "" {
			return "", fmt.Errorf("ssh: remote contribution target has no clone URL")
		}
		script += "\n" + sshRemoteContributionScript(workspacePath, targetURL, &binding)
	}
	return r.resolveSSHScript(req, workspacePath, agentctlBin, script)
}

func (r *SSHExecutor) resolveSSHScript(req *ExecutorCreateRequest, workspacePath, agentctlBin, script string) (string, error) {
	if req == nil {
		return "", fmt.Errorf("ssh: script request is required")
	}
	resolver := scriptengine.NewResolver().
		WithStatic(map[string]string{
			"repository.clone_url":    "''",
			"kandev.agentctl.port":    "",
			"kandev.agentctl.install": "",
			"kandev.agentctl.start":   "",
		}).
		WithProvider(scriptengine.WorkspaceProvider(workspacePath)).
		WithProvider(scriptengine.GitIdentityProvider(req.Metadata)).
		WithProvider(scriptengine.GitHubAuthProvider(req.Env)).
		WithProvider(scriptengine.AgentInstallProvider(r.collectAgentInstallScripts(req))).
		WithProvider(scriptengine.WorktreeProvider(
			workspacePath,
			workspacePath,
			getMetadataString(req.Metadata, MetadataKeyWorktreeID),
			getMetadataString(req.Metadata, MetadataKeyWorktreeBranch),
			getMetadataString(req.Metadata, MetadataKeyBaseBranch),
		)).
		WithProvider(scriptengine.RepositoryProvider(req.Metadata, nil, getGitRemoteURL, nil))

	resolved := resolver.Resolve(script)
	if strings.Contains(resolved, "{{") {
		return "", fmt.Errorf("ssh: unresolved prepare script placeholder")
	}
	_ = agentctlBin // agentctl placeholders are intentionally disabled for SSH scripts.
	return strings.TrimSpace(resolved), nil
}

func (r *SSHExecutor) collectAgentInstallScripts(req *ExecutorCreateRequest) []string {
	if r.agentList == nil || req == nil {
		return nil
	}
	ids := map[string]bool{}
	if req.AgentConfig != nil {
		ids[req.AgentConfig.ID()] = true
	}
	addMethodAgentIDs(ids, req.Metadata, "remote_credentials", func(raw string) []string {
		var values []string
		if json.Unmarshal([]byte(raw), &values) == nil {
			return values
		}
		return nil
	})
	addMethodAgentIDs(ids, req.Metadata, "remote_auth_secrets", func(raw string) []string {
		var values map[string]string
		if json.Unmarshal([]byte(raw), &values) == nil {
			keys := make([]string, 0, len(values))
			for key := range values {
				keys = append(keys, key)
			}
			return keys
		}
		return nil
	})
	var scripts []string
	for _, agent := range r.agentList.ListEnabled() {
		if ids[agent.ID()] && strings.TrimSpace(agent.InstallScript()) != "" {
			scripts = append(scripts, agent.InstallScript())
		}
	}
	return scripts
}

func addMethodAgentIDs(ids map[string]bool, metadata map[string]interface{}, key string, decode func(string) []string) {
	raw := getMetadataString(metadata, key)
	for _, methodID := range decode(raw) {
		if agentID := extractAgentID(methodID); agentID != "" {
			ids[agentID] = true
		}
	}
}

func (r *SSHExecutor) runPrepareScript(ctx context.Context, client *ssh.Client, taskDir string, req *ExecutorCreateRequest, platform SSHRemotePlatform, agentctlBin string) error {
	script, err := r.resolvePrepareScript(req, taskDir, agentctlBin)
	if err != nil {
		return err
	}
	if script == "" {
		return nil
	}
	env := sshRemoteContributionEnv(req, agentctlBin)
	envScript, err := buildSSHEnvInitScript(env)
	if err != nil {
		return fmt.Errorf("ssh: prepare script environment: %w", err)
	}
	r.report(req.OnProgress, "Running prepare script", PrepareStepRunning, "")
	stepCtx, cancel := context.WithTimeout(ctx, sshPrepareTimeout)
	defer cancel()
	shell := sshShellForRemote(req.Metadata, platform)
	command := WrapLoginShell(shell, sshScriptWithEnvironment(script))
	stdout, stderr, runErr := runSSHCommandStdin(stepCtx, client, command, strings.NewReader(envScript))
	output := redactSSHScriptOutput(stdout+"\n"+stderr, env)
	if runErr != nil {
		r.report(req.OnProgress, "Running prepare script", PrepareStepFailed, output)
		return fmt.Errorf("ssh: prepare script failed")
	}
	r.report(req.OnProgress, "Running prepare script", PrepareStepCompleted, output)
	return nil
}

func sshScriptWithEnvironment(script string) string {
	return "set -ae\n. /dev/stdin\nset +a\nset -e\n" + script
}

func redactSSHScriptOutput(output string, env map[string]string) string {
	for _, value := range env {
		if value != "" {
			output = strings.ReplaceAll(output, value, "<redacted>")
		}
	}
	return lastLines(strings.TrimSpace(output), sshPrepareOutputMaxLines)
}

func sshCommandFailureDetail(stdout, stderr string, err error) string {
	detail := strings.TrimSpace(stdout + "\n" + stderr)
	if err != nil {
		if detail != "" {
			detail += "\n"
		}
		detail += err.Error()
	}
	return detail
}

func normalizeSSHRemotePath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	trimmed := strings.TrimRight(path, "/")
	if trimmed == "" {
		return "/"
	}
	return trimmed
}

func (r *SSHExecutor) verifyPrimaryCheckout(ctx context.Context, client *ssh.Client, taskDir string, req *ExecutorCreateRequest, platform SSHRemotePlatform) error {
	targetURL := getMetadataString(req.Metadata, "repository_clone_url")
	if targetURL == "" {
		return nil
	}
	shell := sshShellForRemote(req.Metadata, platform)
	canonicalTaskDir, taskDirStderr, taskDirErr := runSSHCommand(ctx, client, WrapLoginShell(shell, "cd "+shellQuote(taskDir)+" && pwd -P"))
	if taskDirErr != nil {
		r.report(req.OnProgress, "Verifying primary checkout", PrepareStepFailed, sshCommandFailureDetail(canonicalTaskDir, taskDirStderr, taskDirErr))
		return fmt.Errorf("ssh: primary repository checkout is missing")
	}
	root, rootStderr, rootErr := runSSHCommand(ctx, client, WrapLoginShell(shell, "git -C "+shellQuote(taskDir)+" rev-parse --show-toplevel"))
	if rootErr != nil || normalizeSSHRemotePath(root) != normalizeSSHRemotePath(canonicalTaskDir) {
		detail := sshCommandFailureDetail(root, rootStderr, rootErr)
		if rootErr == nil {
			detail = fmt.Sprintf("checkout root %q does not match task directory %q", strings.TrimSpace(root), strings.TrimSpace(canonicalTaskDir))
		}
		r.report(req.OnProgress, "Verifying primary checkout", PrepareStepFailed, detail)
		return fmt.Errorf("ssh: primary repository checkout is missing")
	}
	origin, originStderr, originErr := runSSHCommand(ctx, client, WrapLoginShell(shell, "git -C "+shellQuote(taskDir)+" config --get remote.origin.url"))
	if originErr != nil || strings.TrimSpace(origin) != targetURL {
		r.report(req.OnProgress, "Verifying primary checkout", PrepareStepFailed, sshCommandFailureDetail(origin, originStderr, originErr))
		return fmt.Errorf("ssh: primary repository origin does not match configured repository")
	}
	if head, headStderr, headErr := runSSHCommand(ctx, client, WrapLoginShell(shell, "git -C "+shellQuote(taskDir)+" rev-parse --verify HEAD")); headErr != nil {
		r.report(req.OnProgress, "Verifying primary checkout", PrepareStepFailed, sshCommandFailureDetail(head, headStderr, headErr))
		return fmt.Errorf("ssh: primary repository has no checkout")
	}
	r.report(req.OnProgress, "Verifying primary checkout", PrepareStepCompleted, strings.TrimSpace(canonicalTaskDir))
	return nil
}

func (r *SSHExecutor) runCleanupScript(ctx context.Context, client *ssh.Client, taskDir string, metadata map[string]interface{}, env map[string]string, platform SSHRemotePlatform, instanceID, reason string) error {
	script := strings.TrimSpace(getMetadataString(metadata, MetadataKeyCleanupScript))
	if script == "" {
		return nil
	}
	if taskDir == "" {
		return fmt.Errorf("ssh: cleanup task directory is missing")
	}
	resolved, err := r.resolveSSHScript(&ExecutorCreateRequest{Metadata: metadata, Env: env}, taskDir, "", script)
	if err != nil {
		return err
	}
	envScript, err := buildSSHEnvInitScript(env)
	if err != nil {
		return err
	}
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), sshCleanupTimeout)
	defer cancel()
	shell := sshShellForRemote(metadata, platform)
	stdout, stderr, runErr := runSSHCommandStdin(cleanupCtx, client, WrapLoginShell(shell, sshScriptWithEnvironment(resolved)), strings.NewReader(envScript))
	if runErr != nil {
		r.logger.Warn("ssh cleanup script failed",
			zap.String("instance_id", instanceID), zap.String("reason", reason),
			zap.String("output", redactSSHScriptOutput(stdout+"\n"+stderr, env)), zap.Error(runErr))
		return fmt.Errorf("ssh: cleanup script failed")
	}
	r.logger.Debug("ssh cleanup script completed", zap.String("instance_id", instanceID), zap.String("reason", reason))
	return nil
}
