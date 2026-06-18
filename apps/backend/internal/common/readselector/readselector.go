// Package readselector parses Oh-My-Pi (omp) style read paths of the form
// "path:<selector>" into the bare file path plus the parsed line range.
//
// OMP's read tool embeds a line/range/mode selector in the path argument
// (e.g. "foo.go:43-94", "foo.go:50+150", "foo.go:5-16,960-973", "foo.go:raw",
// "foo.go:2-4:raw"). Kandev opens files by stat'ing the path, so a selector
// left on the path makes the open fail ("Failed to open file … no such file"
// for workspace files, or "path traversal detected" for absolute external
// files whose stat'd "<path>:<range>" simply doesn't exist).
//
// Both the ACP normalizer (so new file links are stored clean and the line
// range is surfaced separately) and the file-open boundary (so already-persisted
// links and any normalization gap still open) call Split — a single source of
// truth shared by both.
package readselector

import (
	"strconv"
	"strings"
)

// Split splits "path:<selector>" into the bare file path plus the parsed line
// range. startLine is the first line referenced by the selector; lineCount is
// the contiguous span when a closed range is given ("N-M" or "N+K"), and 0 when
// the selector is open-ended ("N", "N-") or carries no line numbers ("raw").
// When no valid selector is present the path is returned unchanged with zero
// line info, so paths from non-omp agents (which never match the grammar) and
// ordinary filenames are unaffected.
func Split(raw string) (path string, startLine, lineCount int) {
	// Only inspect colons in the final path segment so a directory component
	// that legitimately contains a colon is never mistaken for a selector.
	// Consider both '/' and '\\' so Windows paths — and their drive-letter
	// colon, e.g. "C:\\dir\\file.go" — are not misread as a selector boundary.
	lastSep := strings.LastIndexByte(raw, '/')
	if b := strings.LastIndexByte(raw, '\\'); b > lastSep {
		lastSep = b
	}
	rel := strings.IndexByte(raw[lastSep+1:], ':')
	if rel < 0 {
		return raw, 0, 0
	}
	colon := lastSep + 1 + rel
	if lastSep < 0 && isWindowsDrivePrefix(raw, colon) {
		return raw, 0, 0
	}
	base, suffix := raw[:colon], raw[colon+1:]
	if base == "" || suffix == "" {
		return raw, 0, 0
	}
	start, count, ok := parseReadSelector(suffix)
	if !ok {
		return raw, 0, 0
	}
	return base, start, count
}

// File is one file reference parsed out of a (possibly multi-file) read path.
type File struct {
	Path      string
	StartLine int
	LineCount int
}

// SplitFiles splits an omp read path that may reference several comma-joined
// files ("a.yaml:1-80,b.yaml:1-80") into one File per file, each with its own
// parsed line range. A comma segment that is purely a line-spec list ("960-973")
// or a mode keyword ("raw"/"conflicts") is treated as an additional range of the
// preceding file — so a single file's multi-range selector ("main.go:5-16,40-80")
// stays one File, matching Split (first range wins).
//
// Multiple files are only reported when every parsed segment yields a real file
// path (one containing a path separator or a filename extension); otherwise —
// e.g. a directory name that legitimately contains a comma — it returns the
// single Split result, so single-file behavior is never disturbed.
func SplitFiles(raw string) []File {
	parts := strings.Split(raw, ",")
	files := make([]File, 0, len(parts))
	for i, part := range parts {
		if i > 0 && isContinuationRange(part) {
			continue
		}
		path, start, count := Split(part)
		files = append(files, File{Path: path, StartLine: start, LineCount: count})
	}
	if len(files) > 1 && allFileish(files) {
		return files
	}
	path, start, count := Split(raw)
	return []File{{Path: path, StartLine: start, LineCount: count}}
}

// isContinuationRange reports whether a comma segment is an extra range of the
// preceding file rather than a new file: a bare line-spec list or a mode keyword.
func isContinuationRange(part string) bool {
	if part == "raw" || part == "conflicts" {
		return true
	}
	_, _, ok := parseLineSpecList(part)
	return ok
}

func allFileish(files []File) bool {
	for _, f := range files {
		if !isFileish(f.Path) {
			return false
		}
	}
	return true
}

// isFileish reports whether a path looks like a concrete file: it has a path
// separator or a "." in its final segment. Used to distinguish a real second
// file from a comma that merely lives inside a single path.
func isFileish(path string) bool {
	if strings.ContainsAny(path, `/\`) {
		return true
	}
	return strings.Contains(path, ".")
}

func isWindowsDrivePrefix(raw string, colon int) bool {
	return colon == 1 && len(raw) >= 2 && isASCIIAlpha(raw[0])
}

func isASCIIAlpha(b byte) bool {
	return (b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z')
}

// parseReadSelector validates the full selector tail (everything after the
// first ':' in the final segment). Selector parts are joined by ':' so combos
// such as "2-4:raw" and "raw:2-4" are accepted; each part must be a line-spec
// list or a recognized mode keyword. The first line-spec encountered drives the
// returned startLine/lineCount.
func parseReadSelector(suffix string) (startLine, lineCount int, ok bool) {
	gotLine := false
	for _, part := range strings.Split(suffix, ":") {
		if part == "raw" || part == "conflicts" {
			continue
		}
		s, c, valid := parseLineSpecList(part)
		if !valid {
			return 0, 0, false
		}
		if !gotLine {
			startLine, lineCount, gotLine = s, c, true
		}
	}
	return startLine, lineCount, true
}

// parseLineSpecList parses a comma-separated list of line specs ("5-16,960-973")
// and reports the start/count of the first spec.
func parseLineSpecList(part string) (startLine, lineCount int, ok bool) {
	segs := strings.Split(part, ",")
	for i, seg := range segs {
		s, c, valid := parseLineSpec(seg)
		if !valid {
			return 0, 0, false
		}
		if i == 0 {
			startLine, lineCount = s, c
		}
	}
	return startLine, lineCount, true
}

// parseLineSpec parses a single line spec: "N", "N-", "N-M", or "N+K".
func parseLineSpec(seg string) (startLine, lineCount int, ok bool) {
	if i := strings.IndexByte(seg, '+'); i >= 0 {
		start, errA := strconv.Atoi(seg[:i])
		count, errB := strconv.Atoi(seg[i+1:])
		if errA != nil || errB != nil || start <= 0 || count < 0 {
			return 0, 0, false
		}
		return start, count, true
	}
	if i := strings.IndexByte(seg, '-'); i >= 0 {
		start, errA := strconv.Atoi(seg[:i])
		if errA != nil || start <= 0 {
			return 0, 0, false
		}
		rest := seg[i+1:]
		if rest == "" { // "N-" — open-ended
			return start, 0, true
		}
		end, errB := strconv.Atoi(rest)
		if errB != nil || end < start {
			return 0, 0, false
		}
		return start, end - start + 1, true
	}
	start, err := strconv.Atoi(seg)
	if err != nil || start <= 0 {
		return 0, 0, false
	}
	return start, 0, true
}
