package process

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/common/subproc"
	"github.com/kandev/kandev/internal/common/turnchanges"
)

const (
	turnCheckpointComparisonRefPrefix = "refs/kandev/turn-comparisons/"
	turnCheckpointBaselineMessage     = "Kandev turn comparison baseline"
	turnCheckpointConflictExitCode    = 1
)

type turnCheckpointComparisonSelection struct {
	basis          turnchanges.ComparisonBasis
	reason         turnchanges.ReasonCode
	startCommitOID string
	startTreeOID   string
	endCommitOID   string
	endTreeOID     string
	comparisonRef  string
}

func (g *GitOperator) resolveTurnCheckpointComparison(
	ctx context.Context,
	start, end turnchanges.CheckpointResult,
	hashAlgorithm string,
) (turnCheckpointComparisonSelection, error) {
	unavailable := func(reason turnchanges.ReasonCode) turnCheckpointComparisonSelection {
		return turnCheckpointComparisonSelection{basis: turnchanges.ComparisonHistoryUnavailable, reason: reason}
	}
	startProv, endProv := start.Provenance, end.Provenance
	if !startProv.Valid() || !endProv.Valid() ||
		!turnCheckpointProvenanceObjectsValid(startProv, hashAlgorithm) ||
		!turnCheckpointProvenanceObjectsValid(endProv, hashAlgorithm) {
		return unavailable(turnchanges.ReasonHistoryProvenanceMissing), nil
	}
	if reason := turnCheckpointContextChange(startProv, endProv); reason != "" {
		return unavailable(reason), nil
	}
	if reason := turnCheckpointOperationReason(startProv, endProv); reason != "" {
		return unavailable(reason), nil
	}
	if g.turnCheckpointDirectEligible(ctx, startProv, endProv) {
		return turnCheckpointComparisonSelection{
			basis:          turnchanges.ComparisonDirect,
			startCommitOID: start.CommitOID, startTreeOID: start.TreeOID,
			endCommitOID: end.CommitOID, endTreeOID: end.TreeOID,
		}, nil
	}
	return g.selectTurnCheckpointAdjusted(ctx, start, end, hashAlgorithm, unavailable)
}

func (g *GitOperator) selectTurnCheckpointAdjusted(
	ctx context.Context,
	start, end turnchanges.CheckpointResult,
	hashAlgorithm string,
	unavailable func(turnchanges.ReasonCode) turnCheckpointComparisonSelection,
) (turnCheckpointComparisonSelection, error) {
	startProv, endProv := start.Provenance, end.Provenance
	if !turnCheckpointTargetsReady(startProv, endProv) {
		return unavailable(turnchanges.ReasonHistoryUnsupported), nil
	}
	if !startProv.TreeMergeCapability || !endProv.TreeMergeCapability {
		return unavailable(turnchanges.ReasonHistoryUnsupported), nil
	}
	if reason := g.turnCheckpointUnsafeAttributes(ctx, startProv, endProv); reason != "" {
		return unavailable(reason), nil
	}
	for _, tree := range []string{start.TreeOID, end.TreeOID} {
		custom, err := g.turnCheckpointTreeCustomMergeAttribute(ctx, tree)
		if err != nil {
			return unavailable(turnchanges.ReasonHistoryProvenanceMissing), nil
		}
		if custom {
			return unavailable(turnchanges.ReasonHistoryUnsupported), nil
		}
	}
	eligible, reason := g.turnCheckpointAdjustedBases(ctx, startProv, endProv)
	if !eligible {
		return unavailable(reason), nil
	}
	baseline, err := g.turnCheckpointAdjustedBaseline(ctx, startProv, endProv, start.TreeOID, hashAlgorithm)
	if err != nil {
		return unavailable(checkpointHistoryReason(err)), nil
	}
	return turnCheckpointComparisonSelection{
		basis:          turnchanges.ComparisonUpstreamAdjusted,
		startCommitOID: baseline.commitOID, startTreeOID: baseline.treeOID,
		endCommitOID: end.CommitOID, endTreeOID: end.TreeOID,
		comparisonRef: baseline.ref,
	}, nil
}

func turnCheckpointProvenanceObjectsValid(provenance *turnchanges.Provenance, hashAlgorithm string) bool {
	if provenance.HashAlgorithm != hashAlgorithm {
		return false
	}
	for _, oid := range []string{
		provenance.HeadCommitOID, provenance.TargetCommitOID, provenance.MergeBaseOID,
	} {
		if oid != "" && !validCheckpointOID(oid, hashAlgorithm) {
			return false
		}
	}
	return true
}

func turnCheckpointContextChange(start, end *turnchanges.Provenance) turnchanges.ReasonCode {
	if start.TargetConfigured != end.TargetConfigured ||
		start.TargetIdentity != end.TargetIdentity || start.TargetRef != end.TargetRef {
		return turnchanges.ReasonHistoryContextChanged
	}
	if start.HeadRef != end.HeadRef {
		return turnchanges.ReasonHistoryContextChanged
	}
	if start.HashAlgorithm != end.HashAlgorithm {
		return turnchanges.ReasonHistoryContextChanged
	}
	return ""
}

func turnCheckpointOperationReason(start, end *turnchanges.Provenance) turnchanges.ReasonCode {
	if start.OperationState != turnCheckpointOperationNone || end.OperationState != turnCheckpointOperationNone {
		return turnchanges.ReasonHistoryContextChanged
	}
	return ""
}

func (g *GitOperator) turnCheckpointDirectEligible(ctx context.Context, start, end *turnchanges.Provenance) bool {
	if start.Unborn && end.Unborn {
		return true
	}
	if start.Unborn && !end.Unborn {
		return g.turnCheckpointUnbornFirstRoot(ctx, start, end)
	}
	if start.Unborn != end.Unborn {
		return false
	}
	if start.HeadCommitOID != "" && start.HeadCommitOID == end.HeadCommitOID {
		return true
	}
	return turnCheckpointTargetsReady(start, end) &&
		start.MergeBaseOID != "" && start.MergeBaseOID == end.MergeBaseOID
}

// turnCheckpointUnbornFirstRoot accepts only a root history created during the
// turn. Imported history has an older root commit; when a target is known, its
// ancestry must not contain that root.
func (g *GitOperator) turnCheckpointUnbornFirstRoot(
	ctx context.Context,
	start, end *turnchanges.Provenance,
) bool {
	if !start.Unborn || end.Unborn || end.HeadCommitOID == "" || start.CapturedAt.IsZero() {
		return false
	}
	if start.HeadRef == "" || start.HeadRef != end.HeadRef {
		return false
	}
	endParents, err := g.turnCheckpointCommitParents(ctx, end.HeadCommitOID)
	if err != nil || len(endParents) != 0 {
		return false
	}
	rootTime, err := g.turnCheckpointCommitTime(ctx, end.HeadCommitOID)
	if err != nil || rootTime.Before(start.CapturedAt.Add(-time.Second)) {
		return false
	}
	if end.TargetConfigured && end.TargetCommitOID != "" &&
		g.turnCheckpointIsAncestor(ctx, end.HeadCommitOID, end.TargetCommitOID) {
		return false
	}
	return true
}

func (g *GitOperator) turnCheckpointCommitParents(ctx context.Context, commit string) ([]string, error) {
	output, err := g.turnCheckpointOutput(ctx, "", "show", "-s", "--format=%P", commit)
	if err != nil {
		return nil, err
	}
	return strings.Fields(string(output)), nil
}

func (g *GitOperator) turnCheckpointCommitTime(ctx context.Context, commit string) (time.Time, error) {
	output, err := g.turnCheckpointOutput(ctx, "", "show", "-s", "--format=%ct", commit)
	if err != nil {
		return time.Time{}, err
	}
	seconds, err := strconv.ParseInt(strings.TrimSpace(string(output)), 10, 64)
	if err != nil {
		return time.Time{}, err
	}
	return time.Unix(seconds, 0).UTC(), nil
}

func turnCheckpointTargetsReady(start, end *turnchanges.Provenance) bool {
	return start.TargetConfigured && end.TargetConfigured &&
		start.TargetStatus == turnCheckpointTargetReady && end.TargetStatus == turnCheckpointTargetReady &&
		start.TargetCommitOID != "" && end.TargetCommitOID != "" &&
		start.MergeBaseOID != "" && end.MergeBaseOID != ""
}

func (g *GitOperator) turnCheckpointAdjustedBases(
	ctx context.Context,
	start, end *turnchanges.Provenance,
) (bool, turnchanges.ReasonCode) {
	if start.TargetRef == start.HeadRef || end.TargetRef == end.HeadRef {
		return false, turnchanges.ReasonHistoryContextChanged
	}
	if strings.HasPrefix(start.TargetRef, "refs/heads/") || strings.HasPrefix(end.TargetRef, "refs/heads/") {
		return false, turnchanges.ReasonHistoryContextChanged
	}
	if g.turnCheckpointTargetAliasesBranch(ctx, start.TargetRef, start.HeadRef) ||
		g.turnCheckpointTargetAliasesBranch(ctx, end.TargetRef, end.HeadRef) {
		return false, turnchanges.ReasonHistoryContextChanged
	}
	if start.MergeBaseOID == end.MergeBaseOID {
		return false, turnchanges.ReasonHistoryUnsupported
	}
	if !g.turnCheckpointIsAncestor(ctx, start.MergeBaseOID, end.MergeBaseOID) {
		return false, turnchanges.ReasonHistoryUnsupported
	}
	if !g.turnCheckpointIsAncestor(ctx, start.MergeBaseOID, start.HeadCommitOID) ||
		!g.turnCheckpointIsAncestor(ctx, end.MergeBaseOID, end.HeadCommitOID) {
		return false, turnchanges.ReasonHistoryUnsupported
	}
	if !g.turnCheckpointUniqueBaseMatches(ctx, start.HeadCommitOID, start.TargetCommitOID, start.MergeBaseOID) ||
		!g.turnCheckpointUniqueBaseMatches(ctx, end.HeadCommitOID, end.TargetCommitOID, end.MergeBaseOID) {
		return false, turnchanges.ReasonHistoryUnsupported
	}
	return true, ""
}

func (g *GitOperator) turnCheckpointTargetAliasesBranch(ctx context.Context, targetRef, headRef string) bool {
	if targetRef == "" || headRef == "" {
		return false
	}
	output, err := g.turnCheckpointOutput(ctx, "", "symbolic-ref", "-q", targetRef)
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(output)) == headRef
}

func (g *GitOperator) turnCheckpointIsAncestor(ctx context.Context, ancestor, descendant string) bool {
	if ancestor == "" || descendant == "" {
		return false
	}
	output, err := g.turnCheckpointOutput(ctx, "", "merge-base", "--all", ancestor, descendant)
	if err != nil {
		return false
	}
	bases := strings.Fields(string(output))
	return len(bases) == 1 && bases[0] == ancestor
}

func (g *GitOperator) turnCheckpointUniqueBaseMatches(ctx context.Context, left, right, expected string) bool {
	base, err := g.turnCheckpointUniqueMergeBase(ctx, left, right)
	return err == nil && base == expected
}

func (g *GitOperator) turnCheckpointUnsafeAttributes(
	ctx context.Context,
	start, end *turnchanges.Provenance,
) turnchanges.ReasonCode {
	for _, tree := range []string{start.HeadCommitOID, end.HeadCommitOID, start.MergeBaseOID, end.MergeBaseOID} {
		if tree == "" {
			continue
		}
		custom, err := g.turnCheckpointTreeCustomMergeAttribute(ctx, tree)
		if err != nil {
			return turnchanges.ReasonHistoryProvenanceMissing
		}
		if custom {
			return turnchanges.ReasonHistoryUnsupported
		}
	}
	return ""
}

func (g *GitOperator) turnCheckpointTreeCustomMergeAttribute(ctx context.Context, tree string) (bool, error) {
	output, err := g.turnCheckpointOutput(ctx, "", "ls-tree", "-r", tree)
	if err != nil {
		return false, err
	}
	for _, line := range strings.Split(string(output), "\n") {
		metadata, path, found := strings.Cut(line, "\t")
		if !found || filepath.Base(strings.TrimSpace(path)) != ".gitattributes" {
			continue
		}
		fields := strings.Fields(metadata)
		if len(fields) != 3 || fields[1] != "blob" {
			continue
		}
		content, err := g.turnCheckpointBlob(ctx, fields[2])
		if err != nil {
			return false, err
		}
		if turnCheckpointAttributesUseCustomMerge(content) {
			return true, nil
		}
	}
	return false, nil
}

func turnCheckpointAttributesUseCustomMerge(content []byte) bool {
	for _, line := range strings.Split(string(content), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		for _, field := range fields[1:] {
			if field == "-merge" || field == "!merge" || field == "merge" {
				continue
			}
			value, found := strings.CutPrefix(field, "merge=")
			if !found {
				continue
			}
			switch value {
			case "text", "binary", "union":
			default:
				return true
			}
		}
	}
	return false
}

type turnCheckpointBaseline struct {
	commitOID string
	treeOID   string
	ref       string
}

func (g *GitOperator) turnCheckpointAdjustedBaseline(
	ctx context.Context,
	start, end *turnchanges.Provenance,
	localTreeOID, hashAlgorithm string,
) (turnCheckpointBaseline, error) {
	treeOID, err := g.turnCheckpointMergeTree(ctx, start.MergeBaseOID, end.MergeBaseOID, localTreeOID, hashAlgorithm)
	if err != nil {
		return turnCheckpointBaseline{}, err
	}
	if !validCheckpointOID(treeOID, hashAlgorithm) {
		return turnCheckpointBaseline{}, turnCheckpointFailure(
			turnchanges.ReasonComparisonFailed, errors.New("git merge-tree returned an invalid tree"))
	}
	ref := turnCheckpointComparisonRefFor(start.ChangeSetID, start.CheckoutID)
	commitOID, err := g.turnCheckpointBaselineCommit(ctx, ref, treeOID, end.MergeBaseOID, hashAlgorithm)
	if err != nil {
		return turnCheckpointBaseline{}, err
	}
	return turnCheckpointBaseline{commitOID: commitOID, treeOID: treeOID, ref: ref}, nil
}

func turnCheckpointComparisonRefFor(changeSetID, checkoutID string) string {
	return turnCheckpointComparisonRefPrefix + changeSetID + "/" + checkoutID
}

func (g *GitOperator) turnCheckpointBaselineCommit(
	ctx context.Context,
	ref, treeOID, upstreamCommit, hashAlgorithm string,
) (string, error) {
	if existing := g.turnCheckpointExistingComparisonCommit(ctx, ref, treeOID, hashAlgorithm); existing != "" {
		return existing, nil
	}
	environment := turnCheckpointCommitEnvironment(time.Now().UTC())
	output, err := g.turnCheckpointOutputWithEnvironment(ctx, "", environment,
		"commit-tree", treeOID, "-p", upstreamCommit, "-m", turnCheckpointBaselineMessage)
	if err != nil {
		return "", turnCheckpointFailure(turnchanges.ReasonComparisonFailed, err)
	}
	commitOID := strings.TrimSpace(string(output))
	if !validCheckpointOID(commitOID, hashAlgorithm) {
		return "", turnCheckpointFailure(turnchanges.ReasonComparisonFailed, errors.New("git commit-tree returned an invalid comparison baseline"))
	}
	if _, err := g.turnCheckpointOutput(ctx, "", "update-ref", ref, commitOID); err != nil {
		return "", turnCheckpointFailure(turnchanges.ReasonComparisonFailed, err)
	}
	return commitOID, nil
}

func (g *GitOperator) turnCheckpointExistingComparisonCommit(
	ctx context.Context,
	ref, treeOID, hashAlgorithm string,
) string {
	output, err := g.turnCheckpointOutput(ctx, "", "for-each-ref", "--format=%(objectname)", ref)
	if err != nil {
		return ""
	}
	commitOID := strings.TrimSpace(string(output))
	if !validCheckpointOID(commitOID, hashAlgorithm) {
		return ""
	}
	treeOutput, err := g.turnCheckpointOutput(ctx, "", "show", "-s", "--format=%T", commitOID)
	if err != nil || strings.TrimSpace(string(treeOutput)) != treeOID {
		return ""
	}
	return commitOID
}

func (g *GitOperator) turnCheckpointMergeTree(ctx context.Context, base, upstream, localTree, hashAlgorithm string) (string, error) {
	if !validTurnCheckpointMergeOID(base) || !validTurnCheckpointMergeOID(upstream) || !validTurnCheckpointMergeOID(localTree) {
		return "", turnCheckpointFailure(turnchanges.ReasonHistoryProvenanceMissing, errors.New("comparison baseline inputs are not exact objects"))
	}
	view, cleanup, err := g.newTurnCheckpointDiffView(ctx, localTree, hashAlgorithm)
	if err != nil {
		return "", turnCheckpointFailure(turnchanges.ReasonComparisonFailed, err)
	}
	defer cleanup()
	localCommit, err := g.turnCheckpointSyntheticTreeCommit(ctx, localTree, hashAlgorithm)
	if err != nil {
		return "", err
	}
	environment := turnCheckpointMergeContextEnvironment(view, localTree)
	stdout, exitCode, err := g.runTurnCheckpointGitWithEnvironment(ctx, view.operator.workDir, environment, []string{
		"merge-tree", "--write-tree", "--merge-base=" + base, upstream, localCommit,
	})
	if err != nil {
		return "", turnCheckpointFailure(turnchanges.ReasonComparisonFailed, err)
	}
	if exitCode == turnCheckpointConflictExitCode {
		return "", turnCheckpointFailure(turnchanges.ReasonHistoryConflict, errors.New("comparison baseline merge conflicted"))
	}
	if exitCode != 0 || len(stdout) == 0 {
		return "", turnCheckpointFailure(turnchanges.ReasonHistoryUnsupported, errors.New("git merge-tree did not produce a clean tree"))
	}
	treeOID := strings.TrimSpace(strings.SplitN(string(stdout), "\n", 2)[0])
	return treeOID, nil
}

// turnCheckpointSyntheticTreeCommit gives merge-tree a commit for the local
// tree. Older Git versions reject a bare tree as the second branch argument,
// so the three-way merge always receives commit objects.
func (g *GitOperator) turnCheckpointSyntheticTreeCommit(
	ctx context.Context,
	treeOID, hashAlgorithm string,
) (string, error) {
	environment := turnCheckpointCommitEnvironment(time.Now().UTC())
	output, err := g.turnCheckpointOutputWithEnvironment(ctx, "", environment,
		"commit-tree", treeOID, "-m", turnCheckpointBaselineMessage)
	if err != nil {
		return "", turnCheckpointFailure(turnchanges.ReasonComparisonFailed, err)
	}
	commitOID := strings.TrimSpace(string(output))
	if !validCheckpointOID(commitOID, hashAlgorithm) {
		return "", turnCheckpointFailure(
			turnchanges.ReasonComparisonFailed, errors.New("git commit-tree returned an invalid local comparison commit"))
	}
	return commitOID, nil
}

func validTurnCheckpointMergeOID(oid string) bool {
	return validCheckpointOID(oid, turnCheckpointSHA1) || validCheckpointOID(oid, turnCheckpointSHA256)
}

// runTurnCheckpointGit runs fixed plumbing arguments built exclusively from
// validated object identities. The generic user-facing argument validator does
// not recognize merge-tree's plumbing flags.
func turnCheckpointMergeContextEnvironment(view *turnCheckpointDiffView, localTree string) []string {
	overrides := make(map[string]string, len(view.environment)+10)
	for key, value := range view.environment {
		overrides[key] = value
	}
	overrides["GIT_CONFIG_NOSYSTEM"] = "1"
	overrides["GIT_CONFIG_GLOBAL"] = os.DevNull
	overrides["GIT_CONFIG_SYSTEM"] = os.DevNull
	overrides["GIT_CONFIG_PARAMETERS"] = ""
	overrides["GIT_ATTR_NOSYSTEM"] = "1"
	overrides["GIT_ATTR_SOURCE"] = localTree
	overrides["GIT_NO_REPLACE_OBJECTS"] = "1"
	overrides["GIT_CONFIG_COUNT"] = "3"
	overrides["GIT_CONFIG_KEY_0"] = "merge.default"
	overrides["GIT_CONFIG_VALUE_0"] = "text"
	overrides["GIT_CONFIG_KEY_1"] = "merge.renormalize"
	overrides["GIT_CONFIG_VALUE_1"] = "false"
	overrides["GIT_CONFIG_KEY_2"] = "core.attributesFile"
	overrides["GIT_CONFIG_VALUE_2"] = os.DevNull
	return turnCheckpointEnvironment(view.operator.environmentValues(), "", overrides)
}

func checkpointHistoryReason(err error) turnchanges.ReasonCode {
	var checkpointErr *TurnCheckpointError
	if errors.As(err, &checkpointErr) && checkpointErr.Reason.Valid() {
		return checkpointErr.Reason
	}
	return turnchanges.ReasonHistoryUnsupported
}

func (g *GitOperator) runTurnCheckpointGitWithEnvironment(
	ctx context.Context,
	workDir string,
	environment []string,
	args []string,
) ([]byte, int, error) {
	stdout := &boundedGitOutput{limit: turnCheckpointOutputLimit}
	stderr := &boundedGitOutput{limit: turnCheckpointStderrLimit}
	checkpointArgs := append([]string{"-c", "core.fsync=" + turnCheckpointFsync}, args...)
	exitCode := 0
	runErr, contextErr := subproc.RunGitAfterAcquire(ctx, subproc.GitLifecycle, 30*time.Second, func(execCtx context.Context) *exec.Cmd {
		cmd := subproc.NewGitCommand(execCtx, checkpointArgs...)
		cmd.Dir = workDir
		cmd.Env = environment
		cmd.Stdout = stdout
		cmd.Stderr = stderr
		return cmd
	})
	if contextErr != nil {
		return nil, 0, contextErr
	}
	if stdout.truncated || stderr.truncated {
		return nil, 0, ErrTurnCheckpointOutputLimit
	}
	if runErr != nil {
		var exitErr *exec.ExitError
		if !errors.As(runErr, &exitErr) {
			return nil, 0, runErr
		}
		exitCode = exitErr.ExitCode()
	}
	return append([]byte(nil), stdout.Bytes()...), exitCode, nil
}
