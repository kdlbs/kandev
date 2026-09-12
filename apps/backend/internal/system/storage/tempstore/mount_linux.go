//go:build linux

package tempstore

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type mountReader struct {
	readMountInfo func() ([]byte, error)
}

func newMountReader() MountReader {
	return &mountReader{readMountInfo: func() ([]byte, error) {
		return os.ReadFile("/proc/self/mountinfo")
	}}
}

func (r *mountReader) Identity(path string) (string, error) {
	canonical, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return "", err
	}
	mounts, err := r.load()
	if err != nil {
		return "", err
	}
	best := ""
	bestIdentity := ""
	for _, mount := range mounts {
		if pathWithin(mount.mountpoint, canonical) && len(mount.mountpoint) > len(best) {
			best = mount.mountpoint
			bestIdentity = mount.identity
		}
	}
	if best == "" || bestIdentity == "" {
		return "", fmt.Errorf("mount identity is unavailable for %s", path)
	}
	return bestIdentity, nil
}

type mountEntry struct {
	mountpoint string
	identity   string
}

func (r *mountReader) load() ([]mountEntry, error) {
	readMountInfo := r.readMountInfo
	if readMountInfo == nil {
		readMountInfo = func() ([]byte, error) {
			return os.ReadFile("/proc/self/mountinfo")
		}
	}
	data, err := readMountInfo()
	if err != nil {
		return nil, fmt.Errorf("read mount information: %w", err)
	}
	mounts := make([]mountEntry, 0)
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 6 {
			continue
		}
		root, err := unescapeMountInfo(fields[3])
		if err != nil {
			return nil, err
		}
		mountpoint, err := unescapeMountInfo(fields[4])
		if err != nil {
			return nil, err
		}
		mountpoint, err = filepath.Abs(filepath.Clean(mountpoint))
		if err != nil {
			return nil, err
		}
		identity := strings.Join([]string{fields[0], fields[1], fields[2], root, mountpoint}, "\x00")
		mounts = append(mounts, mountEntry{mountpoint: mountpoint, identity: identity})
	}
	if len(mounts) == 0 {
		return nil, errors.New("mount information is empty")
	}
	return mounts, nil
}

func unescapeMountInfo(value string) (string, error) {
	var builder strings.Builder
	for index := 0; index < len(value); index++ {
		if value[index] != '\\' {
			builder.WriteByte(value[index])
			continue
		}
		if index+3 >= len(value) {
			return "", fmt.Errorf("invalid mount path escape %q", value)
		}
		code, err := strconv.ParseUint(value[index+1:index+4], 8, 8)
		if err != nil {
			return "", fmt.Errorf("decode mount path %q: %w", value, err)
		}
		builder.WriteByte(byte(code))
		index += 3
	}
	return builder.String(), nil
}
