package passthroughconfig

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"os"
	"path/filepath"
	"reflect"
	"strings"
)

const registryPath = ".kandev/passthrough-claims.json"

type registryEntry struct {
	Fingerprint string          `json:"fingerprint"`
	Owned       bool            `json:"owned"`
	Owners      map[string]bool `json:"owners"`
}

type registry map[string]*registryEntry

func Prepare(ctx context.Context, workspace, privateRoot, owner string, files []File) error {
	var changes []remoteFileChange
	return withRegistry(ctx, workspace, func(root *os.Root, entries registry) error {
		for _, file := range files {
			relative, err := relativeConfigPath(workspace, privateRoot, file)
			if err != nil {
				return err
			}
			change, err := captureRemoteFile(root, relative)
			if err != nil {
				return err
			}
			if err := materializeRemoteFile(root, relative, owner, file, entries, &change); err != nil {
				return err
			}
			changes = append(changes, change)
		}
		return nil
	}, func(root *os.Root) error { return rollbackRemoteFiles(root, changes) })
}

func Cleanup(ctx context.Context, workspace, owner string, prefix bool) error {
	if _, err := os.Lstat(filepath.Join(workspace, registryPath)); os.IsNotExist(err) {
		return nil
	}
	return withRegistry(ctx, workspace, func(root *os.Root, entries registry) error {
		for relative, entry := range entries {
			for claim := range entry.Owners {
				if claim == owner || (prefix && strings.HasPrefix(claim, owner)) {
					delete(entry.Owners, claim)
				}
			}
			if len(entry.Owners) > 0 {
				continue
			}
			if err := removeRemoteClaim(root, relative, entry); err != nil {
				return err
			}
			delete(entries, relative)
		}
		return nil
	}, nil)
}

func withRegistry(ctx context.Context, workspace string, operation func(*os.Root, registry) error, rollback func(*os.Root) error) (result error) {
	lock, err := Acquire(ctx, filepath.Join(workspace, registryPath))
	if err != nil {
		return err
	}
	defer func() { _ = Release(lock) }()
	root, err := os.OpenRoot(workspace)
	if err != nil {
		return err
	}
	defer func() { _ = root.Close() }()
	defer func() {
		if result != nil && rollback != nil {
			result = errors.Join(result, rollback(root))
		}
	}()
	if err := checkRelativePath(root, registryPath); err != nil {
		return err
	}
	entries, original, err := readRegistry(root)
	if err != nil {
		return err
	}
	if err := operation(root, entries); err != nil {
		return err
	}
	encoded, err := json.Marshal(entries)
	if err != nil {
		return err
	}
	return atomicRootWrite(root, registryPath, encoded, original)
}

func readRegistry(root *os.Root) (registry, os.FileInfo, error) {
	entries := registry{}
	original, statErr := root.Lstat(registryPath)
	if statErr != nil && !os.IsNotExist(statErr) {
		return nil, nil, statErr
	}
	if original != nil && !original.Mode().IsRegular() {
		return nil, nil, errors.New("MCP ownership registry is not a regular file")
	}
	data, err := root.ReadFile(registryPath)
	if err != nil && !os.IsNotExist(err) {
		return nil, nil, err
	}
	if err == nil {
		if err := json.Unmarshal(data, &entries); err != nil || entries == nil {
			return nil, nil, errors.New("MCP file ownership registry is unreadable")
		}
		for _, entry := range entries {
			if entry == nil || entry.Owners == nil {
				return nil, nil, errors.New("MCP file ownership registry is invalid")
			}
		}
	}
	return entries, original, nil
}

func relativeConfigPath(workspace, privateRoot string, file File) (string, error) {
	relative, err := filepath.Rel(workspace, file.Path)
	if err != nil || relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", errors.New("MCP config path is outside the remote workspace")
	}
	if file.MergeKey == "" {
		private, err := filepath.Rel(privateRoot, file.Path)
		if err != nil || private == ".." || strings.HasPrefix(private, ".."+string(filepath.Separator)) {
			return "", errors.New("MCP temp config is outside its session directory")
		}
	}
	return relative, nil
}

func materializeRemoteFile(root *os.Root, relative, owner string, file File, entries registry, change *remoteFileChange) error {
	if err := checkRelativePath(root, relative); err != nil {
		return err
	}
	info := change.before
	var err error
	exists := info != nil
	owned := !exists
	content := file.Content
	previous := entries[relative]
	if exists {
		content, owned, err = existingRemoteContent(root, relative, info, file, previous)
		if err != nil {
			return err
		}
	}
	if err := validateRemoteFileClaims(previous, owner, change.original, content); err != nil {
		return err
	}
	change.written, err = atomicRootWriteInfo(root, relative, content, info)
	if err != nil {
		return err
	}
	owners := map[string]bool{}
	if previous != nil {
		owners = previous.Owners
	}
	owners[owner] = true
	entries[relative] = &registryEntry{Fingerprint: Fingerprint(content), Owned: owned, Owners: owners}
	return nil
}

func validateRemoteFileClaims(previous *registryEntry, owner string, existing, proposed []byte) error {
	if previous == nil {
		return nil
	}
	for claim := range previous.Owners {
		if claim == owner {
			continue
		}
		var before, after any
		if json.Unmarshal(existing, &before) != nil || json.Unmarshal(proposed, &after) != nil || !reflect.DeepEqual(before, after) {
			return errors.New("MCP configuration is in use by another session")
		}
	}
	return nil
}

func existingRemoteContent(root *os.Root, relative string, info os.FileInfo, file File, previous *registryEntry) ([]byte, bool, error) {
	content := file.Content
	if !info.Mode().IsRegular() {
		return nil, false, errors.New("MCP config is not a regular file")
	}
	existing, err := root.ReadFile(relative)
	if err != nil {
		return nil, false, err
	}
	owned := previous != nil && previous.Owned && previous.Fingerprint == Fingerprint(existing)
	if file.MergeKey == "" && !owned {
		return nil, false, errors.New("MCP temp config has unknown ownership")
	}
	if file.MergeKey != "" {
		content, err = MergeJSONUnderKey(existing, content, file.MergeKey)
		if err != nil {
			return nil, false, err
		}
	}
	return content, owned, nil
}

func removeRemoteClaim(root *os.Root, relative string, entry *registryEntry) error {
	if !entry.Owned {
		return nil
	}
	if err := checkRelativePath(root, relative); err != nil {
		return err
	}
	info, err := root.Lstat(relative)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return nil
	}
	content, err := root.ReadFile(relative)
	if err != nil {
		return err
	}
	if Fingerprint(content) != entry.Fingerprint {
		return nil
	}
	current, err := root.Lstat(relative)
	if err != nil {
		return err
	}
	if !os.SameFile(info, current) || info.ModTime() != current.ModTime() || info.Size() != current.Size() {
		return nil
	}
	return root.Remove(relative)
}

func checkRelativePath(root *os.Root, relative string) error {
	if filepath.IsAbs(relative) {
		return errors.New("absolute MCP relative path")
	}
	current := ""
	for _, component := range strings.Split(filepath.Clean(relative), string(filepath.Separator)) {
		if component == ".." {
			return errors.New("MCP path escapes its workspace")
		}
		current = filepath.Join(current, component)
		info, err := root.Lstat(current)
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return errors.New("MCP config path contains a symlink")
		}
	}
	return nil
}

func atomicRootWrite(root *os.Root, relative string, data []byte, original os.FileInfo) error {
	_, err := atomicRootWriteInfo(root, relative, data, original)
	return err
}

func atomicRootWriteInfo(root *os.Root, relative string, data []byte, original os.FileInfo) (os.FileInfo, error) {
	if err := root.MkdirAll(filepath.Dir(relative), 0o700); err != nil {
		return nil, err
	}
	temporary := relative + ".kandev-" + uuid.NewString() + ".tmp"
	file, err := root.OpenFile(temporary, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close(); _ = root.Remove(temporary) }()
	if _, err := file.Write(data); err != nil {
		return nil, err
	}
	if err := file.Sync(); err != nil {
		return nil, err
	}
	written, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if err := file.Close(); err != nil {
		return nil, err
	}
	if original != nil {
		current, err := root.Lstat(relative)
		if err != nil {
			return nil, err
		}
		if !os.SameFile(original, current) || original.ModTime() != current.ModTime() || original.Size() != current.Size() {
			return nil, fmt.Errorf("MCP file changed during preparation")
		}
	} else {
		return written, root.Link(temporary, relative)
	}
	return written, root.Rename(temporary, relative)
}
