package launcher

import (
	"fmt"
	"os"
)

type nativeMetadataRoot interface {
	Close() error
	EntryMode(name string) (os.FileMode, error)
	Mkdir(name string, perm os.FileMode) error
	OpenRoot(name string) (nativeMetadataRoot, error)
	OpenSelf() (*os.File, error)
	CreateExclusive(name string, perm os.FileMode) (*os.File, error)
	Rename(oldName, newName string) error
	Remove(name string) error
}

type portableNativeMetadataRoot struct {
	root *os.Root
}

func openNativeMetadataHome(metadata nativeServiceMetadata) (nativeMetadataRoot, error) {
	if metadata.Mode == nativeServiceModeSystem {
		return openSystemNativeMetadataHome(metadata.HomeDir)
	}
	if err := os.MkdirAll(metadata.HomeDir, 0o700); err != nil {
		return nil, fmt.Errorf("create service metadata home: %w", err)
	}
	root, err := os.OpenRoot(metadata.HomeDir)
	if err != nil {
		return nil, fmt.Errorf("open service metadata home: %w", err)
	}
	return &portableNativeMetadataRoot{root: root}, nil
}

func (r *portableNativeMetadataRoot) Close() error {
	return r.root.Close()
}

func (r *portableNativeMetadataRoot) EntryMode(name string) (os.FileMode, error) {
	info, err := r.root.Lstat(name)
	if err != nil {
		return 0, err
	}
	return info.Mode(), nil
}

func (r *portableNativeMetadataRoot) Mkdir(name string, perm os.FileMode) error {
	return r.root.Mkdir(name, perm)
}

func (r *portableNativeMetadataRoot) OpenRoot(name string) (nativeMetadataRoot, error) {
	root, err := r.root.OpenRoot(name)
	if err != nil {
		return nil, err
	}
	return &portableNativeMetadataRoot{root: root}, nil
}

func (r *portableNativeMetadataRoot) OpenSelf() (*os.File, error) {
	return r.root.Open(".")
}

func (r *portableNativeMetadataRoot) CreateExclusive(name string, perm os.FileMode) (*os.File, error) {
	return r.root.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_WRONLY, perm)
}

func (r *portableNativeMetadataRoot) Rename(oldName, newName string) error {
	return r.root.Rename(oldName, newName)
}

func (r *portableNativeMetadataRoot) Remove(name string) error {
	return r.root.Remove(name)
}
