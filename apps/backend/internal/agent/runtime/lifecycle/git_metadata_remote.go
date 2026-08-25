package lifecycle

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/kandev/kandev/internal/agent/agents"
	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/worktree"
)

// installAttestedCloneGitMetadataPolicy renders the agent policy only after
// agentctl has attested the clone it can actually see. The lifecycle request
// still contains no host checkout projection; its executor-visible source
// roots are accepted only after the primary root and materialized siblings
// have passed their respective in-executor checks.
func installAttestedCloneGitMetadataPolicy(ctx context.Context, req *ExecutorCreateRequest, instance *ExecutorInstance) error {
	if !requiresCloneGitMetadataPolicy(req) {
		return nil
	}
	if instance == nil || instance.Client == nil || instance.WorkspacePath == "" {
		return unsupportedGitMetadataProjection("clone checkout attestation is unavailable; start a new session with a supported executor")
	}
	checkoutRoots := instance.GitMetadataAttestationRoots
	if len(checkoutRoots) == 0 {
		checkoutRoots = instance.WorkspaceSourceRoots
	}
	policyEnv, err := attestedCloneGitMetadataRuntimeEnv(ctx, req, instance.Client, instance.WorkspacePath, checkoutRoots)
	if err != nil {
		return unsupportedGitMetadataProjection("clone Git metadata policy installation failed; start a new session with a supported executor")
	}
	if instance.Metadata == nil {
		instance.Metadata = make(map[string]interface{})
	}
	instance.Metadata["runtime_env"] = policyEnv
	return nil
}

// attestedCloneGitMetadataRuntimeEnv renders a clone policy exclusively from
// agentctl's final ordered attestation. It is shared by launch and live
// attachment refreshes so neither path can derive a GitDir from a workspace
// string after the executor has materialized the checkout.
func attestedCloneGitMetadataRuntimeEnv(ctx context.Context, req *ExecutorCreateRequest, client *agentctl.Client, workspacePath string, checkoutRoots []string) (map[string]string, error) {
	if !requiresCloneGitMetadataPolicy(req) || client == nil || workspacePath == "" {
		return nil, errors.New("clone checkout attestation is unavailable")
	}
	expectedRoots := append([]string(nil), checkoutRoots...)
	if len(expectedRoots) == 0 {
		expectedRoots = []string{workspacePath}
	}
	approved, err := client.AttestWorkspaceGitMetadata(ctx, expectedRoots)
	if err != nil {
		return nil, errors.New("clone checkout attestation failed")
	}
	metadata, err := remoteRegularGitMetadataFromAttestations(approved, expectedRoots)
	if err != nil {
		return nil, err
	}
	if err := prepareRemoteRegularGitMetadataPolicy(req, metadata...); err != nil {
		return nil, err
	}
	return remoteGitMetadataRuntimeEnv(req)
}

// remoteRegularGitMetadataFromAttestations accepts only the exact ordered
// checkout sequence lifecycle configured for this executor. GitDir values are
// consumed from agentctl's final proof rather than reconstructed from a
// checkout string. Ordering binds each primary or secondary root to the
// corresponding attestation response, so a reordered response cannot move a
// valid GitDir grant between repositories.
func remoteRegularGitMetadataFromAttestations(approved []agentctl.GitMetadataAttestation, expectedRoots []string) ([]remoteRegularGitMetadata, error) {
	if len(approved) == 0 || len(approved) != len(expectedRoots) {
		return nil, errors.New("clone Git metadata attestation set is incomplete")
	}
	expected := make(map[string]struct{}, len(expectedRoots))
	for _, root := range expectedRoots {
		if root == "" {
			return nil, errors.New("clone Git metadata attestation set is invalid")
		}
		if _, duplicate := expected[root]; duplicate {
			return nil, errors.New("clone Git metadata attestation set is invalid")
		}
		expected[root] = struct{}{}
	}
	metadata := make([]remoteRegularGitMetadata, 0, len(approved))
	for index, expectedRoot := range expectedRoots {
		checkout := approved[index]
		if checkout.CheckoutPath != expectedRoot {
			return nil, errors.New("clone Git metadata attestation set is invalid")
		}
		item := remoteRegularGitMetadata{CheckoutPath: checkout.CheckoutPath, GitDir: checkout.GitDir}
		if !validRemoteRegularGitMetadata(item) {
			return nil, errors.New("clone Git metadata attestation set is invalid")
		}
		metadata = append(metadata, item)
	}
	return metadata, nil
}

// remoteRegularGitMetadata describes the only repository layout supported by
// clone-based executors. Their checkout is created inside a task-owned remote
// workspace, so there is no source checkout or linked-worktree administration
// to expose. A linked .git pointer on a remote host is deliberately rejected:
// host projections cannot authorize paths on another machine.
type remoteRegularGitMetadata struct {
	CheckoutPath string
	GitDir       string
	CurrentRef   string
}

// validateRemoteGitMetadataRequest is the pre-launch half of clone-based
// enforcement. The actual paths are discovered after cloning on the remote
// host, but the agent renderer and the request shape can be attested before
// provisioning an agent process.
func validateRemoteGitMetadataRequest(req *ExecutorCreateRequest) error {
	if req == nil {
		return unsupportedGitMetadataProjection("remote Git metadata policy is unavailable; start a new session with a supported executor")
	}
	if len(req.GitMetadataProjections) > 1 {
		return unsupportedGitMetadataProjection("remote multi-repository Git metadata is not available; use local Docker or standalone Codex, or start a single-repository session")
	}
	// Agents that do not implement FilesystemPolicyAgent (e.g. OpenCodeACP,
	// ClaudeACP, CopilotACP) cannot receive a server-authored filesystem
	// policy overlay. Skip the policy check — in-container Git directory
	// attestation still runs regardless, and there is no policy to enforce.
	if _, ok := req.AgentConfig.(agents.FilesystemPolicyAgent); !ok {
		return nil
	}
	if _, err := remoteFilesystemPolicyDescriptor(req); err != nil {
		return unsupportedGitMetadataProjection("remote Git metadata requires a compatible Codex ACP filesystem policy; update Codex or choose local Docker")
	}
	return nil
}

func remoteFilesystemPolicyDescriptor(req *ExecutorCreateRequest) (*agents.FilesystemPolicyDescriptor, error) {
	policyAgent, ok := req.AgentConfig.(agents.FilesystemPolicyAgent)
	if !ok {
		return nil, errors.New("agent does not support filesystem policy")
	}
	descriptor, ok := policyAgent.FilesystemPolicyDescriptor()
	if !ok || descriptor == nil || descriptor.ConfigEnvKey == "" || descriptor.Renderer == nil {
		return nil, errors.New("agent filesystem policy is unavailable")
	}
	config, err := codexConfigFromEnvironment(req.Env, descriptor.ConfigEnvKey)
	if err != nil {
		return nil, err
	}
	if hasLegacyCodexSandbox(config) {
		return nil, errors.New("legacy Codex sandbox configuration conflicts with task filesystem policy")
	}
	return descriptor, nil
}

// prepareRemoteRegularGitMetadataPolicy merges a server-authored filesystem
// profile into the environment forwarded to a remote agentctl instance. The
// remote resolver proves GitDir is the task checkout's non-symlink .git
// directory before this function is called.
//
// Agents that do not implement FilesystemPolicyAgent cannot receive a
// server-authored policy overlay — skip the merge while keeping the in-container
// attestation (AttestWorkspaceGitMetadata) intact.
func prepareRemoteRegularGitMetadataPolicy(req *ExecutorCreateRequest, metadata ...remoteRegularGitMetadata) error {
	if _, ok := req.AgentConfig.(agents.FilesystemPolicyAgent); !ok {
		// No filesystem policy to merge — the in-container attestation has
		// already completed successfully.
		return nil
	}
	descriptor, err := remoteFilesystemPolicyDescriptor(req)
	if err != nil {
		return err
	}
	if len(metadata) == 0 {
		return errors.New("remote git metadata is invalid")
	}
	for _, item := range metadata {
		if !validRemoteRegularGitMetadata(item) {
			return errors.New("remote git metadata is invalid")
		}
	}
	config, err := codexConfigFromEnvironment(req.Env, descriptor.ConfigEnvKey)
	if err != nil {
		return err
	}
	policy, err := descriptor.Renderer.Render(remoteRegularGitMetadataFilesystemPolicy(metadata...))
	if err != nil {
		return err
	}
	mergeCodexConfig(config, policy)
	encoded, err := jsonMarshalFilesystemPolicy(config)
	if err != nil {
		return err
	}
	if req.Env == nil {
		req.Env = make(map[string]string)
	}
	req.Env[descriptor.ConfigEnvKey] = encoded
	return nil
}

// jsonMarshalFilesystemPolicy keeps the remote policy implementation from
// exporting a second configuration serializer. It also makes errors returned
// to remote executors independent of any host path.
func jsonMarshalFilesystemPolicy(config map[string]any) (string, error) {
	encoded, err := json.Marshal(config)
	if err != nil {
		return "", fmt.Errorf("encode agent filesystem policy: %w", err)
	}
	return string(encoded), nil
}

func remoteRegularGitMetadataFilesystemPolicy(metadata ...remoteRegularGitMetadata) agents.FilesystemPolicy {
	// A regular clone owns its .git directory. Git writes index.lock and, for
	// detached HEADs, HEAD.lock directly there, so granting only descendants is
	// insufficient. Each entry is independently attested and task-owned.
	rules := []agents.FilesystemPolicyRule{{Path: ":minimal", Access: agents.FilesystemAccessRead}}
	seen := map[string]struct{}{":minimal": {}}
	for _, item := range metadata {
		if _, exists := seen[item.GitDir]; exists {
			continue
		}
		seen[item.GitDir] = struct{}{}
		rules = append(rules, agents.FilesystemPolicyRule{Path: item.GitDir, Access: agents.FilesystemAccessWrite})
	}
	return agents.FilesystemPolicy{Name: codexGitMetadataPolicyName, Rules: rules}
}

func remoteGitMetadataRuntimeEnv(req *ExecutorCreateRequest) (map[string]string, error) {
	descriptor, err := remoteFilesystemPolicyDescriptor(req)
	if err != nil {
		return nil, err
	}
	env := cloneStringMap(req.Env)
	if env == nil {
		env = make(map[string]string)
	}
	env[descriptor.ConfigEnvKey] = req.Env[descriptor.ConfigEnvKey]
	return env, nil
}

func validRemoteRegularGitMetadata(metadata remoteRegularGitMetadata) bool {
	if metadata.CheckoutPath == "" || metadata.GitDir != metadata.CheckoutPath+"/.git" {
		return false
	}
	if strings.ContainsAny(metadata.CheckoutPath, "\x00\n\r") || strings.ContainsAny(metadata.GitDir, "\x00\n\r") {
		return false
	}
	return metadata.CurrentRef == "" || worktree.ValidBranchRef(metadata.CurrentRef)
}

// remoteRegularGitMetadataProbeScript writes canonical checkout path, gitdir,
// and current branch ref as exactly three lines. It is intentionally POSIX sh
// so it works on the supported Linux and macOS SSH targets and Sprites.
func remoteRegularGitMetadataProbeScript(workspacePath string) string {
	return `set -eu
workspace=$(cd ` + shellQuote(workspacePath) + ` && pwd -P)
gitdir=$(git -C "$workspace" rev-parse --absolute-git-dir)
gitdir=$(cd "$gitdir" && pwd -P)
[ "$gitdir" = "$workspace/.git" ]
[ -d "$gitdir" ]
[ ! -L "$gitdir" ]
[ -d "$gitdir/objects" ]
[ ! -L "$gitdir/objects" ]
[ -f "$gitdir/HEAD" ]
[ ! -L "$gitdir/HEAD" ]
for config in "${CODEX_HOME:-$HOME/.codex}/config.toml" "$workspace/.codex/config.toml"; do
  if [ -f "$config" ] && grep -Eq '^[[:space:]]*(sandbox_mode|sandbox_workspace_write)[[:space:]]*=' "$config"; then
    exit 17
  fi
done
ref=$(git -C "$workspace" symbolic-ref -q HEAD || true)
case "$ref" in
  '') ;;
  refs/heads/*) ;;
  *) exit 18 ;;
esac
if [ -n "$ref" ]; then
  path=${ref#refs/heads/}
  case "$path" in ''|/*|*'//'*) exit 18 ;; esac
  oldifs=$IFS
  IFS=/
  set -- $path
  IFS=$oldifs
  for part; do [ "$part" != . ] && [ "$part" != .. ]; done
  refpath="$gitdir/$ref"
  logpath="$gitdir/logs/$ref"
  [ ! -L "$gitdir/refs" ]
  [ ! -L "$gitdir/logs" ]
  while [ "$refpath" != "$gitdir" ]; do [ ! -L "$refpath" ]; refpath=${refpath%/*}; done
  while [ "$logpath" != "$gitdir" ]; do [ ! -L "$logpath" ]; logpath=${logpath%/*}; done
fi
printf '%s\n%s\n%s\n' "$workspace" "$gitdir" "$ref"
`
}

func parseRemoteRegularGitMetadata(output string) (remoteRegularGitMetadata, error) {
	lines := strings.Split(strings.TrimSuffix(output, "\n"), "\n")
	if len(lines) != 3 {
		return remoteRegularGitMetadata{}, errors.New("remote git metadata is invalid")
	}
	metadata := remoteRegularGitMetadata{CheckoutPath: lines[0], GitDir: lines[1], CurrentRef: lines[2]}
	if !validRemoteRegularGitMetadata(metadata) {
		return remoteRegularGitMetadata{}, errors.New("remote git metadata is invalid")
	}
	return metadata, nil
}
