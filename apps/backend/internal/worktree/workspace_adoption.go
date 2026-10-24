package worktree

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"

	storageworkspaces "github.com/kandev/kandev/internal/system/storage/workspaces"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/recoveryartifact"
)

// WorkspaceAdoptionInspection pins every inspected directory until publication.
type WorkspaceAdoptionInspection struct {
	Proof      models.WorkspaceAdoptionProof
	manager    *Manager
	handles    map[string]storageworkspaces.DirectoryHandle
	marker     []byte
	source     string
	selection  models.WorkspaceAdoptionSelection
	sourceHead string
	root       string
}

func (i *WorkspaceAdoptionInspection) Close() {
	for _, h := range i.handles {
		_ = h.Close()
	}
}

func adoptionHash(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func (m *Manager) InspectWorkspaceAdoption(ctx context.Context, selection models.WorkspaceAdoptionSelection, slotID, candidate string) (*WorkspaceAdoptionInspection, error) {
	return m.inspectWorkspaceAdoption(ctx, selection, slotID, candidate, "")
}

func (m *Manager) inspectWorkspaceAdoption(ctx context.Context, selection models.WorkspaceAdoptionSelection, slotID, candidate, sourceHead string) (*WorkspaceAdoptionInspection, error) {
	p := models.WorkspaceAdoptionPreview{Selection: selection, SlotID: slotID}
	slot, err := p.Slot()
	if err != nil {
		return nil, err
	}
	base, err := m.TasksBasePath()
	if err != nil {
		return nil, err
	}
	base, err = filepath.Abs(base)
	if err != nil {
		return nil, err
	}
	if !validWorkspaceAdoptionPaths(selection, slot, candidate, base) {
		return nil, models.ErrWorkspaceAdoptionStale
	}
	i := &WorkspaceAdoptionInspection{manager: m, selection: selection, sourceHead: sourceHead, handles: make(map[string]storageworkspaces.DirectoryHandle)}
	if err := i.pinCheckout(selection, slot.WorktreePath); err != nil {
		i.Close()
		return nil, err
	}
	if err := i.pinCheckout(selection, candidate); err != nil {
		i.Close()
		return nil, err
	}
	i.source = slot.WorktreeSourceClonePath
	if i.source == "" {
		i.source = slot.RepositoryLocalPath
	}
	if i.source == "" {
		i.Close()
		return nil, models.ErrWorkspaceAdoptionStale
	}
	if err := i.pinRoot(selection); err != nil {
		i.Close()
		return nil, err
	}
	i.Proof.OriginalPath = slot.WorktreePath
	i.Proof.CandidatePath = candidate
	i.Proof.Branch = slot.WorktreeBranch
	if err := i.pinSourceMetadata(ctx); err != nil {
		i.Close()
		return nil, err
	}
	if err := i.inspectOldMarker(); err != nil {
		i.Close()
		return nil, err
	}
	if err := i.inspectCandidate(ctx); err != nil {
		i.Close()
		return nil, err
	}
	if err := i.pinIdentityPaths(); err != nil {
		i.Close()
		return nil, err
	}
	return i, nil
}

func (i *WorkspaceAdoptionInspection) pinCheckout(selection models.WorkspaceAdoptionSelection, path string) error {
	h, err := i.manager.validateWorktreePathSafe(path)
	if err != nil {
		return err
	}
	if h == nil {
		return models.ErrWorkspaceAdoptionStale
	}
	i.handles[path] = h
	wt := &Worktree{TaskID: selection.Snapshot.EnvironmentOwnerTaskID, TaskDirName: selection.Snapshot.TaskDirName}
	return i.manager.validateExistingWorktreePathOwner(path, wt)
}

func (i *WorkspaceAdoptionInspection) inspectOldMarker() error {
	h := i.handles[i.Proof.OriginalPath]
	mode, err := h.LstatEntry(".git")
	if err != nil {
		return err
	}
	if !mode.IsRegular() {
		return models.ErrWorkspaceAdoptionStale
	}
	i.marker, err = h.ReadFile(".git")
	if err != nil {
		return err
	}
	target, ok := strings.CutPrefix(strings.TrimSpace(string(i.marker)), "gitdir:")
	if !ok || strings.TrimSpace(target) == "" {
		return models.ErrWorkspaceAdoptionStale
	}
	target = strings.TrimSpace(target)
	if !filepath.IsAbs(target) {
		target = filepath.Join(i.Proof.OriginalPath, target)
	}
	target = filepath.Clean(target)
	if err := validateMissingLinkedWorktreeAdmin(i.source, target); err != nil {
		return err
	}
	if err := i.pinRetiredAdministrationParent(target); err != nil {
		return err
	}
	i.Proof.MarkerSHA256 = adoptionHash(i.marker)
	i.Proof.MissingGitDir = target
	return nil
}

func (i *WorkspaceAdoptionInspection) pinRetiredAdministrationParent(target string) error {
	if filepath.Dir(target) != filepath.Join(i.source, ".git", "worktrees") {
		return models.ErrWorkspaceAdoptionStale
	}
	metadata := i.handles[filepath.Join(i.source, ".git")]
	mode, err := metadata.LstatEntry("worktrees")
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !mode.IsDir() || mode&os.ModeSymlink != 0 {
		return models.ErrWorkspaceAdoptionStale
	}
	parent, err := metadata.OpenSubdirectory("worktrees")
	if err != nil {
		return err
	}
	path := filepath.Join(i.source, ".git", "worktrees")
	i.handles[path] = parent
	if err := parent.VerifyPath(path); err != nil {
		return err
	}
	if _, err := parent.LstatEntry(filepath.Base(target)); !errors.Is(err, os.ErrNotExist) {
		return models.ErrWorkspaceAdoptionStale
	}
	return nil
}

func (i *WorkspaceAdoptionInspection) verifyRetiredAdministration() error {
	path := filepath.Join(i.source, ".git", "worktrees")
	parent := i.handles[path]
	if parent == nil {
		if _, err := i.handles[filepath.Join(i.source, ".git")].LstatEntry("worktrees"); !errors.Is(err, os.ErrNotExist) {
			return models.ErrWorkspaceAdoptionStale
		}
		return nil
	}
	if err := parent.VerifyPath(path); err != nil {
		return err
	}
	if _, err := parent.LstatEntry(filepath.Base(i.Proof.MissingGitDir)); !errors.Is(err, os.ErrNotExist) {
		return models.ErrWorkspaceAdoptionStale
	}
	return nil
}

func (i *WorkspaceAdoptionInspection) inspectCandidate(ctx context.Context) error {
	path := i.Proof.CandidatePath
	metadata, err := i.candidateMetadata()
	if err != nil {
		return err
	}
	if err := validateAdoptionMetadata(ctx, metadata, filepath.Join(path, ".git")); err != nil {
		return err
	}
	inspected := i.manager.inspectCheckout(ctx, path, i.handles[path])
	if inspected.operationalErr != nil {
		return inspected.operationalErr
	}
	if inspected.class != checkoutMainHealthy {
		return models.ErrWorkspaceAdoptionStale
	}
	origin, err := i.rawGitInspect(ctx, i.source, "config", "--get", "remote.origin.url")
	if err != nil {
		return err
	}
	other, err := i.rawGitInspect(ctx, path, "config", "--get", "remote.origin.url")
	if err != nil {
		return err
	}
	if strings.TrimSpace(origin) == "" || strings.TrimSpace(origin) != strings.TrimSpace(other) {
		return models.ErrWorkspaceAdoptionStale
	}
	i.Proof.OriginSHA256 = adoptionHash([]byte(strings.TrimSpace(origin)))
	branch, err := i.rawGitInspect(ctx, path, "symbolic-ref", "--short", "HEAD")
	if err != nil {
		return err
	}
	if strings.TrimSpace(branch) != i.Proof.Branch {
		return models.ErrWorkspaceAdoptionStale
	}
	head, err := i.rawGitInspect(ctx, path, "rev-parse", "--verify", "HEAD^{commit}")
	if err != nil {
		return err
	}
	i.Proof.Head = strings.TrimSpace(head)
	if err := i.inspectCandidateHistory(ctx, path); err != nil {
		return err
	}
	index, err := metadata.ReadFile("index")
	if err != nil {
		return err
	}
	i.Proof.IndexSHA256 = adoptionHash(index)
	return validateAdoptionMetadata(ctx, metadata, filepath.Join(path, ".git"))
}

func (i *WorkspaceAdoptionInspection) pinIdentityPaths() error {
	i.Proof.Identities = make(map[string]string)
	for path, h := range i.handles {
		if err := h.VerifyPath(path); err != nil {
			return err
		}
		identity, ok := recoveryartifact.FilesystemIdentity(path)
		if !ok {
			return models.ErrWorkspaceAdoptionStale
		}
		i.Proof.Identities[path] = identity
	}
	for _, path := range []string{filepath.Join(i.Proof.OriginalPath, ".git"), filepath.Join(i.root, storageworkspaces.OwnershipMarkerFilename)} {
		identity, ok := recoveryartifact.FilesystemIdentity(path)
		if !ok {
			return models.ErrWorkspaceAdoptionStale
		}
		i.Proof.Identities[path] = identity
	}
	return nil
}

// Verify rechecks pinned objects and immutable Git proof before transaction commit.
func (i *WorkspaceAdoptionInspection) Verify(ctx context.Context) error {
	for path, expected := range i.Proof.Identities {
		identity, ok := recoveryartifact.FilesystemIdentity(path)
		if !ok || identity != expected {
			return models.ErrWorkspaceAdoptionStale
		}
	}
	for path, h := range i.handles {
		identity, ok := recoveryartifact.FilesystemIdentity(path)
		if !ok || identity != i.Proof.Identities[path] {
			return models.ErrWorkspaceAdoptionStale
		}
		if err := h.VerifyPath(path); err != nil {
			return err
		}
	}
	root := i.root
	rootMarker, err := i.handles[root].ReadFile(storageworkspaces.OwnershipMarkerFilename)
	if err != nil {
		return err
	}
	if adoptionHash(rootMarker) != i.Proof.RootMarkerSHA256 {
		return models.ErrWorkspaceAdoptionStale
	}
	marker, err := i.handles[i.Proof.OriginalPath].ReadFile(".git")
	if err != nil {
		return err
	}
	if !bytes.Equal(marker, i.marker) {
		return models.ErrWorkspaceAdoptionStale
	}
	if err := i.verifyRetiredAdministration(); err != nil {
		return err
	}
	return i.verifyGit(ctx)
}

func (i *WorkspaceAdoptionInspection) verifyGit(ctx context.Context) error {
	before := i.Proof
	if err := validateAdoptionMetadata(ctx, i.handles[filepath.Join(i.source, ".git")], filepath.Join(i.source, ".git")); err != nil {
		return err
	}
	// Reinspection owns temporary metadata handles, not this inspection's pins.
	other := &WorkspaceAdoptionInspection{manager: i.manager, Proof: before, source: i.source, sourceHead: i.sourceHead, handles: map[string]storageworkspaces.DirectoryHandle{i.Proof.CandidatePath: i.handles[i.Proof.CandidatePath]}}
	err := other.inspectCandidate(ctx)
	if h := other.handles[filepath.Join(i.Proof.CandidatePath, ".git")]; h != nil {
		_ = h.Close()
	}
	if err != nil {
		return err
	}
	if before.OriginSHA256 != other.Proof.OriginSHA256 || before.Head != other.Proof.Head || before.SourceHead != other.Proof.SourceHead || before.IndexSHA256 != other.Proof.IndexSHA256 {
		return models.ErrWorkspaceAdoptionStale
	}
	return ctx.Err()
}

// ValidateWorkspaceAdoptionProof validates a retained receipt without granting access.
func (m *Manager) ValidateWorkspaceAdoptionProof(ctx context.Context, p models.WorkspaceAdoptionPreview) error {
	inspection, err := m.openWorkspaceAdoptionReceiptProof(ctx, p)
	if err != nil {
		return err
	}
	defer inspection.Close()
	return nil
}

func (m *Manager) openWorkspaceAdoptionReceiptProof(ctx context.Context, p models.WorkspaceAdoptionPreview) (*WorkspaceAdoptionInspection, error) {
	inspection, err := m.inspectWorkspaceAdoption(ctx, p.Selection, p.SlotID, p.Proof.CandidatePath, p.Proof.SourceHead)
	if err != nil {
		return nil, err
	}
	// Staging and HEAD may legitimately change after adoption. Origin, branch,
	// exact marker and directory identities remain authority for the receipt.
	got := inspection.Proof
	want := p.Proof
	got.Head = want.Head
	got.IndexSHA256 = want.IndexSHA256
	if models.WorkspaceAdoptionDigest(got) != models.WorkspaceAdoptionDigest(want) {
		inspection.Close()
		return nil, models.ErrWorkspaceAdoptionStale
	}
	return inspection, nil
}

func (i *WorkspaceAdoptionInspection) pinRoot(selection models.WorkspaceAdoptionSelection) error {
	base, err := i.manager.TasksBasePath()
	if err != nil {
		return err
	}
	root, err := filepath.Abs(filepath.Join(base, selection.Snapshot.TaskDirName))
	if err != nil {
		return err
	}
	i.root = root
	rootHandle, err := storageworkspaces.OpenDirectoryNoFollow(base, root)
	if err != nil {
		return err
	}
	i.handles[root] = rootHandle
	rootMarker, err := rootHandle.ReadFile(storageworkspaces.OwnershipMarkerFilename)
	if err != nil {
		return err
	}
	var owner storageworkspaces.OwnershipMarker
	if err := json.Unmarshal(rootMarker, &owner); err != nil {
		return err
	}
	if owner.TaskID != selection.Snapshot.EnvironmentOwnerTaskID || owner.TaskDirName != selection.Snapshot.TaskDirName || owner.WorkspaceID != selection.WorkspaceID {
		return models.ErrWorkspaceAdoptionStale
	}
	i.Proof.RootMarkerSHA256 = adoptionHash(rootMarker)

	return nil
}

func (i *WorkspaceAdoptionInspection) inspectCandidateHistory(ctx context.Context, path string) error {
	sourceHead := i.sourceHead
	if sourceHead == "" {
		head, err := i.rawGitInspect(ctx, i.source, "rev-parse", "--verify", i.Proof.Branch+"^{commit}")
		if err != nil {
			return err
		}
		sourceHead = strings.TrimSpace(head)
	}
	if !relocationCommitPattern.MatchString(sourceHead) {
		return models.ErrWorkspaceAdoptionStale
	}
	if _, err := i.rawGitInspect(ctx, path, "merge-base", "--is-ancestor", sourceHead, "HEAD"); err != nil {
		return models.ErrWorkspaceAdoptionStale
	}
	i.Proof.SourceHead = sourceHead
	return nil
}

func validWorkspaceAdoptionPaths(selection models.WorkspaceAdoptionSelection, slot models.WorkspaceRecoveryInventorySlot, candidate, base string) bool {
	root := filepath.Join(base, selection.Snapshot.TaskDirName)
	cwd := selection.Snapshot.WorkspacePath
	return selection.Snapshot.TaskDirName != "" && filepath.Dir(root) == base && (cwd == root || cwd == slot.WorktreePath) && filepath.IsAbs(candidate) && filepath.Clean(candidate) == candidate && candidate != slot.WorktreePath && filepath.Dir(candidate) == root && filepath.Dir(slot.WorktreePath) == root
}

// All adoption identity/history reads use the actual objects, not refs/replace.
func (i *WorkspaceAdoptionInspection) rawGitInspect(ctx context.Context, path string, args ...string) (string, error) {
	explicit := []string{"--no-replace-objects", "--git-dir=" + filepath.Join(path, ".git")}
	return i.manager.runBoundedGitInspectWithEnvironment(ctx, path, mainCheckoutInspectionEnvironment, append(explicit, args...)...)
}
