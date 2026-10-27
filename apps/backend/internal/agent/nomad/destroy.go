package nomad

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/kandev/kandev/internal/agentctl/types"
)

// StopJob deregisters the job without purging it. The stopped job and its
// allocation history remain readable so absence can be proven afterwards.
func (c *Client) StopJob(ctx context.Context, namespace, jobID string) error {
	if namespace == "" || jobID == "" {
		return errors.New("nomad job stop requires namespace and stable job ID")
	}
	path := "/v1/job/" + url.PathEscape(jobID) + "?purge=false&global=false&namespace=" + url.QueryEscape(namespace)
	status, _, err := c.doRaw(ctx, http.MethodDelete, path, nil)
	if err != nil {
		return err
	}
	if status < 200 || status >= 300 {
		return fmt.Errorf("nomad job stop returned HTTP %d", status)
	}
	return nil
}

type receiptFingerprint struct {
	InstallationID  string `json:"installation_id"`
	EnvironmentID   string `json:"environment_id"`
	Generation      int64  `json:"generation"`
	OperationID     string `json:"operation_id"`
	InputDigest     string `json:"input_digest"`
	Namespace       string `json:"namespace"`
	JobID           string `json:"job_id"`
	NodeID          string `json:"node_id"`
	AllocationID    string `json:"allocation_id"`
	AllocationIndex uint64 `json:"allocation_create_index"`
	ServerIdentity  string `json:"state_server_identity"`
	ClaimID         string `json:"claim_id"`
	RequestID       string `json:"receipt_request_id"`
	ReceiptIdentity string `json:"receipt_server_identity"`
	Sealed          bool   `json:"receipt_sealed"`
	Drained         bool   `json:"receipt_drained"`
	Scope           string `json:"receipt_scope"`
	Version         string `json:"receipt_version"`
	CompletedAt     string `json:"receipt_completed_at"`
}

// ReceiptDigest fingerprints the immutable identity and receipt content. Phase,
// Revision and the stored receipt digest and generation are excluded so the
// value is stable across phase transitions.
func ReceiptDigest(state EnvironmentState, receipt types.RuntimeDrainReceipt, claimID string) string {
	encoded, _ := json.Marshal(receiptFingerprint{
		InstallationID: state.InstallationID, EnvironmentID: state.EnvironmentID, Generation: state.Generation,
		OperationID: state.OperationID, InputDigest: state.InputDigest, Namespace: state.Namespace,
		JobID: state.JobID, NodeID: state.NodeID, AllocationID: state.AllocationID,
		AllocationIndex: state.AllocationCreateIndex, ServerIdentity: state.ServerIdentity, ClaimID: claimID,
		RequestID: receipt.RequestID, ReceiptIdentity: receipt.ServerIdentity, Sealed: receipt.Sealed,
		Drained: receipt.Drained, Scope: receipt.Scope, Version: receipt.Version,
		CompletedAt: receipt.CompletedAt.UTC().Format("2006-01-02T15:04:05.000000000Z07:00"),
	})
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:])
}

// ConfirmStopped accepts only a stopped job whose single recorded allocation
// is terminal on the recorded, ready node. An empty allocation list is missing
// history, never evidence of absence.
func ConfirmStopped(state EnvironmentState, job *JobRecord, allocs []AllocationRecord, node *NodeRecord) error {
	if err := state.Validate(); err != nil || state.AllocationID == "" {
		return errors.New("confirm Nomad stop: recorded allocation identity is incomplete")
	}
	if job == nil || job.ID != state.JobID || job.Namespace != state.Namespace || !job.Spec.Stop {
		return errors.New("confirm Nomad stop: job is missing or not stopped")
	}
	if len(allocs) != 1 {
		return errors.New("confirm Nomad stop: allocation history is not exactly the recorded allocation")
	}
	if err := checkTerminalAllocation(state, allocs[0]); err != nil {
		return err
	}
	if node == nil || node.ID != state.NodeID || node.Status != "ready" {
		return errors.New("confirm Nomad stop: recorded node is not ready")
	}
	return nil
}

func checkTerminalAllocation(state EnvironmentState, alloc AllocationRecord) error {
	if alloc.ID != state.AllocationID || alloc.NodeID != state.NodeID || alloc.JobID != state.JobID || alloc.Namespace != state.Namespace {
		return errors.New("confirm Nomad stop: allocation identity does not match the recorded owner")
	}
	if state.AllocationCreateIndex != 0 && alloc.CreateIndex != state.AllocationCreateIndex {
		return errors.New("confirm Nomad stop: allocation creation index does not match")
	}
	if alloc.ClientStatus != "complete" && alloc.ClientStatus != "failed" {
		return errors.New("confirm Nomad stop: allocation is not terminal on the client")
	}
	return nil
}

func fenceOwnerToken(state EnvironmentState) string {
	return strings.Join([]string{
		state.InstallationID, state.NodeID, state.EnvironmentID, fmt.Sprint(state.Generation),
		state.OperationID, state.InputDigest, state.JobID,
	}, ":") + "\n" + state.AllocationID
}

// FenceReleaseCommand returns a POSIX shell command that removes the worker
// fence only when its owner record names exactly this environment. It exits
// non-zero when another owner holds the fence; an already absent fence is
// released.
func FenceReleaseCommand(profile ProfileConfig, state EnvironmentState) (string, error) {
	if state.AllocationID == "" || state.NodeID != profile.NodeID || !safeNodeID.MatchString(state.NodeID) {
		return "", errors.New("release Nomad worker fence: exact allocation ownership is incomplete")
	}
	root, err := normalizedAbsolutePath(profile.AllocationRoot)
	if err != nil || filepath.Clean(root) != state.AllocationRoot {
		return "", errors.New("release Nomad worker fence: allocation root differs from immutable state")
	}
	dir := FenceDir(state, root)
	parent := ""
	if state.PlacementMode == ModePool {
		parent = ` && { rmdir "$(dirname "$d")" 2>/dev/null || true; }`
	}
	return "d=" + shellQuote(dir) + `; f="$d/owner"; ` +
		`[ -e "$d" ] || [ -L "$d" ] || exit 0; ` +
		`[ ! -L "$d" ] && [ -d "$d" ] && [ ! -L "$f" ] && [ -f "$f" ] && ` +
		`[ "$(cat "$f")" = ` + shellQuote(fenceOwnerToken(state)) + ` ] && rm -f "$f" && rmdir "$d"` + parent, nil
}

// WorkspaceRemoveCommand returns a POSIX shell command that deletes a pool
// environment's workspace and its now-empty environment directory. It refuses a
// symlink and any path outside the state's workspace root. Legacy pinned
// workers keep their workspace; it returns an empty command for them.
func WorkspaceRemoveCommand(state EnvironmentState) (string, error) {
	if state.PlacementMode != ModePool {
		return "", nil
	}
	expected, err := EnvironmentWorkspacePath(state.WorkspaceRoot, state.EnvironmentID, state.Generation)
	if err != nil || expected != state.WorkspacePath || filepath.Dir(filepath.Dir(state.WorkspacePath)) != filepath.Join(state.WorkspaceRoot, "environments") {
		return "", errors.New("remove Nomad pool workspace: path differs from the immutable state")
	}
	return "d=" + shellQuote(state.WorkspacePath) + `; b=` + shellQuote(PoolWorktreeBranch(state.EnvironmentID)) +
		`; [ -L "$d" ] && exit 1; [ -e "$d" ] || exit 0; ` + worktreeDetachScript +
		`rm -rf -- "$d" && { rmdir "$(dirname "$d")" 2>/dev/null || true; }`, nil
}

// AllocationDirRemoveCommand returns a POSIX shell command that deletes a pool
// environment's staged job directory (bootstrap script and environment file,
// ready marker, agentctl copy). It refuses a symlink and any path other than
// the environment's own job directory. Legacy pinned workers keep their
// allocation directory; it returns an empty command for them.
func AllocationDirRemoveCommand(state EnvironmentState) (string, error) {
	if state.PlacementMode != ModePool {
		return "", nil
	}
	if state.JobID == "" || filepath.Base(state.JobID) != state.JobID || state.JobID == "." || state.JobID == ".." ||
		state.AllocationRoot == "" || !filepath.IsAbs(state.AllocationRoot) {
		return "", errors.New("remove Nomad pool allocation directory: job identity or allocation root is unsafe")
	}
	return "d=" + shellQuote(filepath.Join(state.AllocationRoot, state.JobID)) +
		`; [ -L "$d" ] && exit 1; [ -e "$d" ] || exit 0; rm -rf -- "$d"`, nil
}

// PoolWorktreeBranch is the per-environment branch a local pool node creates
// in each repository checkout it worktrees from.
func PoolWorktreeBranch(environmentID string) string {
	return "kandev/pool/" + environmentID
}

// worktreeDetachScript removes every git worktree one level below $d, deletes
// the per-environment branch it had checked out (the exact $b or a
// per-generation $b-g<N>, nothing outside that prefix) from its source
// repository when git considers it merged, and prunes the repository. A branch
// holding unmerged commits is kept. A failure to remove the worktree or prune
// exits non-zero so the workspace is retained.
const worktreeDetachScript = `for g in "$d"/*/.git; do [ -f "$g" ] || continue; w="$(dirname "$g")"; ` +
	`c="$(git -C "$w" rev-parse --path-format=absolute --git-common-dir)" || exit 1; ` +
	`k="$(git -C "$w" symbolic-ref --short -q HEAD)"; ` +
	`git -C "$w" worktree remove --force "$w" || exit 1; ` +
	`case "$k" in "$b"|"$b"-*) if git --git-dir="$c" show-ref --verify --quiet "refs/heads/$k"; then git --git-dir="$c" branch -d "$k" >/dev/null 2>&1 || true; fi;; esac; ` +
	`git --git-dir="$c" worktree prune || exit 1; done; `
