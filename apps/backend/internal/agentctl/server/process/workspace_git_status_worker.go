package process

import (
	"context"
	"encoding/hex"
	"os"
	"path/filepath"

	"github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/common/subproc"
	"go.uber.org/zap"
)

func (wt *WorkspaceTracker) scheduleGitStatusEnrichment(job *gitStatusEnrichmentJob) {
	if job == nil || job.indexCleanup == nil || job.status.DetailState != gitStatusDetailPending || !job.contentEvidenceComplete {
		if job != nil && job.indexCleanup != nil {
			job.indexCleanup()
		}
		wt.completeGitStatusEnrichment(job, errGitStatusDetailsUnavailable)
		return
	}
	wt.gitStatusEnrichmentMu.Lock()
	if wt.gitStatusEnrichmentRun {
		queued := wt.queueGitStatusEnrichmentLocked(job)
		wt.gitStatusEnrichmentMu.Unlock()
		if !queued {
			job.indexCleanup()
			wt.completeGitStatusEnrichment(job, nil)
		}
		return
	}
	wt.gitStatusObserveMu.Lock()
	if wt.cancelCtx != nil && wt.cancelCtx.Err() != nil {
		wt.gitStatusObserveMu.Unlock()
		completeGitStatusEnrichmentLocked(job, wt.cancelCtx.Err())
		wt.gitStatusEnrichmentMu.Unlock()
		job.indexCleanup()
		return
	}
	wt.gitStatusObserveWG.Add(1)
	wt.gitStatusObserveMu.Unlock()
	wt.gitStatusEnrichmentRun = true
	wt.gitStatusEnrichmentCurrent = job.fingerprint
	wt.gitStatusEnrichmentJob = job
	wt.gitStatusEnrichmentMu.Unlock()
	go wt.runGitStatusEnrichmentQueue(job)
}

// queueGitStatusEnrichmentLocked keeps only the latest distinct successor and
// lets explicit retries replace an already-published failed attempt.
func (wt *WorkspaceTracker) queueGitStatusEnrichmentLocked(job *gitStatusEnrichmentJob) bool {
	if next := wt.gitStatusEnrichmentNext; next != nil && job.fingerprint == next.fingerprint {
		return false
	}
	if job.fingerprint == wt.gitStatusEnrichmentCurrent {
		current := wt.gitStatusEnrichmentJob
		if current == nil || (!current.correctionRequested && (!job.explicitRetry || !current.unavailablePublished)) {
			return false
		}
	}
	if next := wt.gitStatusEnrichmentNext; next != nil {
		if next.indexCleanup != nil {
			completeGitStatusEnrichmentLocked(next, errGitStatusEvidenceChanged)
			next.indexCleanup()
		}
	}
	wt.gitStatusEnrichmentNext = job
	return true
}

func (wt *WorkspaceTracker) runGitStatusEnrichmentQueue(job *gitStatusEnrichmentJob) {
	defer wt.gitStatusObserveWG.Done()
	current := job
	for current != nil {
		err := wt.runGitStatusEnrichment(current)
		if wt.cancelCtx == nil || wt.cancelCtx.Err() == nil {
			wt.gitStatusEnrichmentMu.Lock()
			published := false
			if err != nil {
				published = wt.publishGitStatusDetailsUnavailable(current)
			} else {
				wt.mu.RLock()
				published = wt.gitStatusFingerprint == current.fingerprint && wt.currentStatus.DetailState == gitStatusDetailUnavailable
				wt.mu.RUnlock()
			}
			var gate func()
			if published {
				current.unavailablePublished = true
				gate = wt.gitStatusAfterUnavailablePublication
			}
			wt.gitStatusEnrichmentMu.Unlock()
			if gate != nil {
				gate()
			}
		}
		wt.gitStatusEnrichmentMu.Lock()
		if current.indexCleanup != nil {
			current.indexCleanup()
		}
		completeGitStatusEnrichmentLocked(current, err)
		current = wt.gitStatusEnrichmentNext
		wt.gitStatusEnrichmentNext = nil
		if current == nil || (wt.cancelCtx != nil && wt.cancelCtx.Err() != nil) {
			if current != nil {
				if current.indexCleanup != nil {
					current.indexCleanup()
				}
				completeGitStatusEnrichmentLocked(current, wt.cancelCtx.Err())
			}
			wt.gitStatusEnrichmentRun = false
			wt.gitStatusEnrichmentCurrent = ""
			wt.gitStatusEnrichmentJob = nil
			wt.gitStatusEnrichmentMu.Unlock()
			return
		}
		wt.gitStatusEnrichmentCurrent = current.fingerprint
		wt.gitStatusEnrichmentJob = current
		wt.gitStatusEnrichmentMu.Unlock()
	}
}

func (wt *WorkspaceTracker) runGitStatusEnrichment(job *gitStatusEnrichmentJob) error {
	timeout := wt.gitStatusEnrichmentTimeout
	if timeout <= 0 {
		timeout = workspaceGitStatusEnrichmentTimeout
	}
	ctx, cancel := context.WithTimeout(wt.cancelCtxOrBackground(), timeout)
	defer cancel()
	ctx = withGitWorkClass(ctx, subproc.GitBackground)
	if err := wt.validateGitStatusEnrichmentBeforeRun(ctx, job); err != nil {
		return err
	}
	status, err := wt.computeEnrichedGitStatus(ctx, job)
	if err != nil {
		return err
	}
	return wt.publishValidatedGitStatusEnrichment(ctx, job, status)
}

func (wt *WorkspaceTracker) validateGitStatusEnrichmentBeforeRun(ctx context.Context, job *gitStatusEnrichmentJob) error {
	if wt.gitStatusBeforeEnrich != nil {
		wt.gitStatusBeforeEnrich()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := wt.validateGitStatusEnrichment(ctx, job); err != nil {
		wt.logger.Debug("discarding changed git status enrichment", zap.Error(err))
		wt.requestGitStatusCorrection(job)
		return err
	}
	return nil
}

func (wt *WorkspaceTracker) computeEnrichedGitStatus(ctx context.Context, job *gitStatusEnrichmentJob) (types.GitStatusUpdate, error) {
	enrichmentCtx := withGitIndexFile(ctx, job.indexSnapshot)
	status := cloneGitStatusUpdate(job.status)
	prior := types.GitStatusUpdate{}
	wt.mu.RLock()
	if wt.gitStatusFingerprint == job.fingerprint {
		prior = cloneGitStatusUpdate(wt.currentStatus)
	}
	wt.mu.RUnlock()

	wt.getAheadBehindCounts(enrichmentCtx, &status, prior)
	if err := ctx.Err(); err != nil {
		return types.GitStatusUpdate{}, err
	}
	wt.getRemoteAheadBehindCounts(enrichmentCtx, &status, prior)
	if err := ctx.Err(); err != nil {
		return types.GitStatusUpdate{}, err
	}
	status.BaseCommit = wt.ResolveBaseCommit(enrichmentCtx)
	if job.comparison.Explicit && job.comparison.Status == comparisonTargetStatusReady && status.BaseCommit == "" {
		status.ComparisonStatus = comparisonTargetStatusUnavailable
		status.ComparisonErrorCode = comparisonTargetErrorMergeBase
	}
	wt.enrichSymlinkMetadata(enrichmentCtx, &status)
	if err := wt.enrichWithDiffDataAgainst(enrichmentCtx, &status, prior, job.headCommit); err != nil {
		return types.GitStatusUpdate{}, err
	}
	if err := wt.enrichWithBranchDiff(enrichmentCtx, &status, prior); err != nil {
		return types.GitStatusUpdate{}, err
	}
	if err := ctx.Err(); err != nil {
		return types.GitStatusUpdate{}, err
	}
	return status, nil
}

func (wt *WorkspaceTracker) publishValidatedGitStatusEnrichment(ctx context.Context, job *gitStatusEnrichmentJob, status types.GitStatusUpdate) error {
	status.StatusState = gitStatusStateReady
	status.FilesComplete = true
	if status.DetailState == gitStatusDetailUnavailable {
		setPendingGitStatusFileDiffState(&status, gitStatusDiffUnavailable)
	} else {
		status.DetailState = gitStatusDetailReady
		setPendingGitStatusFileDiffState(&status, gitStatusDiffReady)
	}
	if wt.gitStatusBeforeFinalValidation != nil {
		wt.gitStatusBeforeFinalValidation()
	}
	if err := wt.validateGitStatusEnrichment(ctx, job); err != nil {
		wt.logger.Debug("discarding changed git status enrichment", zap.Error(err))
		wt.requestGitStatusCorrection(job)
		return err
	}
	if !wt.publishEnrichedGitStatus(job, status) {
		return errGitStatusEvidenceChanged
	}
	return nil
}

func setPendingGitStatusFileDiffState(status *types.GitStatusUpdate, state string) {
	for path, file := range status.Files {
		if file.DiffState == "" || file.DiffState == gitStatusDetailPending {
			file.DiffState = state
		}
		if file.StagedChange != nil && (file.StagedChange.DiffState == "" || file.StagedChange.DiffState == gitStatusDetailPending) {
			facet := *file.StagedChange
			facet.DiffState = state
			file.StagedChange = &facet
		}
		if file.UnstagedChange != nil && (file.UnstagedChange.DiffState == "" || file.UnstagedChange.DiffState == gitStatusDetailPending) {
			facet := *file.UnstagedChange
			facet.DiffState = state
			file.UnstagedChange = &facet
		}
		status.Files[path] = file
	}
}

func (wt *WorkspaceTracker) publishGitStatusDetailsUnavailable(job *gitStatusEnrichmentJob) bool {
	status := cloneGitStatusUpdate(job.status)
	status.StatusState = gitStatusStateReady
	status.FilesComplete = true
	status.DetailState = gitStatusDetailUnavailable
	status.ErrorCode = gitStatusErrorDetailsUnavailable
	setGitStatusFileDiffState(&status, gitStatusDiffUnavailable)
	return wt.publishEnrichedGitStatus(job, status)
}

func (wt *WorkspaceTracker) completeGitStatusEnrichment(job *gitStatusEnrichmentJob, err error) {
	wt.gitStatusEnrichmentMu.Lock()
	defer wt.gitStatusEnrichmentMu.Unlock()
	completeGitStatusEnrichmentLocked(job, err)
}

func completeGitStatusEnrichmentLocked(job *gitStatusEnrichmentJob, err error) {
	if job == nil || job.completed {
		return
	}
	job.err = err
	job.completed = true
	close(job.done)
}

func (wt *WorkspaceTracker) cancelCtxOrBackground() context.Context {
	if wt.cancelCtx != nil {
		return wt.cancelCtx
	}
	return context.Background()
}

func (wt *WorkspaceTracker) publishEnrichedGitStatus(job *gitStatusEnrichmentJob, status types.GitStatusUpdate) bool {
	wt.gitStatusPublishMu.Lock()
	defer wt.gitStatusPublishMu.Unlock()
	wt.mu.Lock()
	if wt.gitStatusFingerprint != job.fingerprint || wt.currentStatus.StatusState != gitStatusStateReady || !wt.currentStatus.FilesComplete {
		wt.mu.Unlock()
		return false
	}
	wt.gitStatusRevision++
	status.TrackerID = wt.gitStatusTrackerID
	status.TrackerEpoch = wt.gitStatusEpoch
	status.SnapshotRevision = wt.gitStatusRevision
	status.Timestamp = wt.currentStatus.Timestamp
	wt.currentStatus = cloneGitStatusUpdate(status)
	wt.mu.Unlock()
	wt.notifyWorkspaceStreamGitStatus(cloneGitStatusUpdate(status))
	return true
}

func (wt *WorkspaceTracker) validateGitStatusEnrichment(ctx context.Context, job *gitStatusEnrichmentJob) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if job == nil || !job.contentEvidenceComplete {
		return errGitStatusEvidenceChanged
	}
	if err := wt.validateGitStatusEnrichmentScope(ctx, job); err != nil {
		return err
	}
	if err := wt.validateGitStatusEnrichmentRepository(ctx, job); err != nil {
		return err
	}
	if err := wt.validateGitStatusEnrichmentIndex(ctx, job); err != nil {
		return err
	}
	if err := wt.validateGitStatusEnrichmentRefs(ctx, job); err != nil {
		return err
	}
	return wt.validateGitStatusEnrichmentFiles(ctx, job)
}

func (wt *WorkspaceTracker) validateGitStatusEnrichmentScope(ctx context.Context, job *gitStatusEnrichmentJob) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if wt.gitEnvironmentVersion() != job.environmentGeneration {
		return errGitStatusEvidenceChanged
	}
	comparison, generation := wt.comparisonSnapshot()
	if generation != job.comparisonGeneration || comparison != job.comparison {
		return errGitStatusEvidenceChanged
	}
	workDir, err := filepath.EvalSymlinks(wt.workDir)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return errGitStatusEvidenceChanged
	}
	workDir, err = filepath.Abs(workDir)
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if err != nil || filepath.Clean(workDir) != filepath.Clean(job.canonicalWorkDir) {
		return errGitStatusEvidenceChanged
	}
	return nil
}

func (wt *WorkspaceTracker) validateGitStatusEnrichmentRepository(ctx context.Context, job *gitStatusEnrichmentJob) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	gitDir, err := wt.gitDirectory(ctx)
	if err != nil || gitDir != job.gitDir {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return errGitStatusEvidenceChanged
	}
	current := wt.newGitStatusUpdate()
	if err := wt.getGitBranchIdentity(ctx, &current); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if current.HeadCommit != job.headCommit || current.Branch != job.branch || current.RemoteBranch != job.remoteBranch {
		return errGitStatusEvidenceChanged
	}
	return nil
}

func (wt *WorkspaceTracker) validateGitStatusEnrichmentIndex(ctx context.Context, job *gitStatusEnrichmentJob) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	indexInfo, err := os.Stat(wt.gitIndexPath)
	if err != nil || !os.SameFile(job.indexSourceInfo, indexInfo) {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return errGitStatusEvidenceChanged
	}
	indexDigest, err := digestFile(ctx, wt.gitIndexPath)
	if err != nil || hex.EncodeToString(indexDigest[:]) != job.indexDigest {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return errGitStatusEvidenceChanged
	}
	return nil
}

func (wt *WorkspaceTracker) validateGitStatusEnrichmentRefs(ctx context.Context, job *gitStatusEnrichmentJob) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	baseRef := wt.resolveBaseBranch(ctx)
	if err := ctx.Err(); err != nil {
		return err
	}
	if baseRef != job.baseRef {
		return errGitStatusEvidenceChanged
	}
	aheadRef := wt.resolveAheadBehindRef(ctx)
	if err := ctx.Err(); err != nil {
		return err
	}
	if aheadRef != job.aheadRef {
		return errGitStatusEvidenceChanged
	}
	return wt.validateGitStatusEnrichmentRefOIDs(ctx, job, baseRef, aheadRef)
}

func (wt *WorkspaceTracker) validateGitStatusEnrichmentRefOIDs(ctx context.Context, job *gitStatusEnrichmentJob, baseRef, aheadRef string) error {
	refs := []struct {
		ref      string
		expected string
	}{
		{ref: baseRef, expected: job.baseOID},
		{ref: aheadRef, expected: job.aheadOID},
		{ref: job.remoteBranch, expected: job.remoteHeadOID},
		{ref: job.comparison.Ref, expected: job.comparisonOID},
	}
	for _, item := range refs {
		if err := wt.validateGitStatusRefOID(ctx, item.ref, item.expected); err != nil {
			return err
		}
	}
	return nil
}

func (wt *WorkspaceTracker) validateGitStatusRefOID(ctx context.Context, ref, expected string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	actual, err := wt.gitRefOID(ctx, ref)
	if err := ctx.Err(); err != nil {
		return err
	}
	if err != nil || actual != expected {
		return errGitStatusEvidenceChanged
	}
	return nil
}

func (wt *WorkspaceTracker) validateGitStatusEnrichmentFiles(ctx context.Context, job *gitStatusEnrichmentJob) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	fileEvidence, contentComplete, err := captureGitStatusFileEvidence(ctx, wt, wt.workDir, job.status)
	if err != nil || !contentComplete || !sameGitStatusFileEvidence(job.fileEvidence, fileEvidence) {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return errGitStatusEvidenceChanged
	}
	return nil
}

func (wt *WorkspaceTracker) requestGitStatusCorrection(job *gitStatusEnrichmentJob) {
	if job == nil || !job.correctionPermitted {
		return
	}
	wt.gitStatusObserveMu.Lock()
	ctx := wt.cancelCtx
	if ctx != nil && ctx.Err() != nil {
		wt.gitStatusObserveMu.Unlock()
		return
	}
	wt.gitStatusObserveWG.Add(1)
	wt.gitStatusObserveMu.Unlock()
	wt.gitStatusEnrichmentMu.Lock()
	if wt.gitStatusEnrichmentJob == job {
		job.correctionRequested = true
	}
	wt.gitStatusEnrichmentMu.Unlock()
	go func() {
		defer wt.gitStatusObserveWG.Done()
		if wt.gitStatusBeforeCorrection != nil {
			wt.gitStatusBeforeCorrection(ctx)
		}
		_, err := wt.observeGitStatusClass(ctx, subproc.GitInteractive, "basic", nil, false)
		if err != nil && !wt.isGitStatusCancellation(err) {
			wt.logger.Debug("corrective git status observation failed", zap.Error(err))
		}
	}()
}
