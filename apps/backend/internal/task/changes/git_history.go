package changes

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/common/subproc"
	"github.com/kandev/kandev/internal/common/turnchanges"
)

const (
	historyHashSHA1                    = "sha1"
	historyHashSHA256                  = "sha256"
	maxHistoryArchivePackBytes         = 32 << 20
	maxHistoryArchiveInstallationBytes = 1 << 30
	maxHistoryArchiveObjects           = 200_000
	maxHistoryReadPatchBytes           = 4 << 20
	maxHistoryReadRendering            = 8 << 20
	historyGitOperationTimeout         = 30 * time.Second
	historyGitRefCleanupTimeout        = 15 * time.Second
)

var (
	// ErrHistoryContentUnavailable reports permanently missing or corrupt
	// archive evidence. Callers must keep the verified snapshot metadata.
	ErrHistoryContentUnavailable = errors.New("historical content is unavailable")
	// ErrHistoryReadRetryable reports a temporary archive read failure such
	// as a timeout. It never changes durable availability by itself.
	ErrHistoryReadRetryable = errors.New("historical content is temporarily unavailable")
	// ErrHistoryRefMoved reports an owned ref that no longer carries the
	// accepted commit. Cleanup must not delete it and must keep its intent.
	ErrHistoryRefMoved = errors.New("owned history ref moved from its accepted commit")
)

var historySourceKeyPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)

// ArchiveReceipt is the durable proof that an interval's object closure lives
// in backend-owned Git storage. It carries only server-derived identities.
type ArchiveReceipt struct {
	SourceID          string    `json:"source_id"`
	HashAlgorithm     string    `json:"hash_algorithm"`
	PackSHA256        string    `json:"pack_sha256"`
	ObjectCount       int64     `json:"object_count"`
	StoredBytes       int64     `json:"stored_bytes"`
	UncompressedBytes int64     `json:"uncompressed_bytes"`
	StartRef          string    `json:"start_ref"`
	EndRef            string    `json:"end_ref"`
	StartCommitOID    string    `json:"start_commit_oid"`
	EndCommitOID      string    `json:"end_commit_oid"`
	PreservedAt       time.Time `json:"preserved_at"`
}

// GitHistoryStore owns backend-provided Git archives. One bare repository is
// kept per durable repository identity, with distinct refs per turn/checkout.
type GitHistoryStore struct {
	root       string
	now        func() time.Time
	mu         sync.Mutex
	keys       map[string]*sync.Mutex
	capacityMu sync.Mutex
	maxBytes   int64
}

func NewGitHistoryStore(root string, now func() time.Time) *GitHistoryStore {
	if now == nil {
		now = time.Now
	}
	return &GitHistoryStore{root: root, now: now, keys: make(map[string]*sync.Mutex), maxBytes: maxHistoryArchiveInstallationBytes}
}

// Enabled reports whether the store has a usable root. A nil store is a
// disabled integration, not an error.
func (s *GitHistoryStore) Enabled() bool {
	return s != nil && s.root != ""
}

func (s *GitHistoryStore) sourceKey(repositoryID, checkoutID string) string {
	candidate := strings.TrimSpace(repositoryID)
	if candidate == "" {
		candidate = strings.TrimSpace(checkoutID)
	}
	if historySourceKeyPattern.MatchString(candidate) {
		return candidate
	}
	sum := sha256.Sum256([]byte(candidate))
	return hex.EncodeToString(sum[:])
}

func (s *GitHistoryStore) lockSource(sourceID string) func() {
	s.mu.Lock()
	lock := s.keys[sourceID]
	if lock == nil {
		lock = &sync.Mutex{}
		s.keys[sourceID] = lock
	}
	s.mu.Unlock()
	lock.Lock()
	return lock.Unlock
}

func (s *GitHistoryStore) repositoryPath(sourceID string) string {
	return filepath.Join(s.root, sourceID+".git")
}

// Import verifies and stores an accepted object export. It is idempotent for
// the same endpoint pair: a verified archive is reused instead of duplicated.
// ImportWithCapacity serializes installation-wide capacity admission with
// import so concurrent repositories cannot overrun the bound.
func (s *GitHistoryStore) ImportWithCapacity(
	ctx context.Context,
	repositoryID, checkoutID string,
	archive *turnchanges.CheckpointArchive,
) (ArchiveReceipt, error) {
	s.capacityMu.Lock()
	defer s.capacityMu.Unlock()
	if !s.HasCapacity(ctx, int64(len(archive.Pack))) {
		return ArchiveReceipt{}, fmt.Errorf("%w: installation archive capacity is exhausted", ErrHistoryContentUnavailable)
	}
	return s.Import(ctx, repositoryID, checkoutID, archive)
}

func (s *GitHistoryStore) Import(
	ctx context.Context,
	repositoryID, checkoutID string,
	archive *turnchanges.CheckpointArchive,
) (ArchiveReceipt, error) {
	if !s.Enabled() {
		return ArchiveReceipt{}, fmt.Errorf("%w: Git history store is not configured", ErrHistoryContentUnavailable)
	}
	if err := validateAcceptedArchive(archive); err != nil {
		return ArchiveReceipt{}, err
	}
	sourceID := s.sourceKey(repositoryID, checkoutID)
	unlock := s.lockSource(sourceID)
	defer unlock()
	if err := os.MkdirAll(s.root, 0o700); err != nil {
		return ArchiveReceipt{}, fmt.Errorf("%w: prepare history store: %v", ErrHistoryReadRetryable, err)
	}
	repoPath := s.repositoryPath(sourceID)
	if err := s.ensureRepository(ctx, repoPath, archive.HashAlgorithm); err != nil {
		return ArchiveReceipt{}, err
	}
	receipt := newArchiveReceipt(sourceID, archive, s.now().UTC())
	if existing, ok := s.verifiedReceipt(ctx, repoPath, receipt); ok {
		existing.PreservedAt = receipt.PreservedAt
		return existing, nil
	}
	if len(archive.Pack) > 0 {
		if err := s.importPack(ctx, repoPath, receipt.PackSHA256, archive.Pack); err != nil {
			return ArchiveReceipt{}, err
		}
	}
	if err := s.verifyObjects(ctx, repoPath, archive.Objects); err != nil {
		return ArchiveReceipt{}, err
	}
	if err := s.verifyEndpointIdentities(ctx, repoPath, archive); err != nil {
		return ArchiveReceipt{}, err
	}
	closure, err := s.archiveObjectClosure(ctx, repoPath, archive.StartTreeOID, archive.EndTreeOID, archive.StartCommitOID, archive.EndCommitOID)
	if err != nil {
		return ArchiveReceipt{}, err
	}
	if err := s.verifyObjects(ctx, repoPath, closure); err != nil {
		return ArchiveReceipt{}, err
	}
	if err := s.ensureArchiveRefs(ctx, repoPath, receipt); err != nil {
		return ArchiveReceipt{}, err
	}
	return receipt, nil
}

func validateAcceptedArchive(archive *turnchanges.CheckpointArchive) error {
	if archive == nil || !archive.Complete || len(archive.Objects) == 0 {
		return fmt.Errorf("%w: accepted object export is incomplete", ErrHistoryContentUnavailable)
	}
	if err := validateArchiveEndpoints(archive); err != nil {
		return err
	}
	if err := validateArchiveBounds(archive); err != nil {
		return err
	}
	return validateArchiveObjects(archive)
}

func validateArchiveEndpoints(archive *turnchanges.CheckpointArchive) error {
	if archive.HashAlgorithm != historyHashSHA1 && archive.HashAlgorithm != historyHashSHA256 {
		return fmt.Errorf("%w: unsupported archive hash algorithm", ErrHistoryContentUnavailable)
	}
	if !validArchiveIdentity(archive.ChangeSetID) || !validArchiveIdentity(archive.CheckoutID) {
		return fmt.Errorf("%w: invalid archive identity", ErrHistoryContentUnavailable)
	}
	if archive.StartCommitOID == "" || archive.StartTreeOID == "" || archive.EndCommitOID == "" || archive.EndTreeOID == "" {
		return fmt.Errorf("%w: archive must describe an accepted endpoint pair", ErrHistoryContentUnavailable)
	}
	return nil
}

func validateArchiveBounds(archive *turnchanges.CheckpointArchive) error {
	if len(archive.Pack) > maxHistoryArchivePackBytes || len(archive.Objects) == 0 || len(archive.Objects) > maxHistoryArchiveObjects {
		return fmt.Errorf("%w: archive exceeds storage bounds", ErrHistoryContentUnavailable)
	}
	if archive.ObjectCount != int64(len(archive.Objects)) {
		return fmt.Errorf("%w: archive object inventory is inconsistent", ErrHistoryContentUnavailable)
	}
	return nil
}

func validateArchiveObjects(archive *turnchanges.CheckpointArchive) error {
	seen := make(map[string]struct{}, len(archive.Objects))
	for _, oid := range archive.Objects {
		if !validArchiveOID(oid, archive.HashAlgorithm) {
			return fmt.Errorf("%w: archive contains an invalid object identity", ErrHistoryContentUnavailable)
		}
		if _, exists := seen[oid]; exists {
			return fmt.Errorf("%w: archive contains duplicate objects", ErrHistoryContentUnavailable)
		}
		seen[oid] = struct{}{}
	}
	return nil
}

func validArchiveIdentity(value string) bool {
	parsed, err := uuid.Parse(value)
	return err == nil && parsed.String() == value
}

func validArchiveOID(oid, algorithm string) bool {
	expected := 40
	if algorithm == historyHashSHA256 {
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
	return true
}

func newArchiveReceipt(sourceID string, archive *turnchanges.CheckpointArchive, now time.Time) ArchiveReceipt {
	digest := sha256.Sum256(archive.Pack)
	return ArchiveReceipt{
		SourceID:          sourceID,
		HashAlgorithm:     archive.HashAlgorithm,
		PackSHA256:        hex.EncodeToString(digest[:]),
		ObjectCount:       archive.ObjectCount,
		StoredBytes:       int64(len(archive.Pack)),
		UncompressedBytes: archive.UncompressedBytes,
		StartRef:          historyRef(archive.ChangeSetID, archive.CheckoutID, "start"),
		EndRef:            historyRef(archive.ChangeSetID, archive.CheckoutID, "end"),
		StartCommitOID:    archive.StartCommitOID,
		EndCommitOID:      archive.EndCommitOID,
		PreservedAt:       now,
	}
}

func historyRef(changeSetID, checkoutID, boundary string) string {
	return "refs/kandev/history/" + changeSetID + "/" + checkoutID + "/" + boundary
}

func (s *GitHistoryStore) ensureRepository(ctx context.Context, repoPath, hashAlgorithm string) error {
	if info, err := os.Stat(filepath.Join(repoPath, "HEAD")); err == nil && info.Mode().IsRegular() {
		format, formatErr := s.git(ctx, "", repoPath, historyGitOperationTimeout, "rev-parse", "--show-object-format")
		if formatErr != nil {
			return formatErr
		}
		if strings.TrimSpace(string(format)) != hashAlgorithm {
			return fmt.Errorf("%w: retained history source uses a different object format", ErrHistoryContentUnavailable)
		}
		return nil
	}
	if err := os.MkdirAll(repoPath, 0o700); err != nil {
		return fmt.Errorf("%w: prepare history repository: %v", ErrHistoryReadRetryable, err)
	}
	initArgs := []string{"init", "--bare", "-q"}
	if hashAlgorithm == historyHashSHA256 {
		initArgs = append(initArgs, "--object-format=sha256")
	}
	initArgs = append(initArgs, repoPath)
	if _, err := s.git(ctx, "", "", historyGitOperationTimeout, initArgs...); err != nil {
		return err
	}
	return nil
}

func (s *GitHistoryStore) verifiedReceipt(
	ctx context.Context,
	repoPath string,
	receipt ArchiveReceipt,
) (ArchiveReceipt, bool) {
	start, err := s.readRef(ctx, repoPath, receipt.StartRef)
	if err != nil || start != receipt.StartCommitOID {
		return ArchiveReceipt{}, false
	}
	end, err := s.readRef(ctx, repoPath, receipt.EndRef)
	if err != nil || end != receipt.EndCommitOID {
		return ArchiveReceipt{}, false
	}
	if _, err := s.git(ctx, "", repoPath, historyGitOperationTimeout, "cat-file", "-e", receipt.StartCommitOID+"^{commit}"); err != nil {
		return ArchiveReceipt{}, false
	}
	if _, err := s.git(ctx, "", repoPath, historyGitOperationTimeout, "cat-file", "-e", receipt.EndCommitOID+"^{commit}"); err != nil {
		return ArchiveReceipt{}, false
	}
	return receipt, true
}

func (s *GitHistoryStore) importPack(ctx context.Context, repoPath, digest string, pack []byte) error {
	packDir := filepath.Join(repoPath, "objects", "pack")
	if err := os.MkdirAll(packDir, 0o700); err != nil {
		return fmt.Errorf("%w: prepare history pack directory: %v", ErrHistoryReadRetryable, err)
	}
	if _, err := os.Stat(filepath.Join(packDir, "pack-"+digest+".idx")); err == nil {
		return nil
	}
	packPath := filepath.Join(packDir, "pack-"+digest+".pack")
	if err := os.WriteFile(packPath, pack, 0o600); err != nil {
		return fmt.Errorf("%w: stage history pack: %v", ErrHistoryReadRetryable, err)
	}
	if _, err := s.git(ctx, "", repoPath, historyGitOperationTimeout*2, "index-pack", packPath); err != nil {
		_ = os.Remove(packPath)
		return err
	}
	return nil
}

func (s *GitHistoryStore) verifyObjects(ctx context.Context, repoPath string, objects []string) error {
	input := strings.Join(objects, "\n") + "\n"
	output, err := s.runGitBounded(ctx, "", repoPath, 32<<20, []byte(input),
		"cat-file", "--batch-check=%(objectname) %(objecttype)")
	if err != nil {
		return err
	}
	lines := strings.Split(strings.TrimSuffix(string(output), "\n"), "\n")
	if len(lines) != len(objects) {
		return fmt.Errorf("%w: stored archive is missing objects", ErrHistoryContentUnavailable)
	}
	for _, line := range lines {
		if strings.HasSuffix(line, " missing") || line == "" {
			return fmt.Errorf("%w: stored archive is missing objects", ErrHistoryContentUnavailable)
		}
	}
	return nil
}

func (s *GitHistoryStore) verifyEndpointIdentities(
	ctx context.Context,
	repoPath string,
	archive *turnchanges.CheckpointArchive,
) error {
	for _, endpoint := range []struct{ commit, tree string }{
		{archive.StartCommitOID, archive.StartTreeOID},
		{archive.EndCommitOID, archive.EndTreeOID},
	} {
		output, err := s.git(ctx, "", repoPath, historyGitOperationTimeout, "rev-parse", "--verify", endpoint.commit+"^{tree}")
		if err != nil {
			return err
		}
		if strings.TrimSpace(string(output)) != endpoint.tree {
			return fmt.Errorf("%w: stored archive endpoints do not match their trees", ErrHistoryContentUnavailable)
		}
	}
	return nil
}

func (s *GitHistoryStore) ensureArchiveRefs(ctx context.Context, repoPath string, receipt ArchiveReceipt) error {
	if err := s.updateRef(ctx, repoPath, receipt.StartRef, receipt.StartCommitOID); err != nil {
		return err
	}
	return s.updateRef(ctx, repoPath, receipt.EndRef, receipt.EndCommitOID)
}

func (s *GitHistoryStore) updateRef(ctx context.Context, repoPath, ref, commit string) error {
	zero := strings.Repeat("0", len(commit))
	if _, err := s.git(ctx, "", repoPath, historyGitOperationTimeout, "update-ref", ref, commit, zero); err == nil {
		return nil
	}
	current, readErr := s.readRef(ctx, repoPath, ref)
	if readErr == nil && current == commit {
		return nil
	}
	return fmt.Errorf("%w: history ref could not be recorded", ErrHistoryReadRetryable)
}

func (s *GitHistoryStore) readRef(ctx context.Context, repoPath, ref string) (string, error) {
	output, err := s.git(ctx, "", repoPath, historyGitOperationTimeout, "for-each-ref", "--format=%(objectname)", ref)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

// DeleteRefs removes only the exact turn-owned refs recorded in the receipt.
// A moved ref is left untouched and reported as unavailable for cleanup.
func (s *GitHistoryStore) DeleteRefs(ctx context.Context, receipt ArchiveReceipt) error {
	if !s.Enabled() {
		return nil
	}
	repoPath := s.repositoryPath(receipt.SourceID)
	unlock := s.lockSource(receipt.SourceID)
	defer unlock()
	return s.deleteRefsAt(ctx, repoPath, []historyRefExpectation{
		{name: receipt.StartRef, commit: receipt.StartCommitOID},
		{name: receipt.EndRef, commit: receipt.EndCommitOID},
	})
}

// DeleteCommonRefs removes executor-owned checkpoint refs from a surviving
// local common repository only when they still carry the accepted commits.
func (s *GitHistoryStore) DeleteCommonRefs(ctx context.Context, gitDir string, refs ...historyRefExpectation) error {
	if !s.Enabled() || gitDir == "" {
		return nil
	}
	unlock := s.lockSource(filepath.Base(gitDir))
	defer unlock()
	return s.deleteRefsAt(ctx, gitDir, refs)
}

type historyRefExpectation struct {
	name   string
	commit string
}

func (s *GitHistoryStore) deleteRefsAt(ctx context.Context, repoPath string, refs []historyRefExpectation) error {
	for _, ref := range refs {
		if ref.name == "" || ref.commit == "" {
			continue
		}
		current, err := s.readRef(ctx, repoPath, ref.name)
		if err != nil {
			if errors.Is(err, ErrHistoryContentUnavailable) {
				continue
			}
			return err
		}
		if current == "" {
			continue
		}
		if current != ref.commit {
			return fmt.Errorf("%w: %s", ErrHistoryRefMoved, ref.name)
		}
		if _, err := s.git(ctx, "", repoPath, historyGitRefCleanupTimeout, "update-ref", "-d", ref.name, ref.commit); err != nil {
			return err
		}
	}
	return nil
}

// VerifyRefs reports whether the receipt's refs and objects still exist. It is
// used by backup preparation and read leases before destructive work.
func (s *GitHistoryStore) VerifyRefs(ctx context.Context, receipt ArchiveReceipt) error {
	repoPath := s.repositoryPath(receipt.SourceID)
	unlock := s.lockSource(receipt.SourceID)
	defer unlock()
	if _, ok := s.verifiedReceipt(ctx, repoPath, receipt); !ok {
		return fmt.Errorf("%w: retained history source is missing", ErrHistoryContentUnavailable)
	}
	return nil
}

// ObjectStoreGrowth reports the actual pack bytes currently owned by this
// archive. Shared-object savings are not inferred.
func (s *GitHistoryStore) ObjectStoreGrowth(ctx context.Context, sourceIDs []string) (int64, error) {
	var total int64
	for _, sourceID := range sourceIDs {
		if !historySourceKeyPattern.MatchString(sourceID) {
			continue
		}
		entries, err := os.ReadDir(filepath.Join(s.repositoryPath(sourceID), "objects", "pack"))
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return 0, err
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".pack") {
				continue
			}
			info, err := entry.Info()
			if err != nil {
				return 0, err
			}
			total += info.Size()
		}
	}
	return total, nil
}

// SortedSourceIDs returns a deterministic source list for administrative
// inventory without exposing repository paths.
func (s *GitHistoryStore) SortedSourceIDs() []string {
	if !s.Enabled() {
		return nil
	}
	entries, err := os.ReadDir(s.root)
	if err != nil {
		return nil
	}
	ids := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasSuffix(entry.Name(), ".git") {
			continue
		}
		ids = append(ids, strings.TrimSuffix(entry.Name(), ".git"))
	}
	sort.Strings(ids)
	return ids
}

// HistoricalFile is the server-owned file identity used for a Git-backed read.
type HistoricalFile struct {
	PathBytes    []byte
	OldPathBytes []byte
	OldBlobOID   string
	NewBlobOID   string
}

func (s *GitHistoryStore) git(
	ctx context.Context,
	workDir, repoPath string,
	timeout time.Duration,
	args ...string,
) ([]byte, error) {
	return s.gitWithInputAndWorkDir(ctx, workDir, repoPath, timeout, nil, args...)
}

func (s *GitHistoryStore) gitWithInputAndWorkDir(
	ctx context.Context,
	workDir, repoPath string,
	timeout time.Duration,
	input []byte,
	args ...string,
) ([]byte, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	fullArgs := make([]string, 0, len(args)+2)
	if repoPath != "" {
		fullArgs = append(fullArgs, "--git-dir", repoPath)
	}
	fullArgs = append(fullArgs, args...)
	out, runErr, contextErr := subproc.RunGitOutputAfterAcquire(ctx, subproc.GitBackground, timeout, func(execCtx context.Context) *exec.Cmd {
		cmd := subproc.NewGitCommand(execCtx, fullArgs...)
		if workDir != "" {
			cmd.Dir = workDir
		}
		cmd.Env = isolatedGitEnvironment()
		if input != nil {
			cmd.Stdin = strings.NewReader(string(input))
		}
		return cmd
	})
	if contextErr != nil {
		return nil, fmt.Errorf("%w: %v", ErrHistoryReadRetryable, contextErr)
	}
	if runErr != nil {
		message := strings.TrimSpace(subprocGitError(runErr))
		if strings.Contains(message, "not a git repository") || strings.Contains(message, "does not exist") ||
			strings.Contains(message, "bad object") || strings.Contains(message, "missing") {
			return nil, fmt.Errorf("%w: %s", ErrHistoryContentUnavailable, message)
		}
		return nil, fmt.Errorf("%w: %s", ErrHistoryReadRetryable, message)
	}
	return append([]byte(nil), out...), nil
}

func subprocGitError(err error) string {
	if err == nil {
		return ""
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && len(exitErr.Stderr) > 0 {
		return string(exitErr.Stderr)
	}
	return err.Error()
}

func isolatedGitEnvironment() []string {
	return isolatedGitEnvironmentWith(map[string]string{
		"GIT_CONFIG_NOSYSTEM":    "1",
		"GIT_CONFIG_GLOBAL":      os.DevNull,
		"GIT_CONFIG_PARAMETERS":  "",
		"GIT_ATTR_NOSYSTEM":      "1",
		"GIT_NO_REPLACE_OBJECTS": "1",
		"GIT_TERMINAL_PROMPT":    "0",
	})
}

func isolatedGitEnvironmentWith(overrides map[string]string) []string {
	filtered := make([]string, 0, len(os.Environ())+len(overrides))
	for _, entry := range os.Environ() {
		key, _, ok := strings.Cut(entry, "=")
		if !ok {
			continue
		}
		if _, overridden := overrides[key]; overridden {
			continue
		}
		filtered = append(filtered, entry)
	}
	keys := make([]string, 0, len(overrides))
	for key := range overrides {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		filtered = append(filtered, key+"="+overrides[key])
	}
	return filtered
}

// ExportPack repacks the accepted closure from the backend store for a backup
// artifact. It never depends on the original executor or worktree.
func (s *GitHistoryStore) ExportPack(
	ctx context.Context,
	receipt ArchiveReceipt,
) ([]byte, []string, string, string, error) {
	repoPath := s.repositoryPath(receipt.SourceID)
	unlock := s.lockSource(receipt.SourceID)
	defer unlock()
	startTree, err := s.archiveCommitTree(ctx, repoPath, receipt.StartCommitOID)
	if err != nil {
		return nil, nil, "", "", err
	}
	endTree, err := s.archiveCommitTree(ctx, repoPath, receipt.EndCommitOID)
	if err != nil {
		return nil, nil, "", "", err
	}
	objects, err := s.archiveObjectClosure(ctx, repoPath, startTree, endTree, receipt.StartCommitOID, receipt.EndCommitOID)
	if err != nil {
		return nil, nil, "", "", err
	}
	pack, err := s.archivePack(ctx, repoPath, objects)
	if err != nil {
		return nil, nil, "", "", err
	}
	return pack, objects, startTree, endTree, nil
}

func (s *GitHistoryStore) archiveCommitTree(ctx context.Context, repoPath, commit string) (string, error) {
	output, err := s.git(ctx, "", repoPath, historyGitOperationTimeout, "rev-parse", "--verify", commit+"^{tree}")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

func (s *GitHistoryStore) archiveObjectClosure(
	ctx context.Context,
	repoPath, startTree, endTree, startCommit, endCommit string,
) ([]string, error) {
	seen := make(map[string]struct{}, 1024)
	objects := make([]string, 0, 1024)
	for _, root := range []string{startTree, endTree} {
		output, err := s.runGitBounded(ctx, "", repoPath, 32<<20, nil, "ls-tree", "-r", "-t", "-z", "--full-tree", root)
		if err != nil {
			return nil, err
		}
		for _, record := range strings.Split(string(output), "\x00") {
			metadata, _, ok := strings.Cut(record, "\t")
			if !ok {
				continue
			}
			fields := strings.Fields(metadata)
			if len(fields) != 3 || fields[1] == "commit" {
				continue
			}
			if _, exists := seen[fields[2]]; exists {
				continue
			}
			seen[fields[2]] = struct{}{}
			objects = append(objects, fields[2])
		}
		if _, exists := seen[root]; !exists {
			seen[root] = struct{}{}
			objects = append(objects, root)
		}
	}
	for _, commit := range []string{startCommit, endCommit} {
		if _, exists := seen[commit]; !exists {
			seen[commit] = struct{}{}
			objects = append(objects, commit)
		}
	}
	sort.Strings(objects)
	return objects, nil
}

func (s *GitHistoryStore) archivePack(ctx context.Context, repoPath string, objects []string) ([]byte, error) {
	packFile, err := os.CreateTemp(s.root, "backup-pack-*.pack")
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrHistoryReadRetryable, err)
	}
	packPath := packFile.Name()
	defer func() {
		_ = packFile.Close()
		_ = os.Remove(packPath)
	}()
	input := strings.Join(objects, "\n") + "\n"
	writer := newLimitedFileWriter(packFile, maxHistoryArchivePackBytes)
	runErr, contextErr := subproc.RunGitAfterAcquire(ctx, subproc.GitBackground, historyGitOperationTimeout*2, func(execCtx context.Context) *exec.Cmd {
		cmd := subproc.NewGitCommand(execCtx, "--git-dir", repoPath, "pack-objects", "--stdout", "-q")
		cmd.Env = isolatedGitEnvironment()
		cmd.Stdin = strings.NewReader(input)
		cmd.Stdout = writer
		return cmd
	})
	if contextErr != nil {
		return nil, fmt.Errorf("%w: %v", ErrHistoryReadRetryable, contextErr)
	}
	if writer.truncated {
		return nil, fmt.Errorf("%w: backup pack exceeds its bound", ErrHistoryContentUnavailable)
	}
	if runErr != nil {
		return nil, fmt.Errorf("%w: %v", ErrHistoryReadRetryable, runErr)
	}
	if err := packFile.Close(); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrHistoryReadRetryable, err)
	}
	pack, err := os.ReadFile(packPath)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrHistoryReadRetryable, err)
	}
	if len(pack) > maxHistoryArchivePackBytes {
		return nil, fmt.Errorf("%w: backup pack exceeds its bound", ErrHistoryContentUnavailable)
	}
	return pack, nil
}

// VerifyPack proves one retention backup pack is self-contained: its objects,
// commits, and interval trees are all restorable without the live archive.
func (s *GitHistoryStore) VerifyPack(ctx context.Context, pack []byte, source HistoryBackupSource) error {
	if len(pack) == 0 || len(pack) > maxHistoryArchivePackBytes {
		return fmt.Errorf("%w: retention checkpoint pack is outside bounds", ErrHistoryContentUnavailable)
	}
	root, err := os.MkdirTemp("", "kandev-history-verify-")
	if err != nil {
		return fmt.Errorf("%w: %v", ErrHistoryReadRetryable, err)
	}
	defer func() { _ = os.RemoveAll(root) }()
	repoPath := filepath.Join(root, "verify.git")
	initArgs := []string{"init", "--bare", "-q"}
	if source.HashAlgorithm == historyHashSHA256 {
		initArgs = append(initArgs, "--object-format=sha256")
	}
	initArgs = append(initArgs, repoPath)
	if _, err := s.git(ctx, "", "", historyGitOperationTimeout, initArgs...); err != nil {
		return err
	}
	packDir := filepath.Join(repoPath, "objects", "pack")
	if err := os.MkdirAll(packDir, 0o700); err != nil {
		return fmt.Errorf("%w: %v", ErrHistoryReadRetryable, err)
	}
	packPath := filepath.Join(packDir, "verify.pack")
	if err := os.WriteFile(packPath, pack, 0o600); err != nil {
		return fmt.Errorf("%w: %v", ErrHistoryReadRetryable, err)
	}
	if _, err := s.git(ctx, "", repoPath, historyGitOperationTimeout*2, "index-pack", packPath); err != nil {
		return err
	}
	if err := s.verifyObjects(ctx, repoPath, source.Objects); err != nil {
		return err
	}
	archive := &turnchanges.CheckpointArchive{
		StartCommitOID: source.StartCommitOID, StartTreeOID: source.StartTreeOID,
		EndCommitOID: source.EndCommitOID, EndTreeOID: source.EndTreeOID,
	}
	return s.verifyEndpointIdentities(ctx, repoPath, archive)
}

// SourceObjects returns the object identities already stored for one source.
// The executor uses them to transfer only missing closure objects.
func (s *GitHistoryStore) SourceObjects(ctx context.Context, repositoryID, checkoutID string) []string {
	if !s.Enabled() {
		return nil
	}
	sourceID := s.sourceKey(repositoryID, checkoutID)
	repoPath := s.repositoryPath(sourceID)
	if _, err := os.Stat(filepath.Join(repoPath, "HEAD")); err != nil {
		return nil
	}
	unlock := s.lockSource(sourceID)
	defer unlock()
	output, err := s.runGitBounded(ctx, "", repoPath, 32<<20, nil, "cat-file", "--batch-all-objects", "--batch-check=%(objectname)")
	if err != nil {
		return nil
	}
	objects := make([]string, 0, 1024)
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		oid, _, _ := strings.Cut(line, " ")
		if oid != "" && (validArchiveOID(oid, "sha1") || validArchiveOID(oid, "sha256")) {
			objects = append(objects, oid)
		}
	}
	if len(objects) > maxHistoryArchiveObjects {
		return objects[:maxHistoryArchiveObjects]
	}
	return objects
}

// ExportPackAt repacks an accepted interval from a surviving common Git
// directory for one retention backup.
func (s *GitHistoryStore) ExportPackAt(
	ctx context.Context, gitDir, startCommit, endCommit, startTree, endTree string,
) ([]byte, []string, string, string, error) {
	objects, err := s.archiveObjectClosure(ctx, gitDir, startTree, endTree, startCommit, endCommit)
	if err != nil {
		return nil, nil, "", "", err
	}
	pack, err := s.archivePack(ctx, gitDir, objects)
	if err != nil {
		return nil, nil, "", "", err
	}
	return pack, objects, startTree, endTree, nil
}
