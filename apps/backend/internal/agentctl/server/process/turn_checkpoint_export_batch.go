package process

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/common/turnchanges"
)

const (
	turnCheckpointExportPageTimeout  = 10 * time.Second
	turnCheckpointExportBatchEntries = turnchanges.MaxExportBatchEntries
)

type turnCheckpointExportPair struct {
	startCommitOID string
	startTreeOID   string
	endCommitOID   string
	endTreeOID     string
	comparisonRef  string
}

func (g *GitOperator) exportTurnCheckpointBatch(
	ctx context.Context,
	request turnchanges.ExportRequest,
	hashAlgorithm string,
) (turnchanges.CheckpointExport, error) {
	start, end, err := g.turnCheckpointPair(ctx, compareRequestForTurnCheckpointExport(request), hashAlgorithm)
	if err != nil {
		return turnchanges.CheckpointExport{}, err
	}
	pair, err := turnCheckpointExportPairFor(request, start, end, hashAlgorithm)
	if err != nil {
		return turnchanges.CheckpointExport{}, err
	}
	view, cleanup, err := g.newTurnCheckpointDiffView(ctx, pair.endTreeOID, hashAlgorithm)
	if err != nil {
		return turnchanges.CheckpointExport{}, checkpointCompareFailure(err)
	}
	defer cleanup()
	files, err := view.compareTurnCheckpointTrees(ctx, pair.startTreeOID)
	if err != nil {
		return turnchanges.CheckpointExport{}, err
	}
	fingerprint := turnCheckpointInventoryFingerprint(request, pair, files)
	if request.InventoryFingerprint != "" && request.InventoryFingerprint != fingerprint {
		return turnchanges.CheckpointExport{}, turnCheckpointFailure(
			turnchanges.ReasonHistoryContextChanged, errors.New("export inventory no longer matches the accepted comparison"))
	}
	export := turnchanges.CheckpointExport{
		ChangeSetID: request.ChangeSetID, CheckoutID: request.CheckoutID,
		HashAlgorithm: request.HashAlgorithm, StartCommitOID: request.StartCommitOID,
		StartTreeOID: request.StartTreeOID, EndCommitOID: request.EndCommitOID, EndTreeOID: request.EndTreeOID,
		ComparisonVersion: request.ComparisonVersion, ComparisonBasis: turnCheckpointExportBasis(request),
		EffectiveStartCommitOID: pair.startCommitOID, EffectiveStartTreeOID: pair.startTreeOID,
		EffectiveEndCommitOID: pair.endCommitOID, EffectiveEndTreeOID: pair.endTreeOID,
		ComparisonRef: pair.comparisonRef, ExportBatchVersion: turnchanges.ExportBatchVersion1,
		InventoryFingerprint: fingerprint, TotalFiles: int64(len(files)), Files: []turnchanges.CheckpointExportFile{},
	}
	return g.exportTurnCheckpointPage(ctx, view, pair.startTreeOID, files, request, &export), nil
}

func turnCheckpointExportPairFor(
	request turnchanges.ExportRequest,
	start, end turnchanges.CheckpointResult,
	hashAlgorithm string,
) (turnCheckpointExportPair, error) {
	pair := turnCheckpointExportPair{
		startCommitOID: start.CommitOID, startTreeOID: start.TreeOID,
		endCommitOID: end.CommitOID, endTreeOID: end.TreeOID,
	}
	if request.ComparisonVersion != turnchanges.ComparisonVersion2 {
		return pair, nil
	}
	if request.EffectiveStartTreeOID == "" || request.EffectiveEndTreeOID == "" ||
		request.EffectiveEndTreeOID != end.TreeOID || request.EffectiveEndCommitOID != end.CommitOID {
		return pair, turnCheckpointFailure(
			turnchanges.ReasonHistoryContextChanged, errors.New("effective comparison pair does not match the accepted endpoints"))
	}
	if !validCheckpointOID(request.EffectiveStartCommitOID, hashAlgorithm) ||
		!validCheckpointOID(request.EffectiveStartTreeOID, hashAlgorithm) {
		return pair, turnCheckpointFailure(turnchanges.ReasonHistoryContextChanged, errors.New("effective comparison pair is invalid"))
	}
	pair.startCommitOID, pair.startTreeOID = request.EffectiveStartCommitOID, request.EffectiveStartTreeOID
	pair.comparisonRef = request.ComparisonRef
	return pair, nil
}

func turnCheckpointExportBasis(request turnchanges.ExportRequest) turnchanges.ComparisonBasis {
	if request.ComparisonVersion == turnchanges.ComparisonVersion2 && request.ComparisonBasis.Valid() {
		return request.ComparisonBasis
	}
	return turnchanges.ComparisonLegacyRaw
}

func (g *GitOperator) exportTurnCheckpointPage(
	ctx context.Context,
	view *turnCheckpointDiffView,
	startTreeOID string,
	files []turnchanges.CheckpointFile,
	request turnchanges.ExportRequest,
	export *turnchanges.CheckpointExport,
) turnchanges.CheckpointExport {
	cursor := request.Cursor
	if cursor < 0 {
		cursor = 0
	}
	maxEntries := request.MaxEntries
	if maxEntries <= 0 || maxEntries > turnCheckpointExportBatchEntries {
		maxEntries = turnCheckpointExportBatchEntries
	}
	budget := request.BudgetBytes
	if budget <= 0 || budget > turnCheckpointExportLimit {
		budget = turnCheckpointExportLimit
	}
	pageCtx, cancel := context.WithTimeout(ctx, turnCheckpointExportPageTimeout)
	defer cancel()
	for index := cursor; index < int64(len(files)) && len(export.Files) < maxEntries; index++ {
		if pageCtx.Err() != nil || ctx.Err() != nil {
			markTurnCheckpointExportIncomplete(export, turnchanges.ReasonExportTimeout)
			export.NextCursor = index
			return *export
		}
		file := files[index]
		item, itemBytes, _ := g.exportTurnCheckpointFile(pageCtx, view, startTreeOID, file, budget-export.ExportBytes)
		if pageCtx.Err() != nil || ctx.Err() != nil {
			markTurnCheckpointExportIncomplete(export, turnchanges.ReasonExportTimeout)
			export.NextCursor = index
			return *export
		}
		// A file-specific failure stays on its entry so later files still
		// export; only page-wide timeout or total-budget exhaustion is fatal.
		export.ExportBytes += itemBytes
		export.Files = append(export.Files, item)
		cursor = index + 1
		if export.ExportBytes >= budget && cursor < int64(len(files)) {
			markTurnCheckpointExportIncomplete(export, turnchanges.ReasonSizeLimit)
			break
		}
	}
	export.NextCursor = cursor
	export.Complete = cursor >= int64(len(files))
	return *export
}

func turnCheckpointInventoryFingerprint(
	request turnchanges.ExportRequest,
	pair turnCheckpointExportPair,
	files []turnchanges.CheckpointFile,
) string {
	digest := sha256.New()
	writeFingerprintPart(digest, request.ChangeSetID)
	writeFingerprintPart(digest, request.CheckoutID)
	writeFingerprintPart(digest, pair.startTreeOID)
	writeFingerprintPart(digest, pair.endTreeOID)
	var count [8]byte
	binary.BigEndian.PutUint64(count[:], uint64(len(files)))
	_, _ = digest.Write(count[:])
	for _, file := range files {
		writeFingerprintBytes(digest, file.PathBytes)
		writeFingerprintBytes(digest, file.OldPathBytes)
		writeFingerprintPart(digest, file.Kind)
		writeFingerprintPart(digest, file.OldOID)
		writeFingerprintPart(digest, file.NewOID)
	}
	return turnCheckpointFingerprintHex(digest.Sum(nil))
}

type fingerprintWriter interface {
	Write([]byte) (int, error)
}

func writeFingerprintPart(writer fingerprintWriter, value string) {
	writeFingerprintBytes(writer, []byte(value))
}

func writeFingerprintBytes(writer fingerprintWriter, value []byte) {
	var length [8]byte
	binary.BigEndian.PutUint64(length[:], uint64(len(value)))
	_, _ = writer.Write(length[:])
	if len(value) > 0 {
		_, _ = writer.Write(value)
	}
}

func turnCheckpointFingerprintHex(sum []byte) string {
	const digits = "0123456789abcdef"
	var builder strings.Builder
	builder.Grow(len(sum) * 2)
	for _, value := range sum {
		builder.WriteByte(digits[value>>4])
		builder.WriteByte(digits[value&0x0f])
	}
	return builder.String()
}
