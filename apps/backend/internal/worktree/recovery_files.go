package worktree

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func snapshotCheckout(source, destination string) error {
	if err := os.MkdirAll(destination, 0700); err != nil {
		return fmt.Errorf("create recovery snapshot: %w", err)
	}
	var directoryModes []recoveryDirectoryMode
	err := filepath.WalkDir(source, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		directoryMode, copyErr := snapshotCheckoutEntry(source, destination, path, entry)
		if directoryMode != nil {
			directoryModes = append(directoryModes, *directoryMode)
		}
		return copyErr
	})
	if err != nil {
		return err
	}
	return applyRecoveryDirectoryModes(directoryModes)
}

func snapshotCheckoutEntry(source, destination, path string, entry os.DirEntry) (*recoveryDirectoryMode, error) {
	rel, err := filepath.Rel(source, path)
	if err != nil || rel == "." {
		return nil, err
	}
	if rel == recoveryGitDirName || strings.HasPrefix(rel, recoveryGitDirName+string(filepath.Separator)) {
		if entry.IsDir() {
			return nil, filepath.SkipDir
		}
		return nil, nil
	}
	info, err := entry.Info()
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeNamedPipe != 0 || info.Mode()&os.ModeSocket != 0 || info.Mode()&os.ModeDevice != 0 {
		return nil, fmt.Errorf("unsupported recovery entry %q", rel)
	}
	target := filepath.Join(destination, rel)
	if entry.IsDir() {
		if err := os.MkdirAll(target, 0700); err != nil {
			return nil, err
		}
		return &recoveryDirectoryMode{path: target, mode: info.Mode().Perm()}, nil
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, snapshotSymlink(path, target)
	}
	return nil, snapshotRegularFile(path, target, info.Mode().Perm())
}

func snapshotSymlink(source, target string) error {
	link, err := os.Readlink(source)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
		return err
	}
	return os.Symlink(link, target)
}

func snapshotRegularFile(source, target string, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
		return err
	}
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	out, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
	if err != nil {
		_ = in.Close()
		return err
	}
	_, copyErr := io.Copy(out, in)
	closeInputErr := in.Close()
	syncErr := out.Sync()
	closeOutputErr := out.Close()
	for _, result := range []error{copyErr, closeInputErr, syncErr, closeOutputErr} {
		if result != nil {
			return result
		}
	}
	return nil
}

//nolint:cyclop // The manifest must record every supported filesystem entry explicitly.
func checkoutManifest(root string) (string, error) {
	var entries []string
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == root {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if rel == recoveryGitDirName || strings.HasPrefix(rel, recoveryGitDirName+string(filepath.Separator)) {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		entryValue := rel + "|" + info.Mode().String()
		if entry.IsDir() {
			entries = append(entries, entryValue)
			return nil
		}
		if !info.Mode().IsRegular() && info.Mode()&os.ModeSymlink == 0 {
			return fmt.Errorf("unsupported recovery entry %q", rel)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			target, err := os.Readlink(path)
			if err != nil {
				return err
			}
			entryValue += "|" + target
		} else {
			file, err := os.Open(path)
			if err != nil {
				return err
			}
			hash := sha256.New()
			_, copyErr := io.Copy(hash, file)
			_ = file.Close()
			if copyErr != nil {
				return copyErr
			}
			entryValue += "|" + hex.EncodeToString(hash.Sum(nil))
		}
		entries = append(entries, entryValue)
		return nil
	})
	if err != nil {
		return "", err
	}
	sort.Strings(entries)
	hash := sha256.Sum256([]byte(strings.Join(entries, "\n")))
	return hex.EncodeToString(hash[:]), nil
}

func restoreSnapshot(source, destination, expectedManifest string) error {
	current, err := checkoutManifest(source)
	if err != nil {
		return fmt.Errorf("manifest recovery snapshot before rematerialization: %w", err)
	}
	if current != expectedManifest {
		return fmt.Errorf("recovery snapshot changed during rematerialization")
	}
	if err := copySnapshotEntries(source, destination); err != nil {
		return err
	}
	current, err = checkoutManifest(destination)
	if err != nil {
		return fmt.Errorf("manifest recovery replacement: %w", err)
	}
	if current != expectedManifest {
		return fmt.Errorf("recovery replacement does not match verified snapshot")
	}
	current, err = checkoutManifest(source)
	if err != nil {
		return fmt.Errorf("manifest recovery snapshot after restoration: %w", err)
	}
	if current != expectedManifest {
		return fmt.Errorf("recovery snapshot changed during restoration")
	}
	return nil
}

//nolint:cyclop // The copy walk handles each filesystem type explicitly.
func copySnapshotEntries(source, destination string) error {
	if err := removeDestinationTypeMismatches(source, destination); err != nil {
		return err
	}
	if err := filepath.WalkDir(destination, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == destination || entry.IsDir() || entry.Name() == recoveryGitDirName {
			return nil
		}
		rel, err := filepath.Rel(destination, path)
		if err != nil {
			return err
		}
		if _, statErr := os.Lstat(filepath.Join(source, rel)); os.IsNotExist(statErr) {
			return os.Remove(path)
		}
		return nil
	}); err != nil {
		return err
	}
	if err := removeDestinationDirectoriesAbsentFromSnapshot(source, destination); err != nil {
		return err
	}
	var directoryModes []recoveryDirectoryMode
	err := filepath.WalkDir(source, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path != source && entry.IsDir() {
			info, infoErr := entry.Info()
			if infoErr != nil {
				return infoErr
			}
			rel, relErr := filepath.Rel(source, path)
			if relErr != nil {
				return relErr
			}
			directoryModes = append(directoryModes, recoveryDirectoryMode{
				path: filepath.Join(destination, rel), mode: info.Mode().Perm(),
			})
		}
		return copySnapshotEntry(source, destination, path, entry)
	})
	if err != nil {
		return err
	}
	return applyRecoveryDirectoryModes(directoryModes)
}

type recoveryDirectoryMode struct {
	path string
	mode os.FileMode
}

func applyRecoveryDirectoryModes(directories []recoveryDirectoryMode) error {
	for i := len(directories) - 1; i >= 0; i-- {
		if err := os.Chmod(directories[i].path, directories[i].mode); err != nil {
			return err
		}
	}
	return nil
}

type recoveryEntryType uint8

const (
	recoveryEntryDirectory recoveryEntryType = iota
	recoveryEntryFile
	recoveryEntrySymlink
)

// removeDestinationTypeMismatches clears only entries which the verified
// snapshot proves must change type. This runs before the absence cleanup so a
// fresh tracked directory cannot make a snapshot file appear as an invalid
// descendant, and no copy operation can follow a replaced symlink.
func removeDestinationTypeMismatches(source, destination string) error {
	return filepath.WalkDir(source, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == source {
			return nil
		}
		rel, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		if rel == recoveryGitDirName || strings.HasPrefix(rel, recoveryGitDirName+string(filepath.Separator)) {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		sourceType, err := recoveryTypeForDirEntry(entry)
		if err != nil {
			return fmt.Errorf("inspect recovery snapshot entry %q: %w", rel, err)
		}
		target := filepath.Join(destination, rel)
		existing, err := os.Lstat(target)
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			return err
		}
		destinationType, err := recoveryTypeForFileInfo(existing)
		if err != nil {
			return fmt.Errorf("unsupported recovery destination entry %q: %w", target, err)
		}
		if sourceType == destinationType {
			return nil
		}
		if err := os.RemoveAll(target); err != nil {
			return fmt.Errorf("replace recovery destination entry %q: %w", target, err)
		}
		return nil
	})
}

func recoveryTypeForDirEntry(entry os.DirEntry) (recoveryEntryType, error) {
	if entry.IsDir() {
		return recoveryEntryDirectory, nil
	}
	if entry.Type()&os.ModeSymlink != 0 {
		return recoveryEntrySymlink, nil
	}
	info, err := entry.Info()
	if err != nil {
		return 0, err
	}
	return recoveryTypeForFileInfo(info)
}

func recoveryTypeForFileInfo(info os.FileInfo) (recoveryEntryType, error) {
	if info.IsDir() {
		return recoveryEntryDirectory, nil
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return recoveryEntrySymlink, nil
	}
	if info.Mode().IsRegular() {
		return recoveryEntryFile, nil
	}
	return 0, fmt.Errorf("entry mode %s", info.Mode())
}

func removeDestinationDirectoriesAbsentFromSnapshot(source, destination string) error {
	var directories []string
	if err := filepath.WalkDir(destination, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == destination || !entry.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(destination, path)
		if err != nil {
			return err
		}
		if rel == recoveryGitDirName {
			return filepath.SkipDir
		}
		if _, err := os.Lstat(filepath.Join(source, rel)); os.IsNotExist(err) {
			directories = append(directories, path)
			return nil
		} else if err != nil {
			return err
		}
		return nil
	}); err != nil {
		return err
	}
	sort.Slice(directories, func(i, j int) bool {
		return len(directories[i]) > len(directories[j])
	})
	for _, directory := range directories {
		if err := os.Remove(directory); err != nil {
			return err
		}
	}
	return nil
}

func copySnapshotEntry(source, destination, path string, entry os.DirEntry) error {
	if path == source {
		return nil
	}
	rel, err := filepath.Rel(source, path)
	if err != nil {
		return err
	}
	target := filepath.Join(destination, rel)
	entryType, err := recoveryTypeForDirEntry(entry)
	if err != nil {
		return err
	}
	if entry.IsDir() {
		return copySnapshotDirectory(entry, target)
	}
	if entry.Type()&os.ModeSymlink != 0 {
		return copySnapshotSymlink(path, target)
	}
	if entryType != recoveryEntryFile {
		return fmt.Errorf("unsupported recovery snapshot entry %q", rel)
	}
	return copySnapshotFile(path, target, entry)
}

func copySnapshotSymlink(source, target string) error {
	link, err := os.Readlink(source)
	if err != nil {
		return err
	}
	if err := removeExistingRecoverySymlink(target); err != nil {
		return err
	}
	return os.Symlink(link, target)
}

func removeExistingRecoverySymlink(target string) error {
	existing, err := os.Lstat(target)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if existing.Mode()&os.ModeSymlink == 0 {
		return fmt.Errorf("recovery destination type changed before symlink restore: %q", target)
	}
	return os.Remove(target)
}

func copySnapshotDirectory(entry os.DirEntry, target string) error {
	if existing, err := os.Lstat(target); err == nil && !existing.IsDir() {
		return fmt.Errorf("recovery destination type changed before directory restore: %q", target)
	} else if err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := os.MkdirAll(target, 0700); err != nil {
		return err
	}
	return os.Chmod(target, 0700)
}

func copySnapshotFile(source, target string, entry os.DirEntry) error {
	info, err := entry.Info()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
		return err
	}
	if existing, err := os.Lstat(target); err == nil {
		if !existing.Mode().IsRegular() {
			return fmt.Errorf("recovery destination type changed before file restore: %q", target)
		}
		if err := os.Remove(target); err != nil {
			return err
		}
	} else if err != nil && !os.IsNotExist(err) {
		return err
	}
	file, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, info.Mode().Perm())
	if err != nil {
		return err
	}
	in, err := os.Open(source)
	if err != nil {
		_ = file.Close()
		return err
	}
	_, copyErr := io.Copy(file, in)
	closeInputErr := in.Close()
	if copyErr != nil {
		_ = file.Close()
		return copyErr
	}
	if closeInputErr != nil {
		_ = file.Close()
		return closeInputErr
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Chmod(target, info.Mode().Perm())
}
