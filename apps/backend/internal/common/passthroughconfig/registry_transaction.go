package passthroughconfig

import (
	"errors"
	"os"
)

type remoteFileChange struct {
	relative string
	original []byte
	before   os.FileInfo
	written  os.FileInfo
}

func captureRemoteFile(root *os.Root, relative string) (remoteFileChange, error) {
	change := remoteFileChange{relative: relative}
	if err := checkRelativePath(root, relative); err != nil {
		return change, err
	}
	info, err := root.Lstat(relative)
	if os.IsNotExist(err) {
		return change, nil
	}
	if err != nil {
		return change, err
	}
	if !info.Mode().IsRegular() {
		return change, errors.New("MCP config is not a regular file")
	}
	change.before = info
	change.original, err = root.ReadFile(relative)
	return change, err
}

func rollbackRemoteFiles(root *os.Root, changes []remoteFileChange) error {
	var result error
	for i := len(changes) - 1; i >= 0; i-- {
		result = errors.Join(result, changes[i].rollback(root))
	}
	return result
}

func (change remoteFileChange) rollback(root *os.Root) error {
	if err := checkRelativePath(root, change.relative); err != nil {
		return err
	}
	current, err := root.Lstat(change.relative)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	// An external replacement or edit no longer belongs to this preparation.
	if !os.SameFile(change.written, current) || change.written.ModTime() != current.ModTime() || change.written.Size() != current.Size() {
		return nil
	}
	if change.before == nil {
		return root.Remove(change.relative)
	}
	return atomicRootWrite(root, change.relative, change.original, current)
}
