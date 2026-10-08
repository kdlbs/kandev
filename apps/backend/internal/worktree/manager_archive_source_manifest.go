package worktree

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	storageworkspaces "github.com/kandev/kandev/internal/system/storage/workspaces"
)

// ArchiveSourceManifest is the content-identity record captured before an
// archived worktree is removed. It deliberately stores digests, never file
// contents, so durable cleanup evidence cannot retain source secrets.
type ArchiveSourceManifest struct {
	TaskID            string                       `json:"task_id"`
	TaskEnvironmentID string                       `json:"task_environment_id,omitempty"`
	WorktreeID        string                       `json:"worktree_id"`
	RepositoryID      string                       `json:"repository_id"`
	HeadOID           string                       `json:"head_oid"`
	IndexStateSHA256  string                       `json:"index_state_sha256"`
	PathPresent       bool                         `json:"path_present"`
	Entries           []ArchiveSourceManifestEntry `json:"entries,omitempty"`
}

var errArchiveSourceManifestHandleClose = errors.New("archive source manifest handle close failed")

// ArchiveSourceManifestEntry identifies one changed path without retaining its
// source bytes. A missing digest without an omission marker represents deletion.
type ArchiveSourceManifestEntry struct {
	Path            string `json:"path"`
	Status          string `json:"status"`
	ContentSHA256   string `json:"content_sha256,omitempty"`
	ContentOmission string `json:"content_omission,omitempty"`
}

// CaptureArchiveSourceManifests reads the git state of exact registered
// worktrees before archive cleanup. Any uncertain ownership, git metadata, or
// filesystem read fails the caller closed.
func (m *Manager) CaptureArchiveSourceManifests(
	ctx context.Context, worktrees []*Worktree,
) (map[string]ArchiveSourceManifest, error) {
	manifests := make(map[string]ArchiveSourceManifest, len(worktrees))
	for _, wt := range worktrees {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if wt == nil {
			continue
		}
		if wt.ID == "" || wt.TaskID == "" || wt.RepositoryID == "" || wt.Path == "" || wt.RepositoryPath == "" {
			return nil, fmt.Errorf("archive source manifest has incomplete worktree identity")
		}
		pathPresent, err := cleanupPathPresent(wt.Path)
		if err != nil {
			return nil, fmt.Errorf("inspect archive source worktree path for %s: %w", wt.ID, err)
		}
		if !pathPresent {
			manifest, captureErr := m.captureAbsentArchiveSourceManifest(ctx, wt)
			if captureErr != nil {
				return nil, captureErr
			}
			manifests[wt.ID] = manifest
			continue
		}
		manifest, captureErr := m.capturePresentArchiveSourceManifest(ctx, wt)
		if captureErr != nil {
			return nil, captureErr
		}
		manifests[wt.ID] = manifest
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return manifests, nil
}

func (m *Manager) captureAbsentArchiveSourceManifest(ctx context.Context, wt *Worktree) (ArchiveSourceManifest, error) {
	manifest := ArchiveSourceManifest{TaskID: wt.TaskID, TaskEnvironmentID: wt.TaskEnvironmentID, WorktreeID: wt.ID, RepositoryID: wt.RepositoryID}
	branch := strings.TrimSpace(wt.Branch)
	if branch == "" {
		return manifest, nil
	}
	oid, found, err := m.captureCleanupBranchOID(ctx, wt.RepositoryPath, "refs/heads/"+branch)
	if err != nil {
		return ArchiveSourceManifest{}, fmt.Errorf("capture absent archive source branch for %s: %w", wt.ID, err)
	}
	if found {
		manifest.HeadOID = oid
	}
	return manifest, nil
}

func (m *Manager) capturePresentArchiveSourceManifest(ctx context.Context, wt *Worktree) (ArchiveSourceManifest, error) {
	if err := m.validateExistingWorktreePathOwner(wt.Path, wt); err != nil {
		return ArchiveSourceManifest{}, fmt.Errorf("validate archive source manifest owner for %s: %w", wt.ID, err)
	}
	gitDir, err := m.runBoundedGitInspect(ctx, wt.Path, "rev-parse", "--git-dir")
	if err != nil {
		return ArchiveSourceManifest{}, fmt.Errorf("verify archive source worktree registration for %s: %w", wt.ID, err)
	}
	if filepath.Clean(strings.TrimSpace(gitDir)) == ".git" {
		return ArchiveSourceManifest{}, fmt.Errorf("archive source manifest path is a primary repository, not a registered worktree")
	}
	if err := m.validateArchiveWorktreeRegistration(ctx, wt); err != nil {
		return ArchiveSourceManifest{}, fmt.Errorf("validate archive source worktree registration for %s: %w", wt.ID, err)
	}
	head, err := m.runBoundedGitInspect(ctx, wt.Path, "rev-parse", "--verify", "HEAD^{commit}")
	if err != nil {
		return ArchiveSourceManifest{}, fmt.Errorf("capture archive source HEAD for %s: %w", wt.ID, err)
	}
	indexStage, err := m.runBoundedGitInspect(ctx, wt.Path, "ls-files", "--stage", "-z")
	if err != nil {
		return ArchiveSourceManifest{}, fmt.Errorf("capture archive source index for %s: %w", wt.ID, err)
	}
	indexState := sha256.Sum256([]byte(indexStage))
	status, err := m.runBoundedGitInspect(ctx, wt.Path, "status", "--porcelain=v1", "-z", "--untracked-files=all", "--ignored=matching")
	if err != nil {
		return ArchiveSourceManifest{}, fmt.Errorf("capture archive source status for %s: %w", wt.ID, err)
	}
	entries, err := archiveSourceManifestEntries(ctx, wt.Path, status)
	if err != nil {
		return ArchiveSourceManifest{}, fmt.Errorf("capture archive source entries for %s: %w", wt.ID, err)
	}
	return ArchiveSourceManifest{TaskID: wt.TaskID, TaskEnvironmentID: wt.TaskEnvironmentID, WorktreeID: wt.ID, RepositoryID: wt.RepositoryID, HeadOID: strings.TrimSpace(head), IndexStateSHA256: fmt.Sprintf("%x", indexState), PathPresent: true, Entries: entries}, nil
}

func (m *Manager) validateArchiveWorktreeRegistration(ctx context.Context, wt *Worktree) error {
	if err := validateArchiveWorktreeGitDirBackpointer(wt.Path, ctx, m); err != nil {
		return err
	}
	worktreeCommonDir, err := m.runBoundedGitInspect(ctx, wt.Path, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return err
	}
	repositoryCommonDir, err := m.runBoundedGitInspect(ctx, wt.RepositoryPath, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return err
	}
	if filepath.Clean(strings.TrimSpace(worktreeCommonDir)) != filepath.Clean(strings.TrimSpace(repositoryCommonDir)) {
		return fmt.Errorf("worktree belongs to a different repository")
	}
	registered, err := m.runBoundedGitInspect(ctx, wt.RepositoryPath, "worktree", "list", "--porcelain", "-z")
	if err != nil {
		return err
	}
	want, err := filepath.Abs(wt.Path)
	if err != nil {
		return err
	}
	for _, field := range strings.Split(registered, "\x00") {
		if strings.HasPrefix(field, "worktree ") && filepath.Clean(strings.TrimPrefix(field, "worktree ")) == filepath.Clean(want) {
			return nil
		}
	}
	return fmt.Errorf("worktree path is not registered in the recorded repository")
}

func validateArchiveWorktreeGitDirBackpointer(worktreePath string, ctx context.Context, manager *Manager) error {
	gitDir, err := manager.runBoundedGitInspect(ctx, worktreePath, "rev-parse", "--path-format=absolute", "--git-dir")
	if err != nil {
		return err
	}
	gitDir = filepath.Clean(strings.TrimSpace(gitDir))
	worktreeRoot, err := openArchiveSourceDirectory(filepath.Dir(worktreePath), worktreePath)
	if err != nil {
		return fmt.Errorf("open registered worktree: %w", err)
	}
	marker, err := readArchiveSourcePointerFile(worktreeRoot, ".git")
	closeErr := worktreeRoot.Close()
	if err != nil {
		return fmt.Errorf("read worktree gitdir pointer: %w", err)
	}
	if closeErr != nil {
		return fmt.Errorf("close worktree gitdir pointer: %w", closeErr)
	}
	pointedGitDir, ok := archiveSourceGitDirPointer(string(marker), worktreePath)
	if !ok || pointedGitDir != gitDir {
		return fmt.Errorf("worktree gitdir pointer does not match git metadata")
	}

	gitMetadata, err := openArchiveSourceDirectory(filepath.Dir(gitDir), gitDir)
	if err != nil {
		return fmt.Errorf("open worktree git metadata: %w", err)
	}
	defer func() { _ = gitMetadata.Close() }()
	backpointer, err := readArchiveSourcePointerFile(gitMetadata, "gitdir")
	if err != nil {
		return fmt.Errorf("read worktree git metadata backpointer: %w", err)
	}
	pointedWorktreeGitFile, ok := archiveSourceResolvePath(string(backpointer), filepath.Dir(gitDir))
	wantWorktreeGitFile, absErr := filepath.Abs(filepath.Join(worktreePath, ".git"))
	if absErr != nil {
		return absErr
	}
	if !ok || pointedWorktreeGitFile != filepath.Clean(wantWorktreeGitFile) {
		return fmt.Errorf("worktree git metadata belongs to a different worktree path")
	}
	return nil
}

func readArchiveSourcePointerFile(directory storageworkspaces.DirectoryHandle, name string) ([]byte, error) {
	mode, err := directory.LstatEntry(name)
	if err != nil {
		return nil, err
	}
	if !mode.IsRegular() {
		return nil, fmt.Errorf("%s is not a regular file", name)
	}
	return directory.ReadFile(name)
}

func archiveSourceGitDirPointer(content, relativeTo string) (string, bool) {
	content = strings.TrimSpace(content)
	if !strings.HasPrefix(content, "gitdir: ") {
		return "", false
	}
	pointer := strings.TrimSpace(strings.TrimPrefix(content, "gitdir: "))
	if pointer == "" {
		return "", false
	}
	return archiveSourceResolvePath(pointer, relativeTo)
}

func archiveSourceResolvePath(pointer, relativeTo string) (string, bool) {
	pointer = strings.TrimSpace(pointer)
	if pointer == "" {
		return "", false
	}
	if !filepath.IsAbs(pointer) {
		pointer = filepath.Join(relativeTo, pointer)
	}
	abs, err := filepath.Abs(pointer)
	if err != nil {
		return "", false
	}
	return filepath.Clean(abs), true
}

func archiveSourceManifestEntries(ctx context.Context, root, output string) ([]ArchiveSourceManifestEntry, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	fields := strings.Split(output, "\x00")
	entries := make([]ArchiveSourceManifestEntry, 0, len(fields))
	for index := 0; index < len(fields); index++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if fields[index] == "" {
			continue
		}
		entry, consumed, err := archiveSourceManifestEntry(ctx, root, fields, index)
		if err != nil {
			return nil, err
		}
		entries = append(entries, entry)
		index += consumed
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return entries, nil
}

func archiveSourceManifestEntry(ctx context.Context, root string, fields []string, index int) (ArchiveSourceManifestEntry, int, error) {
	if err := ctx.Err(); err != nil {
		return ArchiveSourceManifestEntry{}, 0, err
	}
	path, status, consumed, err := archiveSourceManifestStatusPath(fields, index)
	if err != nil {
		return ArchiveSourceManifestEntry{}, 0, err
	}
	if err := archiveSourceManifestPath(root, path); err != nil {
		return ArchiveSourceManifestEntry{}, 0, err
	}
	entry := ArchiveSourceManifestEntry{Path: path, Status: status}
	digest, omitted, err := archiveSourceManifestDigestForEntry(ctx, root, path, status)
	if err != nil {
		if archiveSourceManifestIsDeletedPath(status, err) {
			return entry, consumed, nil
		}
		if archiveSourceManifestIsMissingPathError(err) {
			return ArchiveSourceManifestEntry{}, 0, fmt.Errorf("source path disappeared before content identity was captured")
		}
		return ArchiveSourceManifestEntry{}, 0, err
	}
	if omitted {
		entry.ContentOmission = "ignored_directory"
		return entry, consumed, nil
	}
	if err := ctx.Err(); err != nil {
		return ArchiveSourceManifestEntry{}, 0, err
	}
	entry.ContentSHA256 = digest
	return entry, consumed, nil
}

func archiveSourceManifestStatusPath(fields []string, index int) (string, string, int, error) {
	field := fields[index]
	if len(field) < 4 || field[2] != ' ' {
		return "", "", 0, fmt.Errorf("invalid git status record")
	}
	path := field[3:]
	consumed := 0
	if field[0] == 'R' || field[0] == 'C' || field[1] == 'R' || field[1] == 'C' {
		if index+1 >= len(fields) || fields[index+1] == "" {
			return "", "", 0, fmt.Errorf("git rename or copy record lacks its original path")
		}
		consumed = 1
	}
	return path, field[:2], consumed, nil
}

func archiveSourceManifestIsDeletedPath(status string, err error) bool {
	return len(status) == 2 && (status[0] == 'D' || status[1] == 'D') && archiveSourceManifestIsMissingPathError(err)
}

func archiveSourceManifestIsMissingPathError(err error) bool {
	return errors.Is(err, os.ErrNotExist) && !errors.Is(err, errArchiveSourceManifestHandleClose)
}

func archiveSourceManifestCloseError(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%w: %w", errArchiveSourceManifestHandleClose, err)
}

func archiveSourceManifestPath(root, path string) error {
	if path == "" || filepath.IsAbs(path) {
		return fmt.Errorf("invalid source path")
	}
	rel, err := filepath.Rel(root, filepath.Join(root, path))
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("source path escapes worktree")
	}
	return nil
}

func archiveSourceManifestDigestForEntry(ctx context.Context, root, path, status string) (string, bool, error) {
	parent, name, err := archiveSourceManifestOpenParent(ctx, root, path)
	if err != nil {
		return "", false, err
	}
	digest, omitted, captureErr := archiveSourceManifestDigestEntryAt(ctx, parent, name, status)
	closeErr := archiveSourceManifestCloseError(parent.Close())
	return digest, omitted, errors.Join(captureErr, closeErr)
}

func archiveSourceManifestDigestEntryAt(
	ctx context.Context,
	parent storageworkspaces.DirectoryHandle,
	name, status string,
) (string, bool, error) {
	if err := ctx.Err(); err != nil {
		return "", false, err
	}
	mode, err := parent.LstatEntry(name)
	if err != nil {
		return "", false, err
	}
	if err := ctx.Err(); err != nil {
		return "", false, err
	}
	if status == "!!" && mode.IsDir() {
		return "", true, nil
	}
	if mode&os.ModeSymlink != 0 {
		target, readErr := parent.ReadLink(name)
		if readErr != nil {
			return "", false, readErr
		}
		if err := ctx.Err(); err != nil {
			return "", false, err
		}
		sum := sha256.Sum256([]byte("symlink:\x00" + target))
		return fmt.Sprintf("%x", sum), false, nil
	}
	if !mode.IsRegular() {
		if !mode.IsDir() {
			return "", false, fmt.Errorf("source path is not a regular file or directory")
		}
		directory, err := parent.OpenSubdirectory(name)
		if err != nil {
			return "", false, err
		}
		digest, digestErr := archiveSourceManifestDirectoryDigestHandle(ctx, directory)
		closeErr := archiveSourceManifestCloseError(directory.Close())
		return digest, false, errors.Join(digestErr, closeErr)
	}
	f, err := parent.OpenFile(name)
	if err != nil {
		return "", false, err
	}
	digest, digestErr := archiveSourceManifestReadDigest(ctx, f)
	return digest, false, digestErr
}

type archiveSourceManifestDirectoryRecord struct {
	path   string
	kind   string
	digest string
	target string
}

const (
	archiveSourceManifestKindSymlink   = "symlink"
	archiveSourceManifestKindDirectory = "directory"
	archiveSourceManifestKindFile      = "file"
)

func archiveSourceManifestDirectoryDigestHandle(ctx context.Context, root storageworkspaces.DirectoryHandle) (string, error) {
	records := make([]archiveSourceManifestDirectoryRecord, 0)
	if err := archiveSourceManifestCollectDirectory(ctx, root, "", &records); err != nil {
		return "", err
	}
	sort.Slice(records, func(i, j int) bool { return records[i].path < records[j].path })
	h := sha256.New()
	for _, record := range records {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		_, _ = io.WriteString(h, record.path+"\x00")
		switch record.kind {
		case archiveSourceManifestKindSymlink:
			_, _ = io.WriteString(h, "symlink:\x00"+record.target+"\x00")
		case archiveSourceManifestKindDirectory:
			_, _ = io.WriteString(h, "directory\x00")
		case archiveSourceManifestKindFile:
			_, _ = io.WriteString(h, "file:\x00"+record.digest+"\x00")
		}
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", h.Sum(nil)), nil
}

func archiveSourceManifestCollectDirectory(
	ctx context.Context,
	directory storageworkspaces.DirectoryHandle,
	prefix string,
	records *[]archiveSourceManifestDirectoryRecord,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	entries, err := directory.ReadDir()
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return err
		}
		name := entry.Name()
		if name == ".git" {
			continue
		}
		mode, err := directory.LstatEntry(name)
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		path := name
		if prefix != "" {
			path = filepath.Join(prefix, name)
		}
		if err := archiveSourceManifestCollectEntry(ctx, directory, name, path, mode, records); err != nil {
			return err
		}
	}
	return nil
}

func archiveSourceManifestCollectEntry(
	ctx context.Context,
	directory storageworkspaces.DirectoryHandle,
	name, path string,
	mode os.FileMode,
	records *[]archiveSourceManifestDirectoryRecord,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	switch {
	case mode&os.ModeSymlink != 0:
		target, err := directory.ReadLink(name)
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		*records = append(*records, archiveSourceManifestDirectoryRecord{
			path: path, kind: archiveSourceManifestKindSymlink, target: target,
		})
	case mode.IsDir():
		return archiveSourceManifestCollectSubdirectory(ctx, directory, name, path, records)
	case mode.IsRegular():
		return archiveSourceManifestCollectFile(ctx, directory, name, path, records)
	default:
		return fmt.Errorf("directory source path is not a regular file, directory, or symlink")
	}
	return nil
}

func archiveSourceManifestCollectSubdirectory(
	ctx context.Context,
	directory storageworkspaces.DirectoryHandle,
	name, path string,
	records *[]archiveSourceManifestDirectoryRecord,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	*records = append(*records, archiveSourceManifestDirectoryRecord{
		path: path, kind: archiveSourceManifestKindDirectory,
	})
	child, err := directory.OpenSubdirectory(name)
	if err != nil {
		return err
	}
	collectErr := archiveSourceManifestCollectDirectory(ctx, child, path, records)
	closeErr := archiveSourceManifestCloseError(child.Close())
	return errors.Join(collectErr, closeErr)
}

func archiveSourceManifestCollectFile(
	ctx context.Context,
	directory storageworkspaces.DirectoryHandle,
	name, path string,
	records *[]archiveSourceManifestDirectoryRecord,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	file, err := directory.OpenFile(name)
	if err != nil {
		return err
	}
	digest, err := archiveSourceManifestReadDigest(ctx, file)
	if err != nil {
		return err
	}
	*records = append(*records, archiveSourceManifestDirectoryRecord{
		path: path, kind: archiveSourceManifestKindFile, digest: digest,
	})
	return nil
}

func archiveSourceManifestOpenParent(
	ctx context.Context,
	root, path string,
) (storageworkspaces.DirectoryHandle, string, error) {
	if err := ctx.Err(); err != nil {
		return nil, "", err
	}
	if err := archiveSourceManifestPath(root, path); err != nil {
		return nil, "", err
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		return nil, "", err
	}
	directory, err := openArchiveSourceDirectory(filepath.Dir(absolute), absolute)
	if err != nil {
		return nil, "", err
	}
	components := strings.Split(filepath.ToSlash(filepath.Clean(path)), "/")
	for _, component := range components[:len(components)-1] {
		if err := ctx.Err(); err != nil {
			return nil, "", errors.Join(err, archiveSourceManifestCloseError(directory.Close()))
		}
		child, err := directory.OpenSubdirectory(component)
		if err != nil {
			return nil, "", errors.Join(err, archiveSourceManifestCloseError(directory.Close()))
		}
		if closeErr := archiveSourceManifestCloseError(directory.Close()); closeErr != nil {
			return nil, "", errors.Join(closeErr, archiveSourceManifestCloseError(child.Close()))
		}
		directory = child
	}
	if err := ctx.Err(); err != nil {
		return nil, "", errors.Join(err, archiveSourceManifestCloseError(directory.Close()))
	}
	return directory, components[len(components)-1], nil
}

func archiveSourceManifestFileDigest(ctx context.Context, root, path string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	rootPath, err := filepath.Rel(root, filepath.Dir(path))
	if err != nil {
		return "", err
	}
	parent, err := openArchiveSourceDirectory(filepath.Dir(root), filepath.Join(root, rootPath))
	if err != nil {
		return "", err
	}
	mode, err := parent.LstatEntry(filepath.Base(path))
	if err != nil {
		return "", errors.Join(err, archiveSourceManifestCloseError(parent.Close()))
	}
	if !mode.IsRegular() {
		return "", errors.Join(fmt.Errorf("source path is not a regular file"), archiveSourceManifestCloseError(parent.Close()))
	}
	f, err := parent.OpenFile(filepath.Base(path))
	if err != nil {
		return "", errors.Join(err, archiveSourceManifestCloseError(parent.Close()))
	}
	digest, digestErr := archiveSourceManifestReadDigest(ctx, f)
	return digest, errors.Join(digestErr, archiveSourceManifestCloseError(parent.Close()))
}

func openArchiveSourceDirectory(root, target string) (storageworkspaces.DirectoryHandle, error) {
	return storageworkspaces.OpenDirectoryNoFollow(root, target)
}

func archiveSourceManifestReadDigest(ctx context.Context, f io.ReadCloser) (string, error) {
	h := sha256.New()
	buffer := make([]byte, 32*1024)
	var readErr error
	for {
		if err := ctx.Err(); err != nil {
			readErr = err
			break
		}
		n, err := f.Read(buffer)
		if n > 0 {
			_, _ = h.Write(buffer[:n])
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			readErr = err
			break
		}
		if ctxErr := ctx.Err(); ctxErr != nil {
			readErr = ctxErr
			break
		}
	}
	closeErr := archiveSourceManifestCloseError(f.Close())
	readErr = errors.Join(readErr, ctx.Err())
	if readErr != nil {
		return "", errors.Join(readErr, closeErr)
	}
	if closeErr != nil {
		return "", closeErr
	}
	return fmt.Sprintf("%x", h.Sum(nil)), nil
}
