package turnchanges

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
)

// RawFileChange is the identity and mode/object metadata emitted by git diff
// --raw -z for one exact path or rename/copy pair.
type RawFileChange struct {
	Status  string
	OldMode string
	NewMode string
	OldOID  string
	NewOID  string
	OldPath []byte
	Path    []byte
}

// NumstatFileChange carries nullable line counts from git diff --numstat -z.
// Nil counts mean Git reported a binary file; a pointer to zero is known zero.
type NumstatFileChange struct {
	OldPath []byte
	Path    []byte
	Added   *int64
	Deleted *int64
	Binary  bool
}

// ParseRawDiff parses NUL-delimited --raw output without decoding paths.
func ParseRawDiff(output []byte) ([]RawFileChange, error) {
	if len(output) == 0 {
		return []RawFileChange{}, nil
	}
	var changes []RawFileChange
	for len(output) > 0 {
		header, rest, ok := bytes.Cut(output, []byte{0})
		if !ok {
			return nil, fmt.Errorf("raw diff header is not NUL-terminated")
		}
		metadata, err := parseRawDiffHeader(header)
		if err != nil {
			return nil, err
		}
		output = rest
		path, rest, ok := bytes.Cut(output, []byte{0})
		if !ok || len(path) == 0 {
			return nil, fmt.Errorf("raw diff path is missing or not NUL-terminated")
		}
		metadata.Path = append([]byte(nil), path...)
		if isRenameOrCopy(metadata.Status) {
			oldPath := metadata.Path
			newPath, tail, pathOK := bytes.Cut(rest, []byte{0})
			if !pathOK || len(newPath) == 0 {
				return nil, fmt.Errorf("raw diff rename destination is missing or not NUL-terminated")
			}
			metadata.OldPath = oldPath
			metadata.Path = append([]byte(nil), newPath...)
			rest = tail
		}
		changes = append(changes, metadata)
		output = rest
	}
	return changes, nil
}

func parseRawDiffHeader(header []byte) (RawFileChange, error) {
	fields := strings.Fields(string(header))
	if len(fields) != 5 || len(fields[0]) < 2 || fields[0][0] != ':' {
		return RawFileChange{}, fmt.Errorf("raw diff header has invalid shape")
	}
	change := RawFileChange{
		OldMode: fields[0][1:], NewMode: fields[1],
		OldOID: fields[2], NewOID: fields[3], Status: fields[4],
	}
	if !validRawMode(change.OldMode) || !validRawMode(change.NewMode) || !validRawOIDPair(change.OldOID, change.NewOID) || !validRawStatus(change.Status) {
		return RawFileChange{}, fmt.Errorf("raw diff header has invalid mode, object ID, or status")
	}
	return change, nil
}

func validRawMode(mode string) bool {
	if len(mode) != 6 {
		return false
	}
	for _, digit := range mode {
		if digit < '0' || digit > '7' {
			return false
		}
	}
	return true
}

func validRawOIDPair(oldOID, newOID string) bool {
	if len(oldOID) != len(newOID) || (len(oldOID) != 40 && len(oldOID) != 64) {
		return false
	}
	for _, oid := range []string{oldOID, newOID} {
		for _, digit := range oid {
			if (digit < '0' || digit > '9') && (digit < 'a' || digit > 'f') && (digit < 'A' || digit > 'F') {
				return false
			}
		}
	}
	return true
}

func validRawStatus(status string) bool {
	if len(status) == 0 || len(status) > 4 {
		return false
	}
	kind := status[0]
	if !strings.ContainsRune("ACDMRTUXB", rune(kind)) {
		return false
	}
	if kind == 'R' || kind == 'C' {
		if len(status) < 2 {
			return false
		}
		for _, score := range status[1:] {
			if score < '0' || score > '9' {
				return false
			}
		}
		return true
	}
	return len(status) == 1
}

func isRenameOrCopy(status string) bool {
	return strings.HasPrefix(status, "R") || strings.HasPrefix(status, "C")
}

// ParseNumstat parses NUL-delimited --numstat output. Rename and copy records
// carry an empty path field followed by the old and new paths.
func ParseNumstat(output []byte) ([]NumstatFileChange, error) {
	if len(output) == 0 {
		return []NumstatFileChange{}, nil
	}
	var changes []NumstatFileChange
	for len(output) > 0 {
		record, rest, ok := bytes.Cut(output, []byte{0})
		if !ok {
			return nil, fmt.Errorf("numstat record is not NUL-terminated")
		}
		addedRaw, remainder, ok := bytes.Cut(record, []byte{'\t'})
		if !ok {
			return nil, fmt.Errorf("numstat record has no added-count separator")
		}
		deletedRaw, path, ok := bytes.Cut(remainder, []byte{'\t'})
		if !ok {
			return nil, fmt.Errorf("numstat record has no deleted-count separator")
		}
		counts, err := parseNumstatCounts(addedRaw, deletedRaw)
		if err != nil {
			return nil, err
		}
		if len(path) > 0 {
			counts.Path = append([]byte(nil), path...)
			changes = append(changes, counts)
			output = rest
			continue
		}
		oldPath, tail, oldOK := bytes.Cut(rest, []byte{0})
		newPath, tail, newOK := bytes.Cut(tail, []byte{0})
		if !oldOK || !newOK || len(oldPath) == 0 || len(newPath) == 0 {
			return nil, fmt.Errorf("numstat rename/copy paths are missing or not NUL-terminated")
		}
		counts.OldPath = append([]byte(nil), oldPath...)
		counts.Path = append([]byte(nil), newPath...)
		changes = append(changes, counts)
		output = tail
	}
	return changes, nil
}

func parseNumstatCounts(addedRaw, deletedRaw []byte) (NumstatFileChange, error) {
	if bytes.Equal(addedRaw, []byte("-")) || bytes.Equal(deletedRaw, []byte("-")) {
		if !bytes.Equal(addedRaw, []byte("-")) || !bytes.Equal(deletedRaw, []byte("-")) {
			return NumstatFileChange{}, fmt.Errorf("numstat binary counts are inconsistent")
		}
		return NumstatFileChange{Binary: true}, nil
	}
	added, err := parseNumstatCount(addedRaw)
	if err != nil {
		return NumstatFileChange{}, fmt.Errorf("parse numstat additions: %w", err)
	}
	deleted, err := parseNumstatCount(deletedRaw)
	if err != nil {
		return NumstatFileChange{}, fmt.Errorf("parse numstat deletions: %w", err)
	}
	return NumstatFileChange{Added: &added, Deleted: &deleted}, nil
}

func parseNumstatCount(raw []byte) (int64, error) {
	count, err := strconv.ParseInt(string(raw), 10, 64)
	if err != nil || count < 0 {
		return 0, fmt.Errorf("invalid non-negative count %q", raw)
	}
	return count, nil
}

// JoinGitDiff attaches numstat records to their exact raw-diff path identities.
// A mismatch is an incomplete comparison, never an empty or ready summary.
func JoinGitDiff(raw []RawFileChange, numstat []NumstatFileChange) ([]CheckpointFile, error) {
	byPath := make(map[string]NumstatFileChange, len(numstat))
	for _, counts := range numstat {
		key := string(counts.Path)
		if _, exists := byPath[key]; exists {
			return nil, fmt.Errorf("duplicate numstat path identity")
		}
		byPath[key] = counts
	}
	files := make([]CheckpointFile, 0, len(raw))
	for _, change := range raw {
		key := string(change.Path)
		counts, exists := byPath[key]
		if !exists || !bytes.Equal(counts.OldPath, change.OldPath) {
			return nil, fmt.Errorf("raw diff and numstat path identities differ")
		}
		delete(byPath, key)
		kind, err := fileChangeKind(change)
		if err != nil {
			return nil, err
		}
		file := CheckpointFile{
			PathBytes: append([]byte(nil), change.Path...), OldPathBytes: append([]byte(nil), change.OldPath...),
			Kind: kind, OldOID: change.OldOID, NewOID: change.NewOID,
			OldMode: change.OldMode, NewMode: change.NewMode,
			Submodule: change.OldMode == "160000" || change.NewMode == "160000",
			Added:     counts.Added, Deleted: counts.Deleted, Binary: counts.Binary,
		}
		files = append(files, file)
	}
	if len(byPath) != 0 {
		return nil, fmt.Errorf("numstat contains paths missing from raw diff")
	}
	return files, nil
}

func fileChangeKind(change RawFileChange) (string, error) {
	switch change.Status[0] {
	case 'A':
		return "added", nil
	case 'D':
		return "deleted", nil
	case 'R':
		return "renamed", nil
	case 'C':
		return "copied", nil
	case 'T':
		return "type_changed", nil
	case 'M':
		if change.OldMode != change.NewMode && change.OldOID == change.NewOID {
			return "mode_changed", nil
		}
		return "modified", nil
	default:
		return "", fmt.Errorf("raw diff contains unsupported status %q", change.Status)
	}
}
