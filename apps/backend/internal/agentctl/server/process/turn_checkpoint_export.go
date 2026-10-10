package process

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/kandev/kandev/internal/common/turnchanges"
)

const (
	turnCheckpointPatchLimit     = 4 << 20
	turnCheckpointRenderingLimit = 8 << 20
	turnCheckpointExportLimit    = 32 << 20
)

func (g *GitOperator) ExportTurnCheckpoint(ctx context.Context, request turnchanges.ExportRequest) (turnchanges.CheckpointExport, error) {
	if !validTurnChangeIdentity(request.ChangeSetID) || !validTurnChangeIdentity(request.CheckoutID) {
		return turnchanges.CheckpointExport{}, turnCheckpointFailure(turnchanges.ReasonUnsafeGitState, errors.New("invalid change-set or checkout identity"))
	}
	operationCtx, cancel := context.WithTimeout(ctx, turnCheckpointEndTimeout)
	defer cancel()
	gitDir, hashAlgorithm, err := g.turnCheckpointRepository(operationCtx)
	if err != nil {
		return turnchanges.CheckpointExport{}, err
	}
	unlock, err := acquireTurnCheckpointLock(operationCtx, gitDir)
	if err != nil {
		return turnchanges.CheckpointExport{}, turnCheckpointFailure(turnchanges.ReasonComparisonFailed, err)
	}
	defer unlock()
	return g.exportTurnCheckpointLocked(operationCtx, request, hashAlgorithm)
}

func (g *GitOperator) exportTurnCheckpointLocked(ctx context.Context, request turnchanges.ExportRequest, hashAlgorithm string) (turnchanges.CheckpointExport, error) {
	compareRequest := turnchanges.CompareRequest(request)
	start, end, err := g.turnCheckpointPair(ctx, compareRequest, hashAlgorithm)
	if err != nil {
		return turnchanges.CheckpointExport{}, err
	}
	view, cleanup, err := g.newTurnCheckpointDiffView(ctx, end.TreeOID, hashAlgorithm)
	if err != nil {
		return turnchanges.CheckpointExport{}, checkpointCompareFailure(err)
	}
	defer cleanup()
	files, err := view.compareTurnCheckpointTrees(ctx, start.TreeOID)
	if err != nil {
		return turnchanges.CheckpointExport{}, err
	}
	export := turnchanges.CheckpointExport{
		ChangeSetID: request.ChangeSetID, CheckoutID: request.CheckoutID,
		HashAlgorithm: request.HashAlgorithm, StartCommitOID: request.StartCommitOID,
		StartTreeOID: request.StartTreeOID, EndCommitOID: request.EndCommitOID, EndTreeOID: request.EndTreeOID,
		Files: make([]turnchanges.CheckpointExportFile, 0, len(files)), Complete: true,
	}
	for _, file := range files {
		item, itemBytes, complete := g.exportTurnCheckpointFile(ctx, view, start.TreeOID, file, turnCheckpointExportLimit-export.ExportBytes)
		if !complete {
			markTurnCheckpointExportIncomplete(&export, item.Reason)
		}
		export.ExportBytes += itemBytes
		export.Files = append(export.Files, item)
	}
	return export, nil
}

func (g *GitOperator) exportTurnCheckpointFile(
	ctx context.Context,
	view *turnCheckpointDiffView,
	startTreeOID string,
	file turnchanges.CheckpointFile,
	remaining int64,
) (turnchanges.CheckpointExportFile, int64, bool) {
	item := turnchanges.CheckpointExportFile{File: file, ContentAvailability: turnchanges.Ready}
	if !utf8.Valid(file.PathBytes) || len(file.OldPathBytes) > 0 && !utf8.Valid(file.OldPathBytes) {
		markTurnCheckpointExportPartial(&item, turnchanges.ReasonInvalidPathEncoding)
		return item, 0, false
	}
	patch, err := view.turnCheckpointFilePatch(ctx, startTreeOID, file, false)
	if err != nil || len(patch) > turnCheckpointPatchLimit {
		return incompleteTurnCheckpointPatch(item, err, len(patch) > turnCheckpointPatchLimit)
	}
	item.CanonicalPatch, item.CanonicalBytes = patch, int64(len(patch))
	filtered, err := view.turnCheckpointFilePatch(ctx, startTreeOID, file, true)
	if err != nil || len(filtered) > turnCheckpointPatchLimit {
		reason := exportFailureReason(err)
		if err == nil {
			reason = turnchanges.ReasonSizeLimit
		}
		markTurnCheckpointExportPartial(&item, reason)
	} else {
		item.FilteredPatch = filtered
	}
	if !file.Binary && !file.Submodule {
		g.exportTurnCheckpointRenderings(ctx, &item)
	}
	itemBytes, truncated := fitTurnCheckpointExportFile(&item, remaining)
	return item, itemBytes, !truncated && item.ContentAvailability == turnchanges.Ready
}

func incompleteTurnCheckpointPatch(
	item turnchanges.CheckpointExportFile,
	err error,
	oversized bool,
) (turnchanges.CheckpointExportFile, int64, bool) {
	reason := exportFailureReason(err)
	if oversized {
		reason = turnchanges.ReasonSizeLimit
	}
	markTurnCheckpointExportPartial(&item, reason)
	return item, 0, false
}

func (g *GitOperator) exportTurnCheckpointRenderings(ctx context.Context, item *turnchanges.CheckpointExportFile) {
	oldRendering, err := g.turnCheckpointBlob(ctx, item.File.OldOID)
	if err == nil && len(oldRendering) > turnCheckpointRenderingLimit {
		err = ErrTurnCheckpointOutputLimit
	}
	if err != nil {
		markTurnCheckpointExportPartial(item, exportFailureReason(err))
		return
	}
	newRendering, err := g.turnCheckpointBlob(ctx, item.File.NewOID)
	if err == nil && len(newRendering) > turnCheckpointRenderingLimit {
		err = ErrTurnCheckpointOutputLimit
	}
	if err != nil {
		markTurnCheckpointExportPartial(item, exportFailureReason(err))
		return
	}
	item.OldRendering, item.NewRendering = oldRendering, newRendering
}

func markTurnCheckpointExportPartial(item *turnchanges.CheckpointExportFile, reason turnchanges.ReasonCode) {
	item.ContentAvailability = turnchanges.Unavailable
	item.Truncated = true
	if item.Reason == "" || item.Reason == turnchanges.ReasonContentUnavailable || reason == turnchanges.ReasonSizeLimit {
		item.Reason = reason
	}
}

func markTurnCheckpointExportIncomplete(export *turnchanges.CheckpointExport, reason turnchanges.ReasonCode) {
	export.Complete = false
	if export.Reason == "" || export.Reason == turnchanges.ReasonContentUnavailable || reason == turnchanges.ReasonSizeLimit {
		export.Reason = reason
	}
}

func fitTurnCheckpointExportFile(item *turnchanges.CheckpointExportFile, remaining int64) (int64, bool) {
	used := int64(0)
	truncated := false
	variants := []*[]byte{&item.CanonicalPatch, &item.FilteredPatch, &item.OldRendering, &item.NewRendering}
	for index, variant := range variants {
		if *variant == nil {
			continue
		}
		limit := int64(turnCheckpointPatchLimit)
		if index >= 2 {
			limit = turnCheckpointRenderingLimit
		}
		length := int64(len(*variant))
		if length > limit || length > remaining-used {
			*variant = nil
			truncated = true
			continue
		}
		used += length
	}
	if truncated {
		markTurnCheckpointExportPartial(item, turnchanges.ReasonSizeLimit)
	}
	return used, truncated
}

func (v *turnCheckpointDiffView) turnCheckpointFilePatch(ctx context.Context, startOID string, file turnchanges.CheckpointFile, ignoreWhitespace bool) ([]byte, error) {
	args := []string{"diff", "--binary", "--no-ext-diff", "--no-textconv", "--no-color", "--src-prefix=a/", "--dst-prefix=b/", "-M", "-C", "--find-copies-harder"}
	if ignoreWhitespace {
		args = append(args, "--ignore-all-space")
	}
	args = append(args, "--cached", startOID, "--")
	if len(file.OldPathBytes) > 0 {
		args = append(args, literalGitPathspec(string(file.OldPathBytes)))
	}
	args = append(args, literalGitPathspec(string(file.PathBytes)))
	output, err := v.output(ctx, args...)
	if err != nil {
		return nil, err
	}
	if len(output) == 0 {
		return []byte{}, nil
	}
	_, sections := splitDiffSections(string(output))
	for _, section := range sections {
		if path, ok := turnCheckpointPatchPath(section); ok && path == string(file.PathBytes) {
			return []byte(section), nil
		}
	}
	return nil, turnCheckpointFailure(turnchanges.ReasonContentUnavailable, errors.New("historical patch does not contain the selected file"))
}

func turnCheckpointPatchPath(section string) (string, bool) {
	for _, line := range strings.Split(section, "\n") {
		if path, ok := diffWholeLinePath("copy to ", line); ok {
			return path, true
		}
	}
	return diffSectionPath(section)
}

func (g *GitOperator) turnCheckpointBlob(ctx context.Context, oid string) ([]byte, error) {
	if oid == "" {
		return nil, nil
	}
	if isZeroCheckpointOID(oid) {
		return nil, nil
	}
	if !validCheckpointOID(oid, turnCheckpointSHA1) && !validCheckpointOID(oid, turnCheckpointSHA256) {
		return nil, turnCheckpointFailure(turnchanges.ReasonUnsafeGitState, fmt.Errorf("invalid rendering object ID"))
	}
	output, err := g.turnCheckpointOutput(ctx, "", "cat-file", "blob", oid)
	if err != nil {
		return nil, err
	}
	return append(make([]byte, 0, len(output)), output...), nil
}

func isZeroCheckpointOID(oid string) bool {
	if oid == "" {
		return false
	}
	for _, digit := range oid {
		if digit != '0' {
			return false
		}
	}
	return true
}

func exportFailureReason(err error) turnchanges.ReasonCode {
	if errors.Is(err, ErrTurnCheckpointOutputLimit) {
		return turnchanges.ReasonSizeLimit
	}
	var checkpointErr *TurnCheckpointError
	if errors.As(err, &checkpointErr) {
		return checkpointErr.Reason
	}
	return turnchanges.ReasonContentUnavailable
}
