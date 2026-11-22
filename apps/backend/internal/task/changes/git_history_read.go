package changes

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/kandev/kandev/internal/common/subproc"
	"github.com/kandev/kandev/internal/common/turnchanges"
	"github.com/kandev/kandev/internal/task/models"
)

// ReadPatch renders one historical file patch from the exact accepted trees.
// It applies endpoint attributes through an isolated index and selects only
// the section whose destination is the server-owned file identity.
func (s *GitHistoryStore) ReadPatch(
	ctx context.Context,
	receipt ArchiveReceipt,
	archive turnchanges.CheckpointArchive,
	file HistoricalFile,
	ignoreWhitespace bool,
) ([]byte, error) {
	if !s.Enabled() {
		return nil, fmt.Errorf("%w: Git history store is not configured", ErrHistoryContentUnavailable)
	}
	return s.readPatchAt(ctx, s.repositoryPath(receipt.SourceID), archive, file, ignoreWhitespace)
}

func (s *GitHistoryStore) readPatchAt(
	ctx context.Context,
	repoPath string,
	archive turnchanges.CheckpointArchive,
	file HistoricalFile,
	ignoreWhitespace bool,
) ([]byte, error) {
	unlock := s.lockSource(filepath.Base(repoPath))
	defer unlock()
	view, cleanup, err := s.newHistoryDiffView(ctx, repoPath, archive.EndTreeOID, archive.HashAlgorithm)
	if err != nil {
		return nil, err
	}
	defer cleanup()
	paths := historyPathspecs(file)
	patchArgs := []string{"diff", "--cached", "--binary", "--no-ext-diff", "--no-textconv", "--no-color",
		"--src-prefix=a/", "--dst-prefix=b/", "-M", "-C", "--find-copies-harder"}
	if ignoreWhitespace {
		patchArgs = append(patchArgs, "--ignore-all-space")
	}
	patchArgs = append(patchArgs, archive.StartTreeOID, "--")
	patchArgs = append(patchArgs, paths...)
	output, err := s.runGitAt(ctx, view.dir, view.env, maxHistoryReadPatchBytes, nil, patchArgs...)
	if err != nil {
		return nil, err
	}
	destinations, err := s.historyDiffDestinations(ctx, view, archive.StartTreeOID, paths)
	if err != nil {
		return nil, err
	}
	return selectHistoryPatchSection(output, destinations, file.PathBytes)
}

func historyPathspecs(file HistoricalFile) []string {
	if len(file.OldPathBytes) > 0 && !bytes.Equal(file.OldPathBytes, file.PathBytes) {
		return []string{":(literal)" + string(file.OldPathBytes), ":(literal)" + string(file.PathBytes)}
	}
	return []string{":(literal)" + string(file.PathBytes)}
}

func (s *GitHistoryStore) historyDiffDestinations(
	ctx context.Context,
	view *historyDiffView,
	startTree string,
	pathspecs []string,
) ([][]byte, error) {
	args := []string{"diff", "--cached", "--raw", "-z", "--no-abbrev", "-M", "-C", "--find-copies-harder", startTree, "--"}
	args = append(args, pathspecs...)
	output, err := s.runGitAt(ctx, view.dir, view.env, 1<<20, nil, args...)
	if err != nil {
		return nil, err
	}
	return parseRawDiffDestinations(output)
}

func parseRawDiffDestinations(output []byte) ([][]byte, error) {
	records := bytes.Split(output, []byte{0})
	destinations := make([][]byte, 0, len(records))
	for index := 0; index < len(records); index++ {
		record := records[index]
		if len(record) == 0 {
			continue
		}
		if record[0] != ':' {
			return nil, fmt.Errorf("%w: unexpected raw diff framing", ErrHistoryContentUnavailable)
		}
		fields := strings.Fields(string(record[1:]))
		if len(fields) < 5 {
			return nil, fmt.Errorf("%w: raw diff record is incomplete", ErrHistoryContentUnavailable)
		}
		status := fields[4]
		if strings.HasPrefix(status, "R") || strings.HasPrefix(status, "C") {
			if index+2 >= len(records) {
				return nil, fmt.Errorf("%w: raw rename record is incomplete", ErrHistoryContentUnavailable)
			}
			destinations = append(destinations, append([]byte(nil), records[index+2]...))
			index += 2
			continue
		}
		if index+1 >= len(records) {
			return nil, fmt.Errorf("%w: raw file record is incomplete", ErrHistoryContentUnavailable)
		}
		destinations = append(destinations, append([]byte(nil), records[index+1]...))
		index++
	}
	return destinations, nil
}

func selectHistoryPatchSection(output []byte, destinations [][]byte, path []byte) ([]byte, error) {
	sections := splitHistoryDiffSections(output)
	if len(sections) == 0 {
		return []byte{}, nil
	}
	if len(sections) != len(destinations) {
		if len(sections) == 1 {
			return sections[0], nil
		}
		return nil, fmt.Errorf("%w: historical patch sections do not match the file inventory", ErrHistoryContentUnavailable)
	}
	for index, destination := range destinations {
		if bytes.Equal(destination, path) {
			return sections[index], nil
		}
	}
	return nil, fmt.Errorf("%w: historical patch does not contain the selected file", ErrHistoryContentUnavailable)
}

func splitHistoryDiffSections(output []byte) [][]byte {
	const marker = "diff --git "
	if !bytes.HasPrefix(output, []byte(marker)) {
		first := bytes.Index(output, []byte("\n"+marker))
		if first < 0 {
			return nil
		}
		output = output[first+1:]
	}
	parts := bytes.Split(output, []byte("\n"+marker))
	sections := make([][]byte, 0, len(parts))
	for _, part := range parts {
		if len(part) == 0 {
			continue
		}
		sections = append(sections, append([]byte(nil), part...))
	}
	return sections
}

// CommonSourceUsable verifies a surviving common Git directory still holds
// the accepted commits and trees. It never captures a replacement interval.
func (s *GitHistoryStore) CommonSourceUsable(ctx context.Context, gitDir string, row models.TurnRepositoryChangeSet) bool {
	if !s.Enabled() || gitDir == "" {
		return false
	}
	if info, err := os.Stat(gitDir); err != nil || !info.IsDir() {
		return false
	}
	format, err := s.runGitBounded(ctx, "", gitDir, 1<<20, nil, "rev-parse", "--show-object-format")
	if err != nil || strings.TrimSpace(string(format)) != row.HashAlgorithm {
		return false
	}
	for _, endpoint := range []struct{ commit, tree string }{
		{row.StartCommitOID, row.StartTreeOID}, {row.EndCommitOID, row.EndTreeOID},
	} {
		output, err := s.runGitBounded(ctx, "", gitDir, 1<<20, nil, "rev-parse", "--verify", endpoint.commit+"^{tree}")
		if err != nil || strings.TrimSpace(string(output)) != endpoint.tree {
			return false
		}
	}
	return true
}

// ReadBlobAt returns one historical blob from a common Git source.
func (s *GitHistoryStore) ReadBlobAt(ctx context.Context, gitDir, oid, hashAlgorithm string) ([]byte, error) {
	if oid == "" || isZeroArchiveOID(oid) {
		return nil, nil
	}
	if !validArchiveOID(oid, hashAlgorithm) {
		return nil, fmt.Errorf("%w: historical blob identity is invalid", ErrHistoryContentUnavailable)
	}
	return s.runGitBounded(ctx, "", gitDir, maxHistoryReadRendering, nil, "cat-file", "blob", oid)
}

// ReadBlob returns one historical blob for old/new rendering requests.
func (s *GitHistoryStore) ReadBlob(ctx context.Context, receipt ArchiveReceipt, oid, hashAlgorithm string) ([]byte, error) {
	if oid == "" || isZeroArchiveOID(oid) {
		return nil, nil
	}
	if !validArchiveOID(oid, hashAlgorithm) {
		return nil, fmt.Errorf("%w: historical blob identity is invalid", ErrHistoryContentUnavailable)
	}
	repoPath := s.repositoryPath(receipt.SourceID)
	unlock := s.lockSource(receipt.SourceID)
	defer unlock()
	output, err := s.runGitBounded(ctx, "", repoPath, maxHistoryReadRendering, nil, "cat-file", "blob", oid)
	if err != nil {
		return nil, err
	}
	return output, nil
}

type historyDiffView struct {
	dir string
	env []string
}

func (s *GitHistoryStore) newHistoryDiffView(
	ctx context.Context,
	repoPath, endTree, hashAlgorithm string,
) (*historyDiffView, func(), error) {
	root, err := os.MkdirTemp("", "kandev-history-read-")
	if err != nil {
		return nil, nil, fmt.Errorf("%w: %v", ErrHistoryReadRetryable, err)
	}
	cleanup := func() { _ = os.RemoveAll(root) }
	if hashAlgorithm == "" {
		hashAlgorithm = historyHashSHA1
	}
	baseEnv := isolatedGitEnvironmentWith(map[string]string{"GIT_DEFAULT_HASH": hashAlgorithm})
	initArgs := []string{"init", "-q"}
	if hashAlgorithm == historyHashSHA256 {
		initArgs = append(initArgs, "--object-format=sha256")
	}
	if _, err := s.runGitAt(ctx, root, baseEnv, 1<<20, nil, initArgs...); err != nil {
		cleanup()
		return nil, nil, err
	}
	objects := filepath.Join(repoPath, "objects")
	env := isolatedGitEnvironmentWith(map[string]string{
		"GIT_OBJECT_DIRECTORY":   objects,
		"GIT_ATTR_SOURCE":        endTree,
		"GIT_CONFIG_COUNT":       "1",
		"GIT_CONFIG_KEY_0":       "core.attributesFile",
		"GIT_CONFIG_VALUE_0":     os.DevNull,
		"GIT_ATTR_NOSYSTEM":      "1",
		"GIT_TEMPLATE_DIR":       filepath.Join(root, "templates"),
		"GIT_DEFAULT_HASH":       hashAlgorithm,
		"GIT_NO_REPLACE_OBJECTS": "1",
	})
	if err := os.Mkdir(envValue(env, "GIT_TEMPLATE_DIR"), 0o700); err != nil && !os.IsExist(err) {
		cleanup()
		return nil, nil, fmt.Errorf("%w: %v", ErrHistoryReadRetryable, err)
	}
	if _, err := s.runGitAt(ctx, root, env, 32<<20, nil, "read-tree", endTree); err != nil {
		cleanup()
		return nil, nil, err
	}
	return &historyDiffView{dir: root, env: env}, cleanup, nil
}

func envValue(env []string, key string) string {
	prefix := key + "="
	for index := len(env) - 1; index >= 0; index-- {
		if strings.HasPrefix(env[index], prefix) {
			return strings.TrimPrefix(env[index], prefix)
		}
	}
	return ""
}

func (s *GitHistoryStore) runGitBounded(
	ctx context.Context,
	workDir, repoPath string,
	limit int,
	input []byte,
	args ...string,
) ([]byte, error) {
	environment := isolatedGitEnvironment()
	fullArgs := make([]string, 0, len(args)+2)
	if repoPath != "" {
		fullArgs = append(fullArgs, "--git-dir", repoPath)
	}
	fullArgs = append(fullArgs, args...)
	return s.runGitAtWithEnv(ctx, workDir, environment, fullArgs, limit, input)
}

func (s *GitHistoryStore) runGitAt(
	ctx context.Context,
	workDir string,
	environment []string,
	limit int,
	input []byte,
	args ...string,
) ([]byte, error) {
	return s.runGitAtWithEnv(ctx, workDir, environment, args, limit, input)
}

func (s *GitHistoryStore) runGitAtWithEnv(
	ctx context.Context,
	workDir string,
	environment []string,
	args []string,
	limit int,
	input []byte,
) ([]byte, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	stdout := &boundedGitBuffer{limit: limit}
	stderr := &boundedGitBuffer{limit: 64 << 10}
	runErr, contextErr := subproc.RunGitAfterAcquire(ctx, subproc.GitBackground, historyGitOperationTimeout*2, func(execCtx context.Context) *exec.Cmd {
		cmd := subproc.NewGitCommand(execCtx, args...)
		cmd.Dir = workDir
		cmd.Env = environment
		cmd.Stdout = stdout
		cmd.Stderr = stderr
		if input != nil {
			cmd.Stdin = bytes.NewReader(input)
		}
		return cmd
	})
	if contextErr != nil {
		return nil, fmt.Errorf("%w: %v", ErrHistoryReadRetryable, contextErr)
	}
	if stdout.truncated {
		return nil, fmt.Errorf("%w: historical output exceeds its response bound", ErrHistoryContentUnavailable)
	}
	if runErr != nil {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = runErr.Error()
		}
		if strings.Contains(message, "not a git repository") || strings.Contains(message, "does not exist") ||
			strings.Contains(message, "bad object") || strings.Contains(message, "missing") {
			return nil, fmt.Errorf("%w: %s", ErrHistoryContentUnavailable, message)
		}
		return nil, fmt.Errorf("%w: %s", ErrHistoryReadRetryable, message)
	}
	return append([]byte(nil), stdout.Bytes()...), nil
}

type boundedGitBuffer struct {
	buffer    bytes.Buffer
	limit     int
	truncated bool
}

func (b *boundedGitBuffer) Write(data []byte) (int, error) {
	if b.limit <= 0 {
		return b.buffer.Write(data)
	}
	remaining := b.limit - b.buffer.Len()
	if remaining <= 0 {
		b.truncated = true
		return len(data), nil
	}
	if len(data) > remaining {
		_, _ = b.buffer.Write(data[:remaining])
		b.truncated = true
		return len(data), nil
	}
	return b.buffer.Write(data)
}

func (b *boundedGitBuffer) Bytes() []byte  { return b.buffer.Bytes() }
func (b *boundedGitBuffer) Len() int       { return b.buffer.Len() }
func (b *boundedGitBuffer) String() string { return b.buffer.String() }

func isZeroArchiveOID(oid string) bool {
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

type limitedFileWriter struct {
	file      *os.File
	remaining int64
	truncated bool
}

func newLimitedFileWriter(file *os.File, limit int64) *limitedFileWriter {
	return &limitedFileWriter{file: file, remaining: limit}
}

func (w *limitedFileWriter) Write(data []byte) (int, error) {
	if w.remaining <= 0 {
		w.truncated = true
		return len(data), io.ErrShortWrite
	}
	if int64(len(data)) > w.remaining {
		written, err := w.file.Write(data[:w.remaining])
		w.remaining = 0
		w.truncated = true
		if err != nil {
			return written, err
		}
		return len(data), io.ErrShortWrite
	}
	written, err := w.file.Write(data)
	w.remaining -= int64(written)
	return written, err
}
