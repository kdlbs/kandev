package process

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/common/subproc"
	"github.com/kandev/kandev/internal/common/turnchanges"
)

const (
	turnCheckpointArchiveTimeout     = 90 * time.Second
	turnCheckpointArchivePackLimit   = 32 << 20
	turnCheckpointArchiveObjectLimit = 200_000
	turnCheckpointArchiveBytesLimit  = 256 << 20
	turnCheckpointArchiveStderrLimit = 64 << 10
)

// ArchiveTurnCheckpoint exports a self-contained pack for an already accepted
// endpoint pair. It never captures a new snapshot and rejects endpoints that do
// not match the objects reachable from the immutable checkpoint commits.
func (g *GitOperator) ArchiveTurnCheckpoint(
	ctx context.Context,
	request turnchanges.ArchiveRequest,
) (turnchanges.CheckpointArchive, error) {
	if !validTurnChangeIdentity(request.ChangeSetID) || !validTurnChangeIdentity(request.CheckoutID) {
		return turnchanges.CheckpointArchive{}, turnCheckpointFailure(turnchanges.ReasonUnsafeGitState, errors.New("invalid change-set or checkout identity"))
	}
	if !hasTurnCheckpointEndpointPair(request.HashAlgorithm, request.StartCommitOID, request.StartTreeOID, request.EndCommitOID, request.EndTreeOID) {
		return turnchanges.CheckpointArchive{}, turnCheckpointFailure(turnchanges.ReasonUnsafeGitState, errors.New("archive requires accepted endpoint identities"))
	}
	operationCtx, cancel := context.WithTimeout(ctx, turnCheckpointArchiveTimeout)
	defer cancel()
	gitDir, hashAlgorithm, err := g.turnCheckpointRepository(operationCtx)
	if err != nil {
		return turnchanges.CheckpointArchive{}, err
	}
	if hashAlgorithm != request.HashAlgorithm {
		return turnchanges.CheckpointArchive{}, turnCheckpointFailure(turnchanges.ReasonUnsafeGitState, errors.New("archive hash algorithm does not match the repository"))
	}
	unlock, err := acquireTurnCheckpointLock(operationCtx, gitDir)
	if err != nil {
		return turnchanges.CheckpointArchive{}, turnCheckpointFailure(turnchanges.ReasonCaptureFailed, err)
	}
	defer unlock()
	return g.archiveTurnCheckpointLocked(operationCtx, request)
}

func (g *GitOperator) archiveTurnCheckpointLocked(
	ctx context.Context,
	request turnchanges.ArchiveRequest,
) (turnchanges.CheckpointArchive, error) {
	archive := turnchanges.CheckpointArchive{
		ChangeSetID: request.ChangeSetID, CheckoutID: request.CheckoutID, HashAlgorithm: request.HashAlgorithm,
		StartCommitOID: request.StartCommitOID, StartTreeOID: request.StartTreeOID,
		EndCommitOID: request.EndCommitOID, EndTreeOID: request.EndTreeOID,
	}
	if err := g.verifyTurnCheckpointEndpoint(ctx, request.StartCommitOID, request.StartTreeOID); err != nil {
		return archive, err
	}
	if err := g.verifyTurnCheckpointEndpoint(ctx, request.EndCommitOID, request.EndTreeOID); err != nil {
		return archive, err
	}
	objects, totalBytes, err := g.turnCheckpointArchiveObjects(ctx, request.StartTreeOID, request.EndTreeOID, request.StartCommitOID, request.EndCommitOID)
	if err != nil {
		return archive, err
	}
	if len(objects) > turnCheckpointArchiveObjectLimit || totalBytes > turnCheckpointArchiveBytesLimit {
		return archive, turnCheckpointFailure(turnchanges.ReasonSizeLimit, fmt.Errorf("turn checkpoint archive exceeds object bounds"))
	}
	known := make(map[string]struct{}, len(request.KnownObjects))
	for _, oid := range request.KnownObjects {
		if validCheckpointOID(oid, request.HashAlgorithm) {
			known[oid] = struct{}{}
		}
	}
	missing := make([]string, 0, len(objects))
	for _, oid := range objects {
		if _, exists := known[oid]; !exists {
			missing = append(missing, oid)
		}
	}
	pack, err := g.turnCheckpointArchivePack(ctx, missing)
	if err != nil {
		return archive, err
	}
	if len(pack) > turnCheckpointArchivePackLimit {
		return archive, turnCheckpointFailure(turnchanges.ReasonSizeLimit, fmt.Errorf("turn checkpoint archive pack exceeds %d bytes", turnCheckpointArchivePackLimit))
	}
	archive.Pack = pack
	archive.Objects = objects
	archive.ObjectCount = int64(len(objects))
	archive.UncompressedBytes = totalBytes
	archive.Complete = true
	return archive, nil
}

func (g *GitOperator) verifyTurnCheckpointEndpoint(ctx context.Context, commitOID, treeOID string) error {
	treeOutput, err := g.turnCheckpointOutput(ctx, "", "rev-parse", "--verify", commitOID+"^{tree}")
	if err != nil {
		return turnCheckpointFailure(turnchanges.ReasonContentUnavailable, fmt.Errorf("read accepted checkpoint commit: %w", err))
	}
	if strings.TrimSpace(string(treeOutput)) != treeOID {
		return turnCheckpointFailure(turnchanges.ReasonUnsafeGitState, errors.New("accepted checkpoint tree does not belong to its commit"))
	}
	return nil
}

// turnCheckpointArchiveObjects enumerates the complete tree/blob closure for
// both interval trees plus the two accepted commits. Submodule gitlinks are
// excluded: their commit objects live in the nested repository.
func (g *GitOperator) turnCheckpointArchiveObjects(
	ctx context.Context,
	startTreeOID, endTreeOID, startCommitOID, endCommitOID string,
) ([]string, int64, error) {
	seen := make(map[string]struct{})
	objects := make([]string, 0, 1024)
	for _, root := range []string{startTreeOID, endTreeOID} {
		output, err := g.turnCheckpointOutput(ctx, "", "ls-tree", "-r", "-t", "-z", "--full-tree", root)
		if err != nil {
			if errors.Is(err, ErrTurnCheckpointOutputLimit) {
				return nil, 0, turnCheckpointFailure(turnchanges.ReasonSizeLimit, err)
			}
			return nil, 0, turnCheckpointFailure(turnchanges.ReasonContentUnavailable, err)
		}
		for _, record := range strings.Split(string(output), "\x00") {
			oid, ok := turnCheckpointTreeRecordOID(record)
			if !ok {
				continue
			}
			if _, exists := seen[oid]; exists {
				continue
			}
			seen[oid] = struct{}{}
			objects = append(objects, oid)
		}
		if _, exists := seen[root]; !exists {
			seen[root] = struct{}{}
			objects = append(objects, root)
		}
	}
	for _, commit := range []string{startCommitOID, endCommitOID} {
		if _, exists := seen[commit]; exists {
			continue
		}
		seen[commit] = struct{}{}
		objects = append(objects, commit)
	}
	sort.Strings(objects)
	totalBytes, err := g.turnCheckpointObjectBytes(ctx, objects)
	if err != nil {
		return nil, 0, err
	}
	return objects, totalBytes, nil
}

func turnCheckpointTreeRecordOID(record string) (string, bool) {
	metadata, _, separated := strings.Cut(record, "\t")
	if !separated {
		return "", false
	}
	fields := strings.Fields(metadata)
	if len(fields) != 3 || fields[1] == "commit" {
		return "", false
	}
	if !validCheckpointOID(fields[2], turnCheckpointSHA1) && !validCheckpointOID(fields[2], turnCheckpointSHA256) {
		return "", false
	}
	return fields[2], true
}

func (g *GitOperator) turnCheckpointObjectBytes(ctx context.Context, objects []string) (int64, error) {
	input := []byte(strings.Join(objects, "\n") + "\n")
	output, err := g.turnCheckpointOutputWithInput(ctx, "", nil, input, "cat-file", "--batch-check=%(objectname) %(objecttype) %(objectsize)")
	if err != nil {
		return 0, turnCheckpointFailure(turnchanges.ReasonContentUnavailable, err)
	}
	var total int64
	lines := strings.Split(strings.TrimSuffix(string(output), "\n"), "\n")
	if len(lines) != len(objects) {
		return 0, turnCheckpointFailure(turnchanges.ReasonContentUnavailable, errors.New("checkpoint object inventory changed during archive"))
	}
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) != 3 {
			return 0, turnCheckpointFailure(turnchanges.ReasonContentUnavailable, errors.New("checkpoint object inventory is missing an object"))
		}
		var size int64
		if _, err := fmt.Sscan(fields[2], &size); err != nil || size < 0 {
			return 0, turnCheckpointFailure(turnchanges.ReasonContentUnavailable, errors.New("checkpoint object size is invalid"))
		}
		total += size
	}
	return total, nil
}

func (g *GitOperator) turnCheckpointArchivePack(ctx context.Context, objects []string) ([]byte, error) {
	if len(objects) == 0 {
		return nil, nil
	}
	packFile, err := os.CreateTemp("", "kandev-turn-archive-*.pack")
	if err != nil {
		return nil, turnCheckpointFailure(turnchanges.ReasonCaptureFailed, err)
	}
	packPath := packFile.Name()
	defer func() {
		_ = packFile.Close()
		_ = os.Remove(packPath)
	}()
	input := []byte(strings.Join(objects, "\n") + "\n")
	stderr := &boundedGitOutput{limit: turnCheckpointArchiveStderrLimit}
	writer := &turnCheckpointArchiveWriter{file: packFile, remaining: turnCheckpointArchivePackLimit}
	runErr, contextErr := subproc.RunGitAfterAcquire(ctx, subproc.GitLifecycle, turnCheckpointArchiveTimeout, func(execCtx context.Context) *exec.Cmd {
		cmd := subproc.NewGitCommand(execCtx, "pack-objects", "--stdout", "-q")
		cmd.Dir = g.workDir
		cmd.Env = turnCheckpointEnvironment(g.environmentValues(), "", nil)
		cmd.Stdin = strings.NewReader(string(input))
		cmd.Stdout = writer
		cmd.Stderr = stderr
		return cmd
	})
	if contextErr != nil {
		return nil, turnCheckpointFailure(turnchanges.ReasonCaptureFailed, contextErr)
	}
	if writer.truncated {
		return nil, turnCheckpointFailure(turnchanges.ReasonSizeLimit, errors.New("turn checkpoint archive pack exceeds its bound"))
	}
	if runErr != nil {
		return nil, turnCheckpointFailure(turnchanges.ReasonCaptureFailed, fmt.Errorf("git pack-objects: %s", strings.TrimSpace(stderr.String())))
	}
	if err := packFile.Close(); err != nil {
		return nil, turnCheckpointFailure(turnchanges.ReasonCaptureFailed, err)
	}
	info, err := os.Stat(packPath)
	if err != nil {
		return nil, turnCheckpointFailure(turnchanges.ReasonCaptureFailed, err)
	}
	if info.Size() > turnCheckpointArchivePackLimit {
		return nil, turnCheckpointFailure(turnchanges.ReasonSizeLimit, errors.New("turn checkpoint archive pack exceeds its bound"))
	}
	pack, err := os.ReadFile(packPath)
	if err != nil {
		return nil, turnCheckpointFailure(turnchanges.ReasonCaptureFailed, err)
	}
	return pack, nil
}

func hasTurnCheckpointEndpointPair(hashAlgorithm, startCommit, startTree, endCommit, endTree string) bool {
	return (hashAlgorithm == turnCheckpointSHA1 || hashAlgorithm == turnCheckpointSHA256) &&
		startCommit != "" && startTree != "" && endCommit != "" && endTree != ""
}

type turnCheckpointArchiveWriter struct {
	file      *os.File
	remaining int64
	truncated bool
}

func (w *turnCheckpointArchiveWriter) Write(data []byte) (int, error) {
	if w.remaining <= 0 {
		w.truncated = true
		return len(data), errors.New("turn checkpoint archive pack limit exceeded")
	}
	if int64(len(data)) > w.remaining {
		written, err := w.file.Write(data[:w.remaining])
		w.remaining = 0
		w.truncated = true
		if err != nil {
			return written, err
		}
		return len(data), errors.New("turn checkpoint archive pack limit exceeded")
	}
	written, err := w.file.Write(data)
	w.remaining -= int64(written)
	return written, err
}
