package agents

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// nativeUpdateCheckDirEntryLimit bounds per-directory enumeration. Above it,
// the directory's own metadata records membership changes and only declared
// files are fingerprinted individually.
const nativeUpdateCheckDirEntryLimit = 256

const nativeUpdateCheckDirReadBatch = 128

// Fingerprint identifies the declared update configuration: values of the
// declared environment keys (matched case-insensitively) and the existence,
// size, and modification time of every declared source. File contents are
// never read, because they can hold credentials.
func (c NativeUpdateCheckContext) Fingerprint(resolver NativeUpdateCheckResolver) string {
	digest := sha256.New()
	for _, entry := range declaredEnvEntries(c.EnvKeys, resolver.Env) {
		writeFingerprintField(digest, "env", entry)
	}
	if c.Sources != nil {
		sources := c.Sources(resolver)
		files := slices.Clone(sources.Files)
		slices.Sort(files)
		for _, path := range slices.Compact(files) {
			writeFileFingerprint(digest, "file", path)
		}
		dirs := slices.Clone(sources.Dirs)
		slices.SortFunc(dirs, func(a, b NativeUpdateCheckDir) int { return strings.Compare(a.Path, b.Path) })
		for _, dir := range dirs {
			writeDirFingerprint(digest, dir)
		}
	}
	return hex.EncodeToString(digest.Sum(nil))
}

func declaredEnvEntries(keys, env []string) []string {
	entries := make([]string, 0, len(keys))
	for _, entry := range env {
		name, _, ok := strings.Cut(entry, "=")
		if !ok {
			continue
		}
		if slices.ContainsFunc(keys, func(key string) bool { return strings.EqualFold(key, name) }) {
			entries = append(entries, entry)
		}
	}
	slices.Sort(entries)
	return entries
}

func writeFingerprintField(digest hash.Hash, fields ...string) {
	for _, field := range fields {
		_, _ = io.WriteString(digest, field)
		_, _ = digest.Write([]byte{0})
	}
	_, _ = digest.Write([]byte{'\n'})
}

func writeFileFingerprint(digest hash.Hash, kind, path string) {
	info, err := os.Stat(path)
	if err != nil {
		writeFingerprintField(digest, kind, path, "absent")
		return
	}
	writeFingerprintField(digest, kind, path, "present", info.Mode().Type().String(),
		fmt.Sprint(info.Size()), fmt.Sprint(info.ModTime().UnixNano()))
}

func writeDirFingerprint(digest hash.Hash, dir NativeUpdateCheckDir) {
	names, overLimit := listDirFiles(dir)
	if overLimit {
		writeFileFingerprint(digest, "dir", dir.Path)
		return
	}
	writeFingerprintField(digest, "dir-members", dir.Path)
	for _, name := range names {
		writeFileFingerprint(digest, "member", filepath.Join(dir.Path, name))
	}
}

// listDirFiles returns sorted non-directory entries selected by dir.Match. It
// counts every directory entry toward the scan limit.
func listDirFiles(dir NativeUpdateCheckDir) ([]string, bool) {
	handle, err := openFingerprintDir(dir.Path)
	if err != nil {
		return nil, false
	}
	defer func() { _ = handle.Close() }()
	var names []string
	visited := 0
	for {
		entries, readErr := handle.ReadDir(nativeUpdateCheckDirReadBatch)
		for _, entry := range entries {
			visited++
			if visited > nativeUpdateCheckDirEntryLimit {
				return nil, true
			}
			if entry.IsDir() || (dir.Match != nil && !dir.Match(entry.Name())) {
				continue
			}
			names = append(names, entry.Name())
		}
		if readErr != nil {
			if !errors.Is(readErr, io.EOF) {
				return nil, true
			}
			break
		}
	}
	slices.Sort(names)
	return names, false
}
