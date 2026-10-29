package process

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/common/turnchanges"
)

const (
	turnCheckpointProvenanceHeader  = "Kandev turn change checkpoint"
	turnCheckpointOperationNone     = "none"
	turnCheckpointTreeMergeMinVer   = 38
	turnCheckpointTargetReady       = "ready"
	turnCheckpointTargetUnavailable = "unavailable"
	turnCheckpointOperationMerge    = "merge"
	turnCheckpointOperationRebase   = "rebase"
	turnCheckpointOperationApply    = "rebase"
	turnCheckpointOperationCherry   = "cherry-pick"
	turnCheckpointOperationRevert   = "revert"
	turnCheckpointOperationBisect   = "bisect"
)

var turnCheckpointGitVersionPattern = regexp.MustCompile(`git version (\d+)\.(\d+)`)

// captureTurnCheckpointProvenance resolves the bounded history evidence that
// identifies this endpoint without fetching or inventing a comparison target.
func (g *GitOperator) captureTurnCheckpointProvenance(
	ctx context.Context,
	request turnchanges.CheckpointRequest,
	gitDir, hashAlgorithm string,
) (*turnchanges.Provenance, error) {
	headCommit, headRef, unborn, err := g.turnCheckpointHeadState(ctx)
	if err != nil {
		return nil, err
	}
	provenance := &turnchanges.Provenance{
		Version:             turnchanges.ProvenanceVersion2,
		ChangeSetID:         request.ChangeSetID,
		CheckoutID:          request.CheckoutID,
		Boundary:            request.Boundary,
		HashAlgorithm:       hashAlgorithm,
		Unborn:              unborn,
		HeadCommitOID:       headCommit,
		HeadRef:             headRef,
		OperationState:      turnCheckpointOperationState(gitDir),
		TreeMergeCapability: g.turnCheckpointTreeMergeCapability(ctx),
		CapturedAt:          time.Now().UTC(),
	}
	g.applyTurnCheckpointTarget(ctx, provenance, headCommit, unborn)
	return provenance, nil
}

func (g *GitOperator) turnCheckpointHeadState(ctx context.Context) (string, string, bool, error) {
	output, err := g.turnCheckpointOutput(ctx, "", "rev-parse", "--verify", "HEAD")
	if err == nil {
		commit := strings.TrimSpace(string(output))
		ref, refErr := g.turnCheckpointOutput(ctx, "", "symbolic-ref", "-q", "HEAD")
		if refErr != nil {
			return commit, "", false, nil
		}
		return commit, strings.TrimSpace(string(ref)), false, nil
	}
	if ctx.Err() != nil {
		return "", "", false, ctx.Err()
	}
	ref, refErr := g.turnCheckpointOutput(ctx, "", "symbolic-ref", "-q", "HEAD")
	if refErr != nil || !strings.HasPrefix(strings.TrimSpace(string(ref)), "refs/heads/") {
		return "", "", false, turnCheckpointFailure(
			turnchanges.ReasonCheckoutUnavailable, errors.New("repository HEAD is neither valid nor an unborn branch"))
	}
	return "", strings.TrimSpace(string(ref)), true, nil
}

func (g *GitOperator) applyTurnCheckpointTarget(
	ctx context.Context,
	provenance *turnchanges.Provenance,
	headCommit string,
	unborn bool,
) {
	if g.workspaceTracker == nil {
		return
	}
	resolution := g.workspaceTracker.ComparisonResolution()
	if resolution.Explicit {
		provenance.TargetConfigured = true
		provenance.TargetIdentity = resolution.Display
		provenance.TargetRef = resolution.Ref
		provenance.TargetStatus = resolution.Status
		provenance.TargetErrorCode = resolution.ErrorCode
		if resolution.Status == turnCheckpointTargetReady && resolution.Ref != "" {
			g.applyTurnCheckpointTargetCommit(ctx, provenance, resolution.Ref, headCommit, unborn)
		}
		return
	}
	g.applyTurnCheckpointStoredBase(ctx, provenance, headCommit, unborn)
}

// applyTurnCheckpointStoredBase uses the tracker's existing stored-base
// resolution instead of inventing a default branch. An unresolved stored base
// stays unavailable and never falls back to a same-named remote branch.
func (g *GitOperator) applyTurnCheckpointStoredBase(
	ctx context.Context,
	provenance *turnchanges.Provenance,
	headCommit string,
	unborn bool,
) {
	resolution := g.workspaceTracker.resolveBaseBranchWithReason(ctx)
	switch resolution.reason {
	case baseBranchStored:
		provenance.TargetConfigured = true
		provenance.TargetIdentity = resolution.stored
		provenance.TargetRef = resolution.ref
		provenance.TargetStatus = turnCheckpointTargetReady
		g.applyTurnCheckpointTargetCommit(ctx, provenance, resolution.ref, headCommit, unborn)
	case baseBranchFallbackStoredUnresolved:
		provenance.TargetConfigured = true
		provenance.TargetIdentity = resolution.stored
		provenance.TargetStatus = turnCheckpointTargetUnavailable
		provenance.TargetErrorCode = "comparison_target_unavailable"
	}
}

func (g *GitOperator) applyTurnCheckpointTargetCommit(
	ctx context.Context,
	provenance *turnchanges.Provenance,
	ref, headCommit string,
	unborn bool,
) {
	targetCommit, err := g.turnCheckpointResolveCommit(ctx, ref)
	if err != nil {
		provenance.TargetStatus = turnCheckpointTargetUnavailable
		provenance.TargetErrorCode = "comparison_target_unavailable"
		return
	}
	provenance.TargetCommitOID = targetCommit
	if unborn || headCommit == "" {
		return
	}
	mergeBase, err := g.turnCheckpointUniqueMergeBase(ctx, headCommit, targetCommit)
	if err != nil {
		provenance.TargetStatus = turnCheckpointTargetUnavailable
		provenance.TargetErrorCode = "comparison_merge_base_unavailable"
		return
	}
	provenance.MergeBaseOID = mergeBase
}

func (g *GitOperator) turnCheckpointResolveCommit(ctx context.Context, ref string) (string, error) {
	output, err := g.turnCheckpointOutput(ctx, "", "rev-parse", "--verify", ref)
	if err != nil {
		return "", err
	}
	commit := strings.TrimSpace(string(output))
	if commit == "" {
		return "", errors.New("comparison target did not resolve to a commit")
	}
	return commit, nil
}

func (g *GitOperator) turnCheckpointUniqueMergeBase(ctx context.Context, left, right string) (string, error) {
	output, err := g.turnCheckpointOutput(ctx, "", "merge-base", "--all", left, right)
	if err != nil {
		return "", err
	}
	bases := strings.Fields(string(output))
	if len(bases) != 1 {
		return "", fmt.Errorf("expected one merge base, found %d", len(bases))
	}
	return bases[0], nil
}

func turnCheckpointOperationState(gitDir string) string {
	for _, operation := range []struct {
		path  string
		state string
	}{
		{"MERGE_HEAD", turnCheckpointOperationMerge},
		{"rebase-merge", turnCheckpointOperationRebase},
		{"rebase-apply", turnCheckpointOperationApply},
		{"CHERRY_PICK_HEAD", turnCheckpointOperationCherry},
		{"REVERT_HEAD", turnCheckpointOperationRevert},
		{"BISECT_LOG", turnCheckpointOperationBisect},
	} {
		if _, err := os.Lstat(filepath.Join(gitDir, operation.path)); err == nil {
			return operation.state
		}
	}
	return turnCheckpointOperationNone
}

func (g *GitOperator) turnCheckpointTreeMergeCapability(ctx context.Context) bool {
	output, err := g.turnCheckpointOutput(ctx, "", "version")
	if err != nil {
		return false
	}
	match := turnCheckpointGitVersionPattern.FindStringSubmatch(string(output))
	if len(match) != 3 {
		return false
	}
	major, majorErr := strconv.Atoi(match[1])
	minor, minorErr := strconv.Atoi(match[2])
	if majorErr != nil || minorErr != nil {
		return false
	}
	if major > 2 {
		return true
	}
	return major == 2 && minor >= turnCheckpointTreeMergeMinVer
}

// verifyTurnCheckpointProvenance rejects a capture whose history moved while
// the snapshot was being written.
func (g *GitOperator) verifyTurnCheckpointProvenance(
	ctx context.Context,
	before *turnchanges.Provenance,
	gitDir string,
) error {
	if before == nil {
		return nil
	}
	after, err := g.captureTurnCheckpointProvenance(ctx, turnchanges.CheckpointRequest{
		ChangeSetID: before.ChangeSetID, CheckoutID: before.CheckoutID, Boundary: before.Boundary,
	}, gitDir, before.HashAlgorithm)
	if err != nil {
		return err
	}
	if after.HeadCommitOID != before.HeadCommitOID || after.HeadRef != before.HeadRef ||
		after.Unborn != before.Unborn || after.TargetCommitOID != before.TargetCommitOID ||
		after.MergeBaseOID != before.MergeBaseOID || after.TargetStatus != before.TargetStatus ||
		after.OperationState != before.OperationState {
		return turnCheckpointFailure(
			turnchanges.ReasonHistoryProvenanceMissing, errors.New("repository history changed during checkpoint capture"))
	}
	return nil
}

func turnCheckpointParents(provenance *turnchanges.Provenance) []string {
	if provenance == nil {
		return nil
	}
	parents := make([]string, 0, 2)
	seen := make(map[string]struct{}, 2)
	for _, parent := range []string{provenance.HeadCommitOID, provenance.TargetCommitOID} {
		if parent == "" {
			continue
		}
		if _, exists := seen[parent]; exists {
			continue
		}
		seen[parent] = struct{}{}
		parents = append(parents, parent)
	}
	return parents
}

func encodeTurnCheckpointProvenance(provenance *turnchanges.Provenance) (string, error) {
	if provenance == nil {
		return turnCheckpointProvenanceHeader, nil
	}
	encoded, err := json.Marshal(provenance)
	if err != nil {
		return "", err
	}
	if len(encoded) > turnchanges.MaxProvenanceBytes {
		return "", turnCheckpointFailure(turnchanges.ReasonSizeLimit, errors.New("checkpoint provenance exceeds its byte limit"))
	}
	return turnCheckpointProvenanceHeader + "\n\n" + string(encoded), nil
}

func parseTurnCheckpointProvenance(message string) (*turnchanges.Provenance, error) {
	header, body, found := strings.Cut(message, "\n\n")
	if !found || strings.TrimSpace(header) != turnCheckpointProvenanceHeader {
		return nil, nil
	}
	body = strings.TrimSpace(body)
	if body == "" {
		return nil, nil
	}
	if len(body) > turnchanges.MaxProvenanceBytes {
		return nil, turnCheckpointFailure(turnchanges.ReasonSizeLimit, errors.New("checkpoint provenance exceeds its byte limit"))
	}
	provenance := &turnchanges.Provenance{}
	if err := json.Unmarshal([]byte(body), provenance); err != nil {
		return nil, turnCheckpointFailure(turnchanges.ReasonUnsafeGitState, errors.New("checkpoint provenance is malformed"))
	}
	if !provenance.Valid() {
		return nil, turnCheckpointFailure(turnchanges.ReasonUnsafeGitState, errors.New("checkpoint provenance is unsupported"))
	}
	return provenance, nil
}
