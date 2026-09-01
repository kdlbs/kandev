package lifecycle

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"

	corev1 "k8s.io/api/core/v1"

	kubeexecutor "github.com/kandev/kandev/internal/agent/kubernetes"
	"github.com/kandev/kandev/internal/githubauth"
	"github.com/kandev/kandev/internal/scriptengine"
)

var kubernetesEnvironmentKey = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func (r *KubernetesExecutor) bootstrapPod(
	ctx context.Context,
	runtime *kubernetesRuntimeClient,
	req *ExecutorCreateRequest,
	pod *corev1.Pod,
	profile kubeexecutor.ProfileConfig,
	nonce string,
	binary []byte,
) error {
	if len(binary) == 0 {
		return fmt.Errorf("kubernetes lifecycle: resolved agentctl binary is empty")
	}
	if err := kubernetesWriteFile(ctx, runtime.streams, pod, profile.MainContainer,
		kubernetesAgentctlPath, binary, 0o755); err != nil {
		return fmt.Errorf("kubernetes lifecycle: upload agentctl: %w", err)
	}

	runtimeEnv, err := kubernetesSerializeEnvironment(kubernetesRuntimeEnvironment(req))
	if err != nil {
		return err
	}
	prepareScript, err := kubernetesPrepareScript(req)
	if err != nil {
		return fmt.Errorf("kubernetes lifecycle: resolve prepare script: %w", err)
	}
	if err := kubernetesWriteBootstrapFiles(ctx, runtime.streams, pod, profile.MainContainer,
		runtimeEnv, []byte(prepareScript)); err != nil {
		return fmt.Errorf("kubernetes lifecycle: upload runtime config: %w", err)
	}

	authEnv := kubernetesAuthEnvironment(req.Env, nonce)
	authData, err := kubernetesSerializeEnvironment(authEnv)
	if err != nil {
		return err
	}
	if err := kubernetesWriteFile(ctx, runtime.streams, pod, profile.MainContainer,
		kubernetesAuthEnvPath, authData, 0o600); err != nil {
		return fmt.Errorf("kubernetes lifecycle: upload auth config: %w", err)
	}
	if err := r.materializeKubernetesRemoteFiles(ctx, runtime, req, pod, profile.MainContainer); err != nil {
		return err
	}
	if err := kubernetesWriteFile(ctx, runtime.streams, pod, profile.MainContainer,
		kubernetesStartPath, nil, 0o600); err != nil {
		return fmt.Errorf("kubernetes lifecycle: signal bootstrap: %w", err)
	}
	return nil
}

func kubernetesRuntimeEnvironment(req *ExecutorCreateRequest) map[string]string {
	return map[string]string{
		"AGENTCTL_LISTEN_HOST":        kubernetesControlHost,
		"AGENTCTL_PORT":               strconv.Itoa(int(kubeexecutor.DefaultAgentctlPort)),
		"AGENTCTL_INSTANCE_PORT_BASE": strconv.Itoa(dockerAgentctlInstancePortBase),
		"AGENTCTL_INSTANCE_PORT_MAX":  strconv.Itoa(dockerAgentctlInstancePortMax),
		kubernetesEnvInstanceID:       req.InstanceID,
		kubernetesEnvSessionID:        req.SessionID,
		kubernetesEnvTaskID:           req.TaskID,
		kubernetesEnvEnvironmentID:    req.TaskEnvironmentID,
		kubernetesEnvAgentProfile:     req.OfficeAgentProfileID,
		kubernetesEnvExecutionProfile: req.AgentProfileID,
	}
}

func kubernetesAuthEnvironment(input map[string]string, nonce string) map[string]string {
	values := make(map[string]string, len(input)+2)
	for key, value := range input {
		if !isKubernetesOwnedControlEnvironmentKey(key) {
			values[key] = value
		}
	}
	values["AGENTCTL_BOOTSTRAP_NONCE"] = nonce
	values["HOME"] = kubernetesAuthHomePath
	if hasManagedGitCredentialBrokerEnv(input) {
		values[githubauth.CredentialHelperPathEnv] = kubernetesAgentctlPath
	}
	return values
}

func isKubernetesOwnedControlEnvironmentKey(key string) bool {
	if key == "HOME" || key == githubauth.CredentialHelperPathEnv || strings.HasPrefix(key, "AGENTCTL_") {
		return true
	}
	switch key {
	case kubernetesEnvInstanceID,
		"KANDEV_EXECUTION_ID",
		kubernetesEnvTaskID,
		kubernetesEnvSessionID,
		kubernetesEnvEnvironmentID,
		kubernetesEnvAgentProfile,
		kubernetesEnvExecutionProfile,
		"KANDEV_OFFICE_AGENT_PROFILE_ID":
		return true
	default:
		return false
	}
}

func kubernetesWriteFile(
	ctx context.Context,
	streams *kubeexecutor.StreamOperations,
	pod *corev1.Pod,
	container, destination string,
	data []byte,
	mode os.FileMode,
) error {
	command := kubernetesWriteFileCommand(destination, mode, len(data) > 0)
	var stdin io.Reader
	if len(data) > 0 {
		stdin = bytes.NewReader(data)
	}
	return streams.Exec(ctx, kubeexecutor.ExecRequest{
		Namespace: pod.Namespace, Pod: pod.Name, Container: container,
		Command: []string{"sh", "-c", command}, Stdin: stdin,
		Stdout: io.Discard, Stderr: io.Discard,
	})
}

func kubernetesWriteFileCommand(destination string, mode os.FileMode, hasData bool) string {
	directory := path.Dir(destination)
	writeCommand := ":"
	if hasData {
		writeCommand = `cat > "$temporary"`
	}
	return fmt.Sprintf(`set -efu
umask 077
directory=%s
destination=%s
current=
old_ifs=$IFS
IFS=/
set -- $directory
IFS=$old_ifs
for component in "$@"; do
  [ -n "$component" ] || continue
  current="$current/$component"
  [ ! -L "$current" ] || exit 73
  if [ -e "$current" ]; then
    [ -d "$current" ] || exit 73
  else
    mkdir "$current"
  fi
done
[ ! -L "$destination" ] || exit 73
[ ! -d "$destination" ] || exit 73
temporary=$(mktemp "$directory/.kandev-upload.XXXXXX")
trap 'rm -f "$temporary"' EXIT HUP INT TERM
%s
chmod %04o "$temporary"
mv -f "$temporary" "$destination"
trap - EXIT HUP INT TERM`, shellQuote(directory), shellQuote(destination), writeCommand, mode.Perm())
}

func kubernetesWriteBootstrapFiles(
	ctx context.Context,
	streams *kubeexecutor.StreamOperations,
	pod *corev1.Pod,
	container string,
	runtimeEnv, prepare []byte,
) error {
	command := fmt.Sprintf(
		"set -eu; umask 077; mkdir -p /opt/kandev %s; chmod 0700 %s; "+
			"dd bs=1 count=%d of=%s 2>/dev/null; dd bs=1 count=%d of=%s 2>/dev/null; "+
			"chmod 0600 %s; chmod 0700 %s",
		shellQuote(kubernetesAuthHomePath), shellQuote(kubernetesAuthHomePath),
		len(runtimeEnv), shellQuote(kubernetesRuntimeEnvPath), len(prepare), shellQuote(kubernetesPreparePath),
		shellQuote(kubernetesRuntimeEnvPath), shellQuote(kubernetesPreparePath),
	)
	data := append(append(make([]byte, 0, len(runtimeEnv)+len(prepare)), runtimeEnv...), prepare...)
	return streams.Exec(ctx, kubeexecutor.ExecRequest{
		Namespace: pod.Namespace, Pod: pod.Name, Container: container,
		Command: []string{"sh", "-c", command}, Stdin: bytes.NewReader(data),
		Stdout: io.Discard, Stderr: io.Discard,
	})
}

func kubernetesSerializeEnvironment(values map[string]string) ([]byte, error) {
	keys := make([]string, 0, len(values))
	for key := range values {
		if !kubernetesEnvironmentKey.MatchString(key) {
			return nil, fmt.Errorf("kubernetes lifecycle: invalid environment key %q", key)
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var output strings.Builder
	for _, key := range keys {
		output.WriteString(key)
		output.WriteByte('=')
		output.WriteString(shellQuote(values[key]))
		output.WriteByte('\n')
	}
	return []byte(output.String()), nil
}

func kubernetesPrepareScript(req *ExecutorCreateRequest) (string, error) {
	script := getMetadataString(req.Metadata, MetadataKeySetupScript)
	if script == "" {
		script = DefaultPrepareScript("k8s")
	}
	if script == "" {
		return ":\n", nil
	}
	script += KandevBranchCheckoutPostlude()
	if binding, ok := req.RemoteContributions[""]; ok {
		addition, err := scriptengine.RemoteContributionSetupScript(&binding)
		if err != nil {
			return "", err
		}
		script += addition
	}
	if destination, ok := req.ContributionDestinations[""]; ok {
		addition, err := scriptengine.ContributionDestinationSetupScript(&destination)
		if err != nil {
			return "", err
		}
		script += addition
	}
	installScripts := []string(nil)
	if req.AgentConfig != nil && strings.TrimSpace(req.AgentConfig.InstallScript()) != "" {
		installScripts = append(installScripts, req.AgentConfig.InstallScript())
	}
	resolver := scriptengine.NewResolver().
		WithStatic(map[string]string{kubernetesRepositoryCloneURL: shellQuote("")}).
		WithProvider(scriptengine.WorkspaceProvider(kubernetesWorkspacePath)).
		WithProvider(scriptengine.AgentctlProviderWithOptions(
			int(kubeexecutor.DefaultAgentctlPort),
			kubernetesWorkspacePath,
			scriptengine.AgentctlProviderOptions{BinaryPath: kubernetesAgentctlPath, Start: false},
		)).
		WithProvider(scriptengine.GitIdentityProvider(req.Metadata)).
		WithProvider(scriptengine.GitHubAuthProvider(req.Env)).
		WithProvider(scriptengine.AgentInstallProvider(installScripts)).
		WithProvider(scriptengine.WorktreeProvider(
			"", kubernetesWorkspacePath,
			getMetadataString(req.Metadata, MetadataKeyWorktreeID),
			getMetadataString(req.Metadata, MetadataKeyWorktreeBranch),
			getMetadataString(req.Metadata, MetadataKeyBaseBranch),
		)).
		WithProvider(scriptengine.RepositoryProvider(
			req.Metadata, req.Env, getGitRemoteURL, injectGitHubTokenIntoCloneURL,
		))
	return resolver.Resolve(script), nil
}
