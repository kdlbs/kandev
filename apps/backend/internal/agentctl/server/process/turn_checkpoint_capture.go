package process

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/common/subproc"
	"github.com/kandev/kandev/internal/common/turnchanges"
)

const (
	turnCheckpointStartTimeout = 10 * time.Second
	turnCheckpointEndTimeout   = 15 * time.Second
	turnCheckpointLockStale    = time.Minute
	turnCheckpointOutputLimit  = 8 << 20
	turnCheckpointEntryLimit   = 20_000
	turnCheckpointIndexLimit   = 64 << 20
	turnCheckpointBlobLimit    = 256 << 20
	turnCheckpointStderrLimit  = 64 << 10
	turnCheckpointSHA1         = "sha1"
	turnCheckpointSHA256       = "sha256"
	turnCheckpointConfigTrue   = "true"
	turnCheckpointFsync        = "added,reference"
)

var ErrTurnCheckpointOutputLimit = errors.New("turn checkpoint output limit exceeded")

// TurnCheckpointError records a stable availability reason with the executor
// failure that caused a checkpoint operation to stop.
type TurnCheckpointError struct {
	Reason turnchanges.ReasonCode
	Err    error
}

func (e *TurnCheckpointError) Error() string {
	return fmt.Sprintf("turn checkpoint %s: %v", e.Reason, e.Err)
}

func (e *TurnCheckpointError) Unwrap() error { return e.Err }

func turnCheckpointFailure(reason turnchanges.ReasonCode, err error) error {
	return &TurnCheckpointError{Reason: reason, Err: err}
}

// CaptureTurnCheckpoint stores an exact working-tree snapshot under a
// change-set-owned ref without writing the user's index, HEAD, or branches.
func (g *GitOperator) CaptureTurnCheckpoint(
	ctx context.Context,
	request turnchanges.CheckpointRequest,
) (turnchanges.CheckpointResult, error) {
	if err := validateTurnCheckpointRequest(request); err != nil {
		return turnchanges.CheckpointResult{}, err
	}
	deadline := turnCheckpointStartTimeout
	if request.Boundary == turnchanges.CheckpointEnd {
		deadline = turnCheckpointEndTimeout
	}
	operationCtx, cancel := context.WithTimeout(ctx, deadline)
	defer cancel()
	gitDir, hashAlgorithm, err := g.turnCheckpointRepository(operationCtx)
	if err != nil {
		return turnchanges.CheckpointResult{}, err
	}
	unlock, err := acquireTurnCheckpointLock(operationCtx, gitDir)
	if err != nil {
		return turnchanges.CheckpointResult{}, turnCheckpointFailure(turnchanges.ReasonCaptureFailed, err)
	}
	defer unlock()
	return g.captureTurnCheckpointLocked(operationCtx, request, gitDir, hashAlgorithm)
}

func validateTurnCheckpointRequest(request turnchanges.CheckpointRequest) error {
	if !validTurnChangeIdentity(request.ChangeSetID) || !validTurnChangeIdentity(request.CheckoutID) {
		return turnCheckpointFailure(turnchanges.ReasonUnsafeGitState, errors.New("invalid change-set or checkout identity"))
	}
	if request.Boundary != turnchanges.CheckpointStart && request.Boundary != turnchanges.CheckpointEnd {
		return turnCheckpointFailure(turnchanges.ReasonUnsafeGitState, errors.New("invalid checkpoint boundary"))
	}
	return nil
}

func (g *GitOperator) captureTurnCheckpointLocked(
	ctx context.Context,
	request turnchanges.CheckpointRequest,
	gitDir, hashAlgorithm string,
) (turnchanges.CheckpointResult, error) {
	if err := removeStaleTurnCheckpointIndexes(gitDir, time.Now()); err != nil {
		return turnchanges.CheckpointResult{}, turnCheckpointFailure(turnchanges.ReasonCaptureFailed, err)
	}
	ref := turnCheckpointRef(request.ChangeSetID, request.CheckoutID, request.Boundary)
	if existing, found, err := g.existingTurnCheckpoint(ctx, request, ref, hashAlgorithm); err != nil {
		return turnchanges.CheckpointResult{}, err
	} else if found {
		return existing, nil
	}
	indexPath, err := createTurnCheckpointIndex(gitDir)
	if err != nil {
		return turnchanges.CheckpointResult{}, turnCheckpointFailure(turnchanges.ReasonCaptureFailed, err)
	}
	defer removeTurnCheckpointIndex(indexPath)
	indexPresent, err := copyTurnCheckpointIndex(filepath.Join(gitDir, "index"), indexPath)
	if err != nil {
		return turnchanges.CheckpointResult{}, turnCheckpointFailure(turnchanges.ReasonUnsafeGitState, err)
	}
	if err := g.prepareTurnCheckpointIndex(ctx, indexPath, indexPresent); err != nil {
		return turnchanges.CheckpointResult{}, err
	}
	treeOID, commitOID, capturedAt, err := g.writeTurnCheckpointCommit(ctx, indexPath, hashAlgorithm)
	if err != nil {
		return turnchanges.CheckpointResult{}, err
	}
	zeroOID := strings.Repeat("0", len(commitOID))
	if _, err := g.turnCheckpointOutput(ctx, indexPath, "update-ref", ref, commitOID, zeroOID); err != nil {
		return g.resolveTurnCheckpointRefRace(ctx, request, ref, hashAlgorithm, err)
	}
	return turnchanges.CheckpointResult{
		ChangeSetID: request.ChangeSetID, CheckoutID: request.CheckoutID, Boundary: request.Boundary,
		CommitOID: commitOID, TreeOID: treeOID, HashAlgorithm: hashAlgorithm,
		ReachabilityRef: ref, CapturedAt: capturedAt,
	}, nil
}

func (g *GitOperator) writeTurnCheckpointCommit(
	ctx context.Context,
	indexPath, hashAlgorithm string,
) (string, string, time.Time, error) {
	treeOutput, err := g.turnCheckpointOutput(ctx, indexPath, "write-tree")
	if err != nil {
		return "", "", time.Time{}, turnCheckpointFailure(turnchanges.ReasonCaptureFailed, err)
	}
	treeOID := strings.TrimSpace(string(treeOutput))
	if !validCheckpointOID(treeOID, hashAlgorithm) {
		return "", "", time.Time{}, turnCheckpointFailure(turnchanges.ReasonCaptureFailed, errors.New("git write-tree returned an invalid object ID"))
	}
	capturedAt := time.Now().UTC().Truncate(time.Second)
	commitOutput, err := g.turnCheckpointOutputWithEnvironment(ctx, indexPath, turnCheckpointCommitEnvironment(capturedAt), "commit-tree", treeOID, "-m", "Kandev turn change checkpoint")
	if err != nil {
		return "", "", time.Time{}, turnCheckpointFailure(turnchanges.ReasonCaptureFailed, err)
	}
	commitOID := strings.TrimSpace(string(commitOutput))
	if !validCheckpointOID(commitOID, hashAlgorithm) {
		return "", "", time.Time{}, turnCheckpointFailure(turnchanges.ReasonCaptureFailed, errors.New("git commit-tree returned an invalid object ID"))
	}
	return treeOID, commitOID, capturedAt, nil
}

func (g *GitOperator) resolveTurnCheckpointRefRace(
	ctx context.Context,
	request turnchanges.CheckpointRequest,
	ref, hashAlgorithm string,
	updateErr error,
) (turnchanges.CheckpointResult, error) {
	if existing, found, readErr := g.existingTurnCheckpoint(ctx, request, ref, hashAlgorithm); readErr == nil && found {
		return existing, nil
	}
	return turnchanges.CheckpointResult{}, turnCheckpointFailure(turnchanges.ReasonCaptureFailed, updateErr)
}

func validTurnChangeIdentity(value string) bool {
	parsed, err := uuid.Parse(value)
	return err == nil && parsed.String() == value
}

func turnCheckpointRef(changeSetID, checkoutID string, boundary turnchanges.CheckpointBoundary) string {
	return "refs/kandev/turn-changes/" + changeSetID + "/" + checkoutID + "/" + string(boundary)
}

func validCheckpointOID(oid, algorithm string) bool {
	expected := 40
	if algorithm == turnCheckpointSHA256 {
		expected = 64
	}
	if len(oid) != expected {
		return false
	}
	for _, digit := range oid {
		if (digit < '0' || digit > '9') && (digit < 'a' || digit > 'f') {
			return false
		}
	}
	return algorithm == turnCheckpointSHA1 || algorithm == turnCheckpointSHA256
}

func turnCheckpointCommitEnvironment(capturedAt time.Time) map[string]string {
	date := capturedAt.Format(time.RFC3339)
	return map[string]string{
		"GIT_AUTHOR_NAME":     "Kandev Turn Capture",
		"GIT_AUTHOR_EMAIL":    "turn-capture@kandev.invalid",
		"GIT_AUTHOR_DATE":     date,
		"GIT_COMMITTER_NAME":  "Kandev Turn Capture",
		"GIT_COMMITTER_EMAIL": "turn-capture@kandev.invalid",
		"GIT_COMMITTER_DATE":  date,
	}
}

func (g *GitOperator) turnCheckpointRepository(ctx context.Context) (string, string, error) {
	gitDirOutput, err := g.turnCheckpointOutput(ctx, "", "rev-parse", "--absolute-git-dir")
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "not a git repository") {
			return "", "", turnCheckpointFailure(turnchanges.ReasonNoGitRepository, err)
		}
		return "", "", turnCheckpointFailure(turnchanges.ReasonCheckoutUnavailable, err)
	}
	gitDir, err := filepath.Abs(strings.TrimSpace(string(gitDirOutput)))
	if err != nil {
		return "", "", turnCheckpointFailure(turnchanges.ReasonCheckoutUnavailable, err)
	}
	gitDir, err = filepath.EvalSymlinks(gitDir)
	if err != nil {
		return "", "", turnCheckpointFailure(turnchanges.ReasonCheckoutUnavailable, err)
	}
	formatOutput, err := g.turnCheckpointOutput(ctx, "", "rev-parse", "--show-object-format")
	if err != nil {
		return "", "", turnCheckpointFailure(turnchanges.ReasonCheckoutUnavailable, err)
	}
	hashAlgorithm := strings.TrimSpace(string(formatOutput))
	if hashAlgorithm != turnCheckpointSHA1 && hashAlgorithm != turnCheckpointSHA256 {
		return "", "", turnCheckpointFailure(turnchanges.ReasonUnsupportedExecutor, fmt.Errorf("unsupported Git object format %q", hashAlgorithm))
	}
	return gitDir, hashAlgorithm, nil
}

func (g *GitOperator) existingTurnCheckpoint(
	ctx context.Context,
	request turnchanges.CheckpointRequest,
	ref, hashAlgorithm string,
) (turnchanges.CheckpointResult, bool, error) {
	output, err := g.turnCheckpointOutput(ctx, "", "for-each-ref", "--format=%(objectname)", ref)
	if err != nil {
		return turnchanges.CheckpointResult{}, false, turnCheckpointFailure(turnchanges.ReasonCaptureFailed, err)
	}
	commitOID := strings.TrimSpace(string(output))
	if commitOID == "" {
		return turnchanges.CheckpointResult{}, false, nil
	}
	if !validCheckpointOID(commitOID, hashAlgorithm) {
		return turnchanges.CheckpointResult{}, false, turnCheckpointFailure(turnchanges.ReasonUnsafeGitState, errors.New("existing checkpoint ref points to an invalid object"))
	}
	treeOutput, err := g.turnCheckpointOutput(ctx, "", "show", "-s", "--format=%T", commitOID)
	if err != nil {
		return turnchanges.CheckpointResult{}, false, turnCheckpointFailure(turnchanges.ReasonCaptureFailed, err)
	}
	treeOID := strings.TrimSpace(string(treeOutput))
	if !validCheckpointOID(treeOID, hashAlgorithm) {
		return turnchanges.CheckpointResult{}, false, turnCheckpointFailure(turnchanges.ReasonUnsafeGitState, errors.New("existing checkpoint commit has an invalid tree"))
	}
	dateOutput, err := g.turnCheckpointOutput(ctx, "", "show", "-s", "--format=%cI", commitOID)
	if err != nil {
		return turnchanges.CheckpointResult{}, false, turnCheckpointFailure(turnchanges.ReasonCaptureFailed, err)
	}
	capturedAt, err := time.Parse(time.RFC3339, strings.TrimSpace(string(dateOutput)))
	if err != nil {
		return turnchanges.CheckpointResult{}, false, turnCheckpointFailure(turnchanges.ReasonUnsafeGitState, errors.New("existing checkpoint commit has invalid capture time"))
	}
	return turnchanges.CheckpointResult{
		ChangeSetID: request.ChangeSetID, CheckoutID: request.CheckoutID, Boundary: request.Boundary,
		CommitOID: commitOID, TreeOID: treeOID, HashAlgorithm: hashAlgorithm,
		ReachabilityRef: ref, CapturedAt: capturedAt.UTC(), Reused: true,
	}, true, nil
}

func (g *GitOperator) prepareTurnCheckpointIndex(ctx context.Context, indexPath string, indexPresent bool) error {
	if indexPresent {
		if _, err := g.turnCheckpointOutput(ctx, indexPath, "update-index", "--no-split-index"); err != nil {
			return turnCheckpointFailure(turnchanges.ReasonUnsafeGitState, err)
		}
	}
	if err := g.prepareTurnCheckpointHead(ctx, indexPath, indexPresent); err != nil {
		return err
	}
	sparse, err := g.turnCheckpointSparseMode(ctx)
	if err != nil {
		return err
	}
	if err := g.refreshTurnCheckpointIndexFlags(ctx, indexPath); err != nil {
		return turnCheckpointFailure(turnchanges.ReasonUnsafeGitState, err)
	}
	return g.stageTurnCheckpointIndex(ctx, indexPath, sparse)
}

func (g *GitOperator) prepareTurnCheckpointHead(ctx context.Context, indexPath string, indexPresent bool) error {
	_, headErr := g.turnCheckpointOutput(ctx, indexPath, "rev-parse", "--verify", "HEAD")
	if headErr != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		unbornOutput, unbornErr := g.turnCheckpointOutput(ctx, "", "symbolic-ref", "-q", "HEAD")
		if unbornErr != nil || !strings.HasPrefix(strings.TrimSpace(string(unbornOutput)), "refs/heads/") {
			return turnCheckpointFailure(turnchanges.ReasonCheckoutUnavailable, errors.New("repository HEAD is neither valid nor an unborn branch"))
		}
		return nil
	}
	if !indexPresent {
		if _, err := g.turnCheckpointOutput(ctx, indexPath, "read-tree", "HEAD"); err != nil {
			return turnCheckpointFailure(turnchanges.ReasonCaptureFailed, err)
		}
	}
	return nil
}

func (g *GitOperator) turnCheckpointSparseMode(ctx context.Context) (bool, error) {
	sparseOutput, sparseErr := g.turnCheckpointOutput(ctx, "", "config", "--bool", "--default=false", "core.sparseCheckout")
	if sparseErr != nil {
		return false, turnCheckpointFailure(turnchanges.ReasonUnsafeGitState, sparseErr)
	}
	if strings.TrimSpace(string(sparseOutput)) != turnCheckpointConfigTrue {
		return false, nil
	}
	coneOutput, coneErr := g.turnCheckpointOutput(ctx, "", "config", "--bool", "--default=false", "core.sparseCheckoutCone")
	if coneErr != nil {
		return false, turnCheckpointFailure(turnchanges.ReasonUnsafeGitState, coneErr)
	}
	if strings.TrimSpace(string(coneOutput)) != turnCheckpointConfigTrue {
		return false, turnCheckpointFailure(turnchanges.ReasonSparseCaptureUnsupported, errors.New("non-cone sparse checkout capture is unsupported"))
	}
	return true, nil
}

func (g *GitOperator) stageTurnCheckpointIndex(ctx context.Context, indexPath string, sparse bool) error {
	if err := g.preflightTurnCheckpointBlobs(ctx, indexPath); err != nil {
		return err
	}
	preservedEntries, err := g.missingTurnCheckpointIndexEntries(ctx, indexPath)
	if err != nil {
		if errors.Is(err, ErrTurnCheckpointOutputLimit) {
			return turnCheckpointFailure(turnchanges.ReasonEntryLimit, err)
		}
		return turnCheckpointFailure(turnchanges.ReasonUnsafeGitState, err)
	}
	addArgs := []string{"add", "-A"}
	if sparse {
		addArgs = []string{"add", "--sparse", "-A"}
	}
	if _, err := g.turnCheckpointOutput(ctx, indexPath, addArgs...); err != nil {
		return turnCheckpointFailure(turnchanges.ReasonCaptureFailed, err)
	}
	renormalizeArgs := []string{"add", "--renormalize", "-u"}
	if sparse {
		renormalizeArgs = []string{"add", "--sparse", "--renormalize", "-u"}
	}
	if _, err := g.turnCheckpointOutput(ctx, indexPath, renormalizeArgs...); err != nil {
		return turnCheckpointFailure(turnchanges.ReasonCaptureFailed, err)
	}
	if len(preservedEntries) > 0 {
		if _, err := g.turnCheckpointOutputWithInput(ctx, indexPath, nil, preservedEntries, "update-index", "-z", "--index-info"); err != nil {
			return turnCheckpointFailure(turnchanges.ReasonCaptureFailed, err)
		}
	}
	return nil
}

func (g *GitOperator) missingTurnCheckpointIndexEntries(ctx context.Context, indexPath string) ([]byte, error) {
	output, err := g.turnCheckpointOutput(ctx, indexPath, "ls-files", "-v", "--stage", "-z")
	if err != nil {
		return nil, err
	}
	if len(output) == 0 {
		return nil, nil
	}
	if output[len(output)-1] != 0 {
		return nil, errors.New("git ls-files returned an incomplete staged-entry record")
	}
	entries := bytes.Split(output[:len(output)-1], []byte{0})
	if len(entries) > turnCheckpointEntryLimit {
		return nil, ErrTurnCheckpointOutputLimit
	}
	var preserved bytes.Buffer
	for _, entry := range entries {
		record, preserve, err := missingTurnCheckpointIndexEntry(entry, g.workDir)
		if err != nil {
			return nil, err
		}
		if preserve {
			preserved.Write(record)
		}
	}
	return preserved.Bytes(), nil
}

func missingTurnCheckpointIndexEntry(entry []byte, workDir string) ([]byte, bool, error) {
	separator := bytes.IndexByte(entry, '\t')
	if len(entry) < 4 || entry[1] != ' ' || separator < 0 {
		return nil, false, errors.New("git ls-files returned an invalid staged-entry record")
	}
	metadata := strings.Fields(string(entry[2:separator]))
	path := entry[separator+1:]
	if len(metadata) != 3 || !safeTurnCheckpointPath(workDir, path) {
		return nil, false, errors.New("git index contains invalid staged-entry metadata")
	}
	if metadata[0] != "160000" && entry[0] != 'S' && entry[0] != 's' {
		return nil, false, nil
	}
	_, err := os.Lstat(filepath.Join(workDir, filepath.FromSlash(string(path))))
	if err == nil {
		return nil, false, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, false, err
	}
	record := make([]byte, 0, separator+len(path)+1)
	record = append(record, strings.Join(metadata, " ")...)
	record = append(record, '\t')
	record = append(record, path...)
	record = append(record, 0)
	return record, true, nil
}

func (g *GitOperator) preflightTurnCheckpointBlobs(ctx context.Context, indexPath string) error {
	output, err := g.turnCheckpointOutput(ctx, indexPath, "ls-files", "--modified", "--others", "--exclude-standard", "-z")
	if errors.Is(err, ErrTurnCheckpointOutputLimit) {
		return turnCheckpointFailure(turnchanges.ReasonEntryLimit, err)
	}
	if err != nil {
		return turnCheckpointFailure(turnchanges.ReasonCaptureFailed, err)
	}
	return validateTurnCheckpointCandidateBytes(output, g.workDir, turnCheckpointBlobLimit)
}

func validateTurnCheckpointCandidateBytes(output []byte, workDir string, limit int64) error {
	if len(output) == 0 {
		return nil
	}
	if output[len(output)-1] != 0 {
		return turnCheckpointFailure(turnchanges.ReasonCaptureFailed, errors.New("git ls-files returned an incomplete candidate path"))
	}
	paths := bytes.Split(output[:len(output)-1], []byte{0})
	if len(paths) > turnCheckpointEntryLimit {
		return turnCheckpointFailure(turnchanges.ReasonEntryLimit, ErrTurnCheckpointOutputLimit)
	}
	var total int64
	for _, path := range paths {
		if !safeTurnCheckpointPath(workDir, path) {
			return turnCheckpointFailure(turnchanges.ReasonUnsafeGitState, errors.New("git index contains a path outside the checkout"))
		}
		info, err := os.Lstat(filepath.Join(workDir, filepath.FromSlash(string(path))))
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return turnCheckpointFailure(turnchanges.ReasonCaptureFailed, err)
		}
		if info.IsDir() {
			continue
		}
		if !info.Mode().IsRegular() && info.Mode()&os.ModeSymlink == 0 {
			return turnCheckpointFailure(turnchanges.ReasonUnsafeGitState, errors.New("turn checkpoint candidate is not a regular file or symlink"))
		}
		if info.Size() > limit-total {
			return turnCheckpointFailure(turnchanges.ReasonSizeLimit, errors.New("turn checkpoint candidate content exceeds the endpoint byte limit"))
		}
		total += info.Size()
	}
	return nil
}

func (g *GitOperator) refreshTurnCheckpointIndexFlags(ctx context.Context, indexPath string) error {
	output, err := g.turnCheckpointOutput(ctx, indexPath, "ls-files", "-v", "-z")
	if err != nil {
		return err
	}
	assumePaths, skipPaths, err := turnCheckpointFlaggedPaths(output, g.workDir)
	if err != nil {
		return err
	}
	if len(assumePaths) > 0 {
		if _, err := g.turnCheckpointOutputWithInput(ctx, indexPath, nil, assumePaths, "update-index", "--no-assume-unchanged", "-z", "--stdin"); err != nil {
			return err
		}
	}
	if len(skipPaths) > 0 {
		if _, err := g.turnCheckpointOutputWithInput(ctx, indexPath, nil, skipPaths, "update-index", "--no-skip-worktree", "-z", "--stdin"); err != nil {
			return err
		}
	}
	return nil
}

func turnCheckpointFlaggedPaths(output []byte, workDir string) ([]byte, []byte, error) {
	if len(output) == 0 {
		return nil, nil, nil
	}
	if output[len(output)-1] != 0 {
		return nil, nil, errors.New("git ls-files returned an incomplete NUL record")
	}
	var assumePaths, skipPaths bytes.Buffer
	entries := bytes.Split(output[:len(output)-1], []byte{0})
	if len(entries) > turnCheckpointEntryLimit {
		return nil, nil, ErrTurnCheckpointOutputLimit
	}
	for _, entry := range entries {
		path, assume, skip, err := turnCheckpointIndexFlagAction(entry, workDir)
		if err != nil {
			return nil, nil, err
		}
		if assume {
			assumePaths.Write(path)
			assumePaths.WriteByte(0)
		}
		if skip {
			skipPaths.Write(path)
			skipPaths.WriteByte(0)
		}
	}
	return assumePaths.Bytes(), skipPaths.Bytes(), nil
}

func turnCheckpointIndexFlagAction(entry []byte, workDir string) ([]byte, bool, bool, error) {
	if len(entry) < 3 || entry[1] != ' ' {
		return nil, false, false, errors.New("git ls-files returned an invalid index flag record")
	}
	flag, path := entry[0], entry[2:]
	if !safeTurnCheckpointPath(workDir, path) {
		return nil, false, false, errors.New("git index contains a path outside the checkout")
	}
	assume := flag >= 'a' && flag <= 'z'
	if flag != 'S' && flag != 's' {
		return path, assume, false, nil
	}
	_, err := os.Lstat(filepath.Join(workDir, filepath.FromSlash(string(path))))
	if err == nil {
		return path, assume, true, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return path, assume, false, nil
	}
	return nil, false, false, err
}

func safeTurnCheckpointPath(workDir string, path []byte) bool {
	if len(path) == 0 {
		return false
	}
	value := string(path)
	if filepath.IsAbs(value) || filepath.IsAbs(filepath.FromSlash(value)) {
		return false
	}
	candidate := filepath.Join(workDir, filepath.FromSlash(value))
	relative, err := filepath.Rel(workDir, candidate)
	if err != nil || filepath.IsAbs(relative) || relative == ".." {
		return false
	}
	return !strings.HasPrefix(relative, ".."+string(os.PathSeparator))
}

func (g *GitOperator) turnCheckpointOutput(ctx context.Context, indexPath string, args ...string) ([]byte, error) {
	return g.turnCheckpointOutputWithEnvironment(ctx, indexPath, nil, args...)
}

func (g *GitOperator) turnCheckpointOutputWithEnvironment(
	ctx context.Context,
	indexPath string,
	environmentOverrides map[string]string,
	args ...string,
) ([]byte, error) {
	return g.turnCheckpointOutputWithInput(ctx, indexPath, environmentOverrides, nil, args...)
}

func (g *GitOperator) turnCheckpointOutputWithInput(
	ctx context.Context,
	indexPath string,
	environmentOverrides map[string]string,
	input []byte,
	args ...string,
) ([]byte, error) {
	if err := validateGitCommandArgs(args); err != nil {
		return nil, turnCheckpointFailure(turnchanges.ReasonUnsafeGitState, err)
	}
	stdout := &boundedGitOutput{limit: turnCheckpointOutputLimit}
	stderr := &boundedGitOutput{limit: turnCheckpointStderrLimit}
	environment := turnCheckpointEnvironment(g.environmentValues(), indexPath, environmentOverrides)
	checkpointArgs := append([]string{"-c", "core.fsync=" + turnCheckpointFsync}, args...)
	runErr, contextErr := subproc.RunGitAfterAcquire(ctx, subproc.GitLifecycle, 30*time.Second, func(execCtx context.Context) *exec.Cmd {
		cmd := subproc.NewGitCommand(execCtx, checkpointArgs...)
		cmd.Dir = g.workDir
		cmd.Env = environment
		cmd.Stdout = stdout
		cmd.Stderr = stderr
		if input != nil {
			cmd.Stdin = bytes.NewReader(input)
		}
		return cmd
	})
	if contextErr != nil {
		return nil, contextErr
	}
	if stdout.truncated || stderr.truncated {
		return nil, ErrTurnCheckpointOutputLimit
	}
	if runErr != nil {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = runErr.Error()
		}
		return nil, fmt.Errorf("git %s: %s", args[0], message)
	}
	return append([]byte(nil), stdout.Bytes()...), nil
}

type boundedGitOutput struct {
	bytes.Buffer
	limit     int
	truncated bool
}

func (b *boundedGitOutput) Write(data []byte) (int, error) {
	remaining := b.limit - b.Len()
	if remaining <= 0 {
		b.truncated = true
		return len(data), nil
	}
	if len(data) > remaining {
		_, _ = b.Buffer.Write(data[:remaining])
		b.truncated = true
		return len(data), nil
	}
	return b.Buffer.Write(data)
}

func turnCheckpointEnvironment(environment []string, indexPath string, overrides map[string]string) []string {
	filtered := make([]string, 0, len(environment)+len(overrides)+1)
	overridden := make(map[string]struct{}, len(overrides))
	for key := range overrides {
		overridden[key] = struct{}{}
	}
	for _, value := range environment {
		key, _, ok := strings.Cut(value, "=")
		if !ok || isTurnCheckpointGitBinding(key) {
			continue
		}
		if _, exists := overridden[key]; exists {
			continue
		}
		filtered = append(filtered, value)
	}
	if indexPath != "" {
		filtered = append(filtered, "GIT_INDEX_FILE="+indexPath)
	}
	for key, value := range overrides {
		filtered = append(filtered, key+"="+value)
	}
	return filtered
}

func isTurnCheckpointGitBinding(key string) bool {
	for _, prefix := range []string{
		"GIT_DIR", "GIT_WORK_TREE", "GIT_COMMON_DIR", "GIT_INDEX_FILE", "GIT_OBJECT_DIRECTORY",
		"GIT_ALTERNATE_OBJECT_DIRECTORIES", "GIT_IMPLICIT_WORK_TREE", "GIT_PREFIX",
		"GIT_CEILING_DIRECTORIES", "GIT_DISCOVERY_ACROSS_FILESYSTEM",
	} {
		if strings.HasPrefix(key, prefix) {
			return true
		}
	}
	return false
}
