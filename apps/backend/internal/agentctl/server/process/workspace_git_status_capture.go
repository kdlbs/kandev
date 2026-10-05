package process

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/agentctl/types"
)

var (
	errGitStatusEvidenceChanged    = errors.New("git status evidence changed during capture")
	errGitStatusDetailsUnavailable = errors.New("git status details unavailable")
)

type gitStatusBasicCapture struct {
	status      types.GitStatusUpdate
	fingerprint string
	job         *gitStatusEnrichmentJob
}

type gitStatusEnrichmentJob struct {
	status                  types.GitStatusUpdate
	fingerprint             string
	done                    chan struct{}
	err                     error
	completed               bool
	indexSnapshot           string
	indexCleanup            func()
	indexSourceInfo         os.FileInfo
	indexDigest             string
	canonicalWorkDir        string
	gitDir                  string
	headCommit              string
	branch                  string
	remoteBranch            string
	remoteHeadOID           string
	baseRef                 string
	baseOID                 string
	aheadRef                string
	aheadOID                string
	comparison              ComparisonResolution
	comparisonOID           string
	comparisonGeneration    uint64
	environmentGeneration   uint64
	fileEvidence            map[string]gitStatusFileEvidence
	correctionPermitted     bool
	correctionRequested     bool
	contentEvidenceComplete bool
	explicitRetry           bool
	unavailablePublished    bool
}

type gitStatusFileEvidence struct {
	Exists         bool
	Size           int64
	Mode           os.FileMode
	ModifiedNanos  int64
	Identity       string
	LinkTarget     string
	IsSubmodule    bool
	SubmoduleHead  string
	ContentDigest  string
	ContentChecked bool
}

type gitStatusCaptureSeed struct {
	canonicalWorkDir string
	gitDir           string
	indexSourceInfo  os.FileInfo
	indexDigest      [32]byte
	indexSnapshot    string
	indexCleanup     func()
	fileEvidence     map[string]gitStatusFileEvidence
	contentComplete  bool
}

type gitStatusComparisonEvidence struct {
	baseRef       string
	baseOID       string
	aheadRef      string
	aheadOID      string
	remoteHeadOID string
	comparisonOID string
}

func cloneGitStatusUpdate(status types.GitStatusUpdate) types.GitStatusUpdate {
	status.Modified = append([]string(nil), status.Modified...)
	status.Added = append([]string(nil), status.Added...)
	status.Deleted = append([]string(nil), status.Deleted...)
	status.Untracked = append([]string(nil), status.Untracked...)
	status.Renamed = append([]string(nil), status.Renamed...)
	if status.Files != nil {
		files := make(map[string]types.FileInfo, len(status.Files))
		for path, file := range status.Files {
			file.IsSymlink = cloneBoolPointer(file.IsSymlink)
			file.StagedChange = cloneGitStatusFacet(file.StagedChange)
			file.UnstagedChange = cloneGitStatusFacet(file.UnstagedChange)
			files[path] = file
		}
		status.Files = files
	}
	return status
}

func cloneBoolPointer(value *bool) *bool {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func cloneGitStatusFacet(facet *types.FileChangeFacet) *types.FileChangeFacet {
	if facet == nil {
		return nil
	}
	copy := *facet
	copy.IsSymlink = cloneBoolPointer(facet.IsSymlink)
	return &copy
}

func gitStatusValueFingerprint(status types.GitStatusUpdate) string {
	status.Timestamp = time.Time{}
	status.TrackerID = ""
	status.TrackerEpoch = 0
	status.SnapshotRevision = 0
	encoded, err := json.Marshal(status)
	if err != nil {
		return fmt.Sprintf("unhashable:%s:%d", status.RepositoryName, len(status.Files))
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:])
}

func (wt *WorkspaceTracker) publishGitStatus(status types.GitStatusUpdate, ordinal uint64, fingerprint string, retryOption ...bool) (types.GitStatusUpdate, bool) {
	explicitRetry := len(retryOption) > 0 && retryOption[0]
	wt.gitStatusPublishMu.Lock()
	defer wt.gitStatusPublishMu.Unlock()

	wt.mu.Lock()
	if ordinal < wt.gitStatusLatestID {
		current := cloneGitStatusUpdate(wt.currentStatus)
		wt.mu.Unlock()
		return current, false
	}
	wt.gitStatusLatestID = ordinal
	status = cloneGitStatusUpdate(status)
	status.TrackerID = wt.gitStatusTrackerID
	status.TrackerEpoch = wt.gitStatusEpoch

	if fingerprint != "" && fingerprint == wt.gitStatusFingerprint && wt.currentStatus.StatusState == gitStatusStateReady && status.StatusState == gitStatusStateReady {
		if wt.currentStatus.DetailState == gitStatusDetailReady {
			current := cloneGitStatusUpdate(wt.currentStatus)
			current.Timestamp = status.Timestamp
			wt.currentStatus = cloneGitStatusUpdate(current)
			wt.mu.Unlock()
			return current, false
		}
		if wt.currentStatus.DetailState == gitStatusDetailPending && status.DetailState == gitStatusDetailPending {
			status.SnapshotRevision = wt.currentStatus.SnapshotRevision
			wt.currentStatus = cloneGitStatusUpdate(status)
			wt.mu.Unlock()
			return status, false
		}
		if wt.currentStatus.DetailState == gitStatusDetailUnavailable && status.DetailState == gitStatusDetailPending && !explicitRetry {
			current := cloneGitStatusUpdate(wt.currentStatus)
			current.Timestamp = status.Timestamp
			wt.currentStatus = cloneGitStatusUpdate(current)
			wt.mu.Unlock()
			return current, false
		}
	}

	wt.gitStatusRevision++
	wt.gitStatusDetailSourceRevision = 0
	status.SnapshotRevision = wt.gitStatusRevision
	wt.currentStatus = cloneGitStatusUpdate(status)
	wt.gitStatusFingerprint = fingerprint
	wt.mu.Unlock()

	wt.notifyWorkspaceStreamGitStatus(cloneGitStatusUpdate(status))
	return status, true
}

func (wt *WorkspaceTracker) captureBasicGitStatus(ctx context.Context) (gitStatusBasicCapture, error) {
	status := wt.newGitStatusUpdate()
	comparison, comparisonGeneration := wt.comparisonSnapshot()
	environmentGeneration := wt.gitEnvironmentVersion()
	if comparison.Explicit {
		status.ComparisonTarget = comparison.Display
		status.ComparisonStatus = comparison.Status
		status.ComparisonErrorCode = comparison.ErrorCode
	}
	if wt.gitIndexPath == "" {
		status.StatusState = gitStatusStateUnavailable
		status.ErrorCode = gitStatusErrorNoRepository
		status.DetailState = gitStatusDetailUnavailable
		return gitStatusBasicCapture{status: status, fingerprint: gitStatusValueFingerprint(status)}, nil
	}
	seed, err := wt.captureBasicGitStatusSeed(ctx, &status)
	if err != nil {
		return gitStatusBasicCapture{}, err
	}
	keepIndex := false
	defer func() {
		if !keepIndex {
			seed.indexCleanup()
		}
	}()
	comparisonEvidence, err := wt.captureGitStatusComparisonEvidence(ctx, status, comparison)
	if err != nil {
		return gitStatusBasicCapture{}, err
	}
	if err := wt.validateBasicGitStatusCapture(ctx, status, seed, comparison, comparisonGeneration, environmentGeneration, comparisonEvidence); err != nil {
		return gitStatusBasicCapture{}, err
	}
	if !seed.contentComplete {
		status.DetailState = gitStatusDetailUnavailable
		setGitStatusFileDiffState(&status, gitStatusDetailUnavailable)
	}
	status.StatusState = gitStatusStateReady
	status.FilesComplete = true
	if status.DetailState == "" {
		status.DetailState = gitStatusDetailPending
	}
	setGitStatusFileDiffState(&status, status.DetailState)
	job, fingerprint := newGitStatusEnrichmentJob(status, seed, comparison, comparisonGeneration, environmentGeneration, comparisonEvidence)
	keepIndex = true
	return gitStatusBasicCapture{status: status, fingerprint: fingerprint, job: job}, nil
}

func (wt *WorkspaceTracker) captureBasicGitStatusSeed(ctx context.Context, status *types.GitStatusUpdate) (gitStatusCaptureSeed, error) {
	canonicalWorkDir, err := filepath.EvalSymlinks(wt.workDir)
	if err != nil {
		return gitStatusCaptureSeed{}, fmt.Errorf("resolve workspace identity: %w", err)
	}
	canonicalWorkDir, err = filepath.Abs(canonicalWorkDir)
	if err != nil {
		return gitStatusCaptureSeed{}, fmt.Errorf("make workspace identity absolute: %w", err)
	}
	gitDir, err := wt.gitDirectory(ctx)
	if err != nil {
		return gitStatusCaptureSeed{}, err
	}
	indexSourceInfo, err := os.Stat(wt.gitIndexPath)
	if err != nil {
		return gitStatusCaptureSeed{}, fmt.Errorf("stat git index before status: %w", err)
	}
	indexDigest, err := digestFile(ctx, wt.gitIndexPath)
	if err != nil {
		return gitStatusCaptureSeed{}, fmt.Errorf("digest git index before status: %w", err)
	}
	if err := wt.getGitBranchIdentity(ctx, status); err != nil {
		return gitStatusCaptureSeed{}, err
	}
	indexSnapshot, indexCleanup, err := wt.parseGitStatusOutputWithSnapshot(ctx, status)
	if err != nil {
		return gitStatusCaptureSeed{}, err
	}
	fileEvidence, contentComplete, err := captureGitStatusFileEvidence(ctx, wt, wt.workDir, *status)
	if err != nil {
		indexCleanup()
		return gitStatusCaptureSeed{}, err
	}
	return gitStatusCaptureSeed{
		canonicalWorkDir: canonicalWorkDir,
		gitDir:           gitDir,
		indexSourceInfo:  indexSourceInfo,
		indexDigest:      indexDigest,
		indexSnapshot:    indexSnapshot,
		indexCleanup:     indexCleanup,
		fileEvidence:     fileEvidence,
		contentComplete:  contentComplete,
	}, nil
}

func (wt *WorkspaceTracker) captureGitStatusComparisonEvidence(ctx context.Context, status types.GitStatusUpdate, comparison ComparisonResolution) (gitStatusComparisonEvidence, error) {
	evidence := gitStatusComparisonEvidence{baseRef: wt.resolveBaseBranch(ctx), aheadRef: wt.resolveAheadBehindRef(ctx)}
	var err error
	evidence.baseOID, err = wt.gitRefOID(ctx, evidence.baseRef)
	if err != nil {
		return gitStatusComparisonEvidence{}, err
	}
	evidence.aheadOID, err = wt.gitRefOID(ctx, evidence.aheadRef)
	if err != nil {
		return gitStatusComparisonEvidence{}, err
	}
	evidence.remoteHeadOID, err = wt.gitRefOID(ctx, status.RemoteBranch)
	if err != nil {
		return gitStatusComparisonEvidence{}, err
	}
	evidence.comparisonOID, err = wt.gitRefOID(ctx, comparison.Ref)
	return evidence, err
}

func (wt *WorkspaceTracker) validateBasicGitStatusCapture(
	ctx context.Context,
	status types.GitStatusUpdate,
	seed gitStatusCaptureSeed,
	comparison ComparisonResolution,
	comparisonGeneration, environmentGeneration uint64,
	evidence gitStatusComparisonEvidence,
) error {
	if err := wt.validateBasicGitStatusRepository(ctx, status, seed); err != nil {
		return err
	}
	if err := wt.validateBasicGitStatusIndex(ctx, seed); err != nil {
		return err
	}
	return wt.validateBasicGitStatusEvidence(ctx, status, seed, comparison, comparisonGeneration, environmentGeneration, evidence)
}

func (wt *WorkspaceTracker) validateBasicGitStatusRepository(ctx context.Context, status types.GitStatusUpdate, seed gitStatusCaptureSeed) error {
	endStatus := wt.newGitStatusUpdate()
	if err := wt.getGitBranchIdentity(ctx, &endStatus); err != nil {
		return err
	}
	endGitDir, err := wt.gitDirectory(ctx)
	if err != nil {
		return err
	}
	workDir, err := filepath.EvalSymlinks(wt.workDir)
	if err != nil {
		return fmt.Errorf("recheck workspace identity: %w", err)
	}
	workDir, err = filepath.Abs(workDir)
	if err != nil {
		return err
	}
	if endStatus.Branch != status.Branch || endStatus.HeadCommit != status.HeadCommit || endStatus.RemoteBranch != status.RemoteBranch ||
		endGitDir != seed.gitDir || filepath.Clean(workDir) != filepath.Clean(seed.canonicalWorkDir) {
		return errGitStatusEvidenceChanged
	}
	return nil
}

func (wt *WorkspaceTracker) validateBasicGitStatusIndex(ctx context.Context, seed gitStatusCaptureSeed) error {
	indexInfo, err := os.Stat(wt.gitIndexPath)
	if err != nil {
		return fmt.Errorf("stat git index after status: %w", err)
	}
	if !os.SameFile(seed.indexSourceInfo, indexInfo) {
		return errGitStatusEvidenceChanged
	}
	indexDigest, err := digestFile(ctx, wt.gitIndexPath)
	if err != nil {
		return fmt.Errorf("digest git index after status: %w", err)
	}
	if indexDigest != seed.indexDigest {
		return errGitStatusEvidenceChanged
	}
	return nil
}

func (wt *WorkspaceTracker) validateBasicGitStatusEvidence(
	ctx context.Context,
	status types.GitStatusUpdate,
	seed gitStatusCaptureSeed,
	comparison ComparisonResolution,
	comparisonGeneration, environmentGeneration uint64,
	evidence gitStatusComparisonEvidence,
) error {
	endEvidence, err := wt.captureGitStatusComparisonEvidence(ctx, status, comparison)
	if err != nil {
		return err
	}
	fileEvidence, contentComplete, err := captureGitStatusFileEvidence(ctx, wt, wt.workDir, status)
	if err != nil {
		return err
	}
	currentComparison, currentGeneration := wt.comparisonSnapshot()
	if err := ctx.Err(); err != nil {
		return err
	}
	if evidence != endEvidence ||
		contentComplete != seed.contentComplete || !sameGitStatusFileEvidence(seed.fileEvidence, fileEvidence) ||
		currentGeneration != comparisonGeneration || currentComparison != comparison || wt.gitEnvironmentVersion() != environmentGeneration {
		return errGitStatusEvidenceChanged
	}
	return nil
}

func newGitStatusEnrichmentJob(
	status types.GitStatusUpdate,
	seed gitStatusCaptureSeed,
	comparison ComparisonResolution,
	comparisonGeneration, environmentGeneration uint64,
	evidence gitStatusComparisonEvidence,
) (*gitStatusEnrichmentJob, string) {
	indexDigest := hex.EncodeToString(seed.indexDigest[:])
	fingerprint := gitStatusCaptureFingerprint(
		gitStatusValueFingerprint(status), seed.canonicalWorkDir, seed.gitDir, indexDigest, status.HeadCommit, status.Branch,
		status.RemoteBranch, evidence.remoteHeadOID, evidence.baseRef, evidence.baseOID, evidence.aheadRef, evidence.aheadOID,
		comparison, evidence.comparisonOID, comparisonGeneration, environmentGeneration, seed.fileEvidence,
	)
	job := &gitStatusEnrichmentJob{
		status:                  cloneGitStatusUpdate(status),
		fingerprint:             fingerprint,
		done:                    make(chan struct{}),
		indexSnapshot:           seed.indexSnapshot,
		indexCleanup:            seed.indexCleanup,
		indexSourceInfo:         seed.indexSourceInfo,
		indexDigest:             indexDigest,
		canonicalWorkDir:        seed.canonicalWorkDir,
		gitDir:                  seed.gitDir,
		headCommit:              status.HeadCommit,
		branch:                  status.Branch,
		remoteBranch:            status.RemoteBranch,
		remoteHeadOID:           evidence.remoteHeadOID,
		baseRef:                 evidence.baseRef,
		baseOID:                 evidence.baseOID,
		aheadRef:                evidence.aheadRef,
		aheadOID:                evidence.aheadOID,
		comparison:              comparison,
		comparisonOID:           evidence.comparisonOID,
		comparisonGeneration:    comparisonGeneration,
		environmentGeneration:   environmentGeneration,
		fileEvidence:            seed.fileEvidence,
		correctionPermitted:     true,
		contentEvidenceComplete: seed.contentComplete,
	}
	return job, fingerprint
}

func (wt *WorkspaceTracker) comparisonSnapshot() (ComparisonResolution, uint64) {
	wt.mu.RLock()
	defer wt.mu.RUnlock()
	resolution := ComparisonResolution{}
	if wt.comparisonTarget != nil {
		resolution = ComparisonResolution{
			Explicit:  true,
			Ref:       wt.comparisonTargetRef,
			Display:   wt.comparisonTarget.DisplayIdentity(),
			Status:    wt.comparisonTargetStatus,
			ErrorCode: wt.comparisonTargetErrorCode,
		}
	}
	return resolution, wt.comparisonGeneration
}

func (wt *WorkspaceTracker) gitDirectory(ctx context.Context) (string, error) {
	out, err := wt.runGitOutput(ctx, "rev-parse", "--absolute-git-dir")
	if err != nil {
		return "", err
	}
	path := strings.TrimSpace(string(out))
	if !filepath.IsAbs(path) {
		path = filepath.Join(wt.workDir, path)
	}
	return filepath.Clean(path), nil
}

func (wt *WorkspaceTracker) gitRefOID(ctx context.Context, ref string) (string, error) {
	if ref == "" {
		return "", nil
	}
	if !gitObjectIDPattern.MatchString(ref) {
		rest, hasOriginPrefix := strings.CutPrefix(ref, "origin/")
		check := ref
		if hasOriginPrefix {
			check = rest
		}
		if !safeBranchRefPattern.MatchString(check) || strings.Contains(check, "..") || strings.HasSuffix(check, ".lock") {
			return "", nil
		}
	}
	out, err := wt.runGitOutput(ctx, "rev-parse", "--verify", ref+"^{commit}")
	if err != nil {
		return "", nil
	}
	return strings.TrimSpace(string(out)), nil
}

func captureGitStatusFileEvidence(ctx context.Context, wt *WorkspaceTracker, root string, status types.GitStatusUpdate) (map[string]gitStatusFileEvidence, bool, error) {
	paths := make(map[string]struct{}, len(status.Files)*2)
	for path, file := range status.Files {
		paths[path] = struct{}{}
		if file.OldPath != "" {
			paths[file.OldPath] = struct{}{}
		}
		if file.StagedChange != nil && file.StagedChange.OldPath != "" {
			paths[file.StagedChange.OldPath] = struct{}{}
		}
		if file.UnstagedChange != nil && file.UnstagedChange.OldPath != "" {
			paths[file.UnstagedChange.OldPath] = struct{}{}
		}
	}
	ordered := make([]string, 0, len(paths))
	for path := range paths {
		ordered = append(ordered, path)
	}
	sort.Strings(ordered)
	evidence := make(map[string]gitStatusFileEvidence, len(ordered))
	complete := true
	for _, path := range ordered {
		if err := ctx.Err(); err != nil {
			return nil, false, err
		}
		entry, err := captureOneGitStatusFile(ctx, wt, root, path)
		if err != nil {
			return nil, false, err
		}
		if entry.Exists && entry.Mode.IsRegular() && entry.Size <= maxDiffFileSize && !entry.ContentChecked {
			complete = false
		}
		evidence[path] = entry
	}
	return evidence, complete, nil
}

func captureOneGitStatusFile(ctx context.Context, wt *WorkspaceTracker, root, path string) (gitStatusFileEvidence, error) {
	relative, err := gitStatusRelativePath(path)
	if err != nil {
		return gitStatusFileEvidence{}, err
	}
	fullPath := filepath.Join(root, relative)
	before, err := os.Lstat(fullPath)
	if errors.Is(err, os.ErrNotExist) {
		return gitStatusFileEvidence{}, nil
	}
	if err != nil {
		return gitStatusFileEvidence{}, fmt.Errorf("stat changed path: %w", err)
	}
	evidence := fileEvidenceFromInfo(before)
	evidence, err = captureGitStatusFileContent(ctx, wt, relative, fullPath, before, evidence)
	if err != nil {
		return gitStatusFileEvidence{}, err
	}
	after, err := os.Lstat(fullPath)
	if err != nil || !sameGitStatusFileInfo(before, after) {
		return gitStatusFileEvidence{}, errGitStatusEvidenceChanged
	}
	return evidence, nil
}

func gitStatusRelativePath(path string) (string, error) {
	relative := filepath.Clean(filepath.FromSlash(path))
	if relative == "." || filepath.IsAbs(relative) || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("invalid repository-relative status path")
	}
	return relative, nil
}

func captureGitStatusFileContent(
	ctx context.Context,
	wt *WorkspaceTracker,
	relative, fullPath string,
	before os.FileInfo,
	evidence gitStatusFileEvidence,
) (gitStatusFileEvidence, error) {
	switch {
	case before.Mode()&os.ModeSymlink != 0:
		target, err := os.Readlink(fullPath)
		if err != nil {
			return gitStatusFileEvidence{}, fmt.Errorf("read changed symlink: %w", err)
		}
		evidence.LinkTarget = target
		digest := sha256.Sum256([]byte(target))
		evidence.ContentDigest = hex.EncodeToString(digest[:])
		evidence.ContentChecked = true
	case before.IsDir():
		var err error
		evidence, err = captureGitSubmoduleHead(ctx, wt, relative, fullPath, evidence)
		if err != nil {
			return gitStatusFileEvidence{}, err
		}
	case before.Mode().IsRegular() && before.Size() <= maxDiffFileSize:
		digest, err := digestFile(ctx, fullPath)
		if err != nil {
			return gitStatusFileEvidence{}, err
		}
		evidence.ContentDigest = hex.EncodeToString(digest[:])
		evidence.ContentChecked = true
	}
	return evidence, nil
}

func captureGitSubmoduleHead(ctx context.Context, wt *WorkspaceTracker, relative, fullPath string, evidence gitStatusFileEvidence) (gitStatusFileEvidence, error) {
	if _, err := os.Lstat(filepath.Join(fullPath, ".git")); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return evidence, nil
		}
		return gitStatusFileEvidence{}, err
	}
	evidence.IsSubmodule = true
	out, err := wt.runGitOutput(ctx, "-C", relative, "rev-parse", "HEAD")
	if err != nil {
		return gitStatusFileEvidence{}, err
	}
	evidence.SubmoduleHead = strings.TrimSpace(string(out))
	if !gitObjectIDPattern.MatchString(evidence.SubmoduleHead) {
		return gitStatusFileEvidence{}, errGitStatusEvidenceChanged
	}
	return evidence, nil
}

func fileEvidenceFromInfo(info os.FileInfo) gitStatusFileEvidence {
	return gitStatusFileEvidence{
		Exists:        true,
		Size:          info.Size(),
		Mode:          info.Mode(),
		ModifiedNanos: info.ModTime().UnixNano(),
		Identity:      gitStatusFileIdentity(info),
	}
}

func gitStatusFileIdentity(info os.FileInfo) string {
	if info == nil || info.Sys() == nil {
		return ""
	}
	value := reflect.ValueOf(info.Sys())
	if value.Kind() == reflect.Pointer {
		value = value.Elem()
	}
	if value.Kind() != reflect.Struct {
		return fmt.Sprintf("%T", info.Sys())
	}
	device := value.FieldByName("Dev")
	inode := value.FieldByName("Ino")
	if device.IsValid() && inode.IsValid() {
		return fmt.Sprintf("%v:%v", device.Interface(), inode.Interface())
	}
	return fmt.Sprintf("%T", info.Sys())
}

func sameGitStatusFileInfo(left, right os.FileInfo) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return os.SameFile(left, right) && left.Size() == right.Size() && left.Mode() == right.Mode() && left.ModTime().Equal(right.ModTime())
}

func digestFile(ctx context.Context, path string) (digest [32]byte, resultErr error) {
	file, err := os.Open(path)
	if err != nil {
		return digest, err
	}
	defer func() {
		if err := file.Close(); resultErr == nil {
			resultErr = err
		}
	}()
	hasher := sha256.New()
	buffer := make([]byte, 32*1024)
	for {
		if err := ctx.Err(); err != nil {
			return digest, err
		}
		count, readErr := file.Read(buffer)
		if count > 0 {
			_, _ = hasher.Write(buffer[:count])
		}
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			return digest, readErr
		}
	}
	copy(digest[:], hasher.Sum(nil))
	return digest, nil
}

func gitStatusCaptureFingerprint(
	statusFingerprint, canonicalWorkDir, gitDir, indexDigest, head, branch, remoteBranch, remoteHeadOID,
	baseRef, baseOID, aheadRef, aheadOID string, comparison ComparisonResolution, comparisonOID string,
	comparisonGeneration, environmentGeneration uint64, fileEvidence map[string]gitStatusFileEvidence,
) string {
	keys := make([]string, 0, len(fileEvidence))
	for path := range fileEvidence {
		keys = append(keys, path)
	}
	sort.Strings(keys)
	hasher := sha256.New()
	for _, value := range []string{
		statusFingerprint, canonicalWorkDir, gitDir, indexDigest, head, branch, remoteBranch, remoteHeadOID,
		baseRef, baseOID, aheadRef, aheadOID, comparison.Ref, comparison.Display, comparison.Status,
		comparison.ErrorCode, comparisonOID, fmt.Sprint(comparison.Explicit), fmt.Sprint(comparisonGeneration), fmt.Sprint(environmentGeneration),
	} {
		_, _ = io.WriteString(hasher, value)
		_, _ = hasher.Write([]byte{0})
	}
	for _, path := range keys {
		encoded, _ := json.Marshal(fileEvidence[path])
		_, _ = io.WriteString(hasher, path)
		_, _ = hasher.Write([]byte{0})
		_, _ = hasher.Write(encoded)
	}
	return hex.EncodeToString(hasher.Sum(nil))
}

func sameGitStatusFileEvidence(left, right map[string]gitStatusFileEvidence) bool {
	if len(left) != len(right) {
		return false
	}
	for path, expected := range left {
		actual, ok := right[path]
		if !ok || expected.Exists != actual.Exists || expected.Size != actual.Size || expected.Mode != actual.Mode ||
			expected.ModifiedNanos != actual.ModifiedNanos || expected.Identity != actual.Identity || expected.LinkTarget != actual.LinkTarget ||
			expected.ContentDigest != actual.ContentDigest || expected.ContentChecked != actual.ContentChecked ||
			expected.IsSubmodule != actual.IsSubmodule || expected.SubmoduleHead != actual.SubmoduleHead {
			return false
		}
	}
	return true
}
