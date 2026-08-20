package lifecycle

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/worktree"
)

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
func prepareRemoteRegularGitMetadataPolicy(req *ExecutorCreateRequest, metadata remoteRegularGitMetadata) error {
	descriptor, err := remoteFilesystemPolicyDescriptor(req)
	if err != nil {
		return err
	}
	if !validRemoteRegularGitMetadata(metadata) {
		return errors.New("remote git metadata is invalid")
	}
	config, err := codexConfigFromEnvironment(req.Env, descriptor.ConfigEnvKey)
	if err != nil {
		return err
	}
	policy, err := descriptor.Renderer.Render(remoteRegularGitMetadataFilesystemPolicy(metadata))
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

func remoteRegularGitMetadataFilesystemPolicy(metadata remoteRegularGitMetadata) agents.FilesystemPolicy {
	// A regular clone has one task-owned .git directory. Git writes index.lock
	// and, for detached HEADs, HEAD.lock directly in that directory, so granting
	// only descendants is insufficient. Unlike a linked worktree, this is not a
	// shared common directory: it belongs solely to the remote task checkout.
	return agents.FilesystemPolicy{
		Name: codexGitMetadataPolicyName,
		Rules: []agents.FilesystemPolicyRule{
			{Path: ":minimal", Access: agents.FilesystemAccessRead},
			{Path: metadata.GitDir, Access: agents.FilesystemAccessWrite},
		},
	}
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
