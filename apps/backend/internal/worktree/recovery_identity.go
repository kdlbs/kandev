package worktree

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"

	"github.com/kandev/kandev/internal/system/storage/workspaces"
)

func verifyRecoveryRequiredIdentityTrees(original, snapshot, replacement string) error {
	paths := []string{original, snapshot}
	if replacement != "" {
		paths = append(paths, replacement)
	}
	before := make([]string, len(paths))
	for index, path := range paths {
		manifest, err := recoveryRequiredIdentityManifest(path)
		if err != nil {
			return fmt.Errorf("inspect recovery set-ID identity: %w", err)
		}
		before[index] = manifest
	}
	if before[0] != before[1] || len(before) == 3 && before[1] != before[2] {
		return errors.New("recovery set-ID identity does not match the original checkout")
	}
	for index, path := range paths {
		after, err := recoveryRequiredIdentityManifest(path)
		if err != nil || after != before[index] {
			return errors.New("recovery set-ID identity changed during verification")
		}
	}
	return nil
}

func recoveryRequiredIdentityManifest(root string) (string, error) {
	root = filepath.Clean(root)
	handle, err := workspaces.OpenDirectoryNoFollow(filepath.Dir(root), root)
	if err != nil {
		return "", err
	}
	defer func() { _ = handle.Close() }()
	var entries []string
	if err := appendRecoveryRequiredIdentityEntries(handle, root, "", true, &entries); err != nil {
		return "", err
	}
	if err := handle.VerifyPath(root); err != nil {
		return "", err
	}
	return recoveryEntriesManifest(entries), nil
}

func appendRecoveryRequiredIdentityEntries(
	handle workspaces.DirectoryHandle,
	root, relative string,
	isRoot bool,
	entries *[]string,
) error {
	children, err := handle.ReadDir()
	if err != nil {
		return err
	}
	names := make([]string, 0, len(children))
	for _, child := range children {
		if isRoot && child.Name() == recoveryGitDirName {
			continue
		}
		names = append(names, child.Name())
	}
	sort.Strings(names)
	for _, name := range names {
		mode, err := handle.LstatEntry(name)
		if err != nil {
			return err
		}
		entryPath := filepath.Join(relative, name)
		switch {
		case mode.IsDir():
			if err := appendRecoveryDirectoryIdentity(handle, root, entryPath, name, entries); err != nil {
				return err
			}
		case mode&os.ModeSymlink != 0:
			continue
		case mode.Type() == 0:
			if mode&recoverySpecialMode == 0 {
				continue
			}
			if err := appendRecoveryFileIdentity(handle, name, entryPath, mode, entries); err != nil {
				return err
			}
		default:
			return fmt.Errorf("unsupported recovery identity entry %q", entryPath)
		}
	}
	return nil
}

func appendRecoveryDirectoryIdentity(
	parent workspaces.DirectoryHandle,
	root, relative, name string,
	entries *[]string,
) error {
	child, err := parent.OpenSubdirectory(name)
	if err != nil {
		return err
	}
	defer func() { _ = child.Close() }()
	path := filepath.Join(root, relative)
	if err := child.VerifyPath(path); err != nil {
		return err
	}
	before, err := workspaces.PinnedDirectoryInfo(child)
	if err != nil {
		return err
	}
	if err := appendRecoveryIdentityEntry(relative, before, entries); err != nil {
		return err
	}
	if err := appendRecoveryRequiredIdentityEntries(child, root, relative, false, entries); err != nil {
		return err
	}
	after, err := workspaces.PinnedDirectoryInfo(child)
	if err != nil {
		return err
	}
	if err := verifyRecoverySourceInfo(before, after); err != nil {
		return err
	}
	return child.VerifyPath(path)
}

func appendRecoveryFileIdentity(
	parent workspaces.DirectoryHandle,
	name, relative string,
	mode os.FileMode,
	entries *[]string,
) error {
	reader, err := parent.OpenFile(name)
	if err != nil {
		return err
	}
	file, ok := reader.(*os.File)
	if !ok {
		_ = reader.Close()
		return errors.New("pinned recovery identity file has no file descriptor")
	}
	before, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return err
	}
	if before.Mode() != mode {
		_ = file.Close()
		return fmt.Errorf("recovery identity file changed during inspection: %s", relative)
	}
	if err := appendRecoveryIdentityEntry(relative, before, entries); err != nil {
		_ = file.Close()
		return err
	}
	after, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return err
	}
	if err := verifyRecoverySourceInfo(before, after); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}

func appendRecoveryIdentityEntry(relative string, info os.FileInfo, entries *[]string) error {
	uid, gid, requireUID, requireGID, err := recoveryRequiredIdentity(info)
	if err != nil {
		return err
	}
	if !requireUID && !requireGID {
		return nil
	}
	uidValue, gidValue := "-", "-"
	if requireUID {
		uidValue = strconv.Itoa(uid)
	}
	if requireGID {
		gidValue = strconv.Itoa(gid)
	}
	*entries = append(*entries, fmt.Sprintf("%q|uid=%s|gid=%s", relative, uidValue, gidValue))
	return nil
}
