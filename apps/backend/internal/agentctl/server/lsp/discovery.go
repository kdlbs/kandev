package lsp

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	sharedlsp "github.com/kandev/kandev/internal/lsp"
	"github.com/kandev/kandev/internal/lsp/installer"
)

const (
	DiscoveryReasonCanceled    = "canceled"
	DiscoveryReasonDeadline    = "deadline"
	DiscoveryReasonDepthLimit  = "depth_limit"
	DiscoveryReasonEntryLimit  = "entry_limit"
	DiscoveryReasonInvalidRoot = "invalid_root"
	DiscoveryReasonUnreadable  = "unreadable"

	defaultDiscoveryDuration = 2 * time.Second
	defaultDiscoveryEntries  = 10_000
	defaultDiscoveryDepth    = 6
)

type discoveryOptions struct {
	maxDuration time.Duration
	maxEntries  int
	maxDepth    int
}

type languageScanner struct {
	ctx        context.Context
	root       string
	options    discoveryOptions
	fileNames  map[string][]string
	extensions map[string][]string

	languages      map[string]struct{}
	reasons        map[string]struct{}
	visited        map[string]struct{}
	entriesScanned int
	readableRoot   bool
}

func DiscoverLanguages(ctx context.Context, root string, repositorySubpaths []string) sharedlsp.DiscoveryResult {
	return discoverLanguages(ctx, root, repositorySubpaths, discoveryOptions{
		maxDuration: defaultDiscoveryDuration,
		maxEntries:  defaultDiscoveryEntries,
		maxDepth:    defaultDiscoveryDepth,
	})
}

// DiscoverLanguagesAtRoots scans several trusted host roots under one shared
// deadline, entry budget, and depth budget. It is used for task environments
// whose runtime workspace path is not directly readable by the backend (for
// example /workspace inside Local Docker).
func DiscoverLanguagesAtRoots(ctx context.Context, roots []string) sharedlsp.DiscoveryResult {
	return discoverLanguagesAtRoots(ctx, roots, discoveryOptions{
		maxDuration: defaultDiscoveryDuration,
		maxEntries:  defaultDiscoveryEntries,
		maxDepth:    defaultDiscoveryDepth,
	})
}

func discoverLanguages(
	parent context.Context,
	root string,
	repositorySubpaths []string,
	options discoveryOptions,
) sharedlsp.DiscoveryResult {
	options = normalizeDiscoveryOptions(options)
	ctx, cancel := context.WithTimeout(parent, options.maxDuration)
	defer cancel()
	root = cleanAbsolutePath(root)
	scanner := newLanguageScanner(ctx, root, options)
	if !scanner.contextAvailable() {
		return scanner.result()
	}
	return scanner.scan(scanner.scanRoots(repositorySubpaths))
}

func discoverLanguagesAtRoots(
	parent context.Context,
	roots []string,
	options discoveryOptions,
) sharedlsp.DiscoveryResult {
	options = normalizeDiscoveryOptions(options)
	ctx, cancel := context.WithTimeout(parent, options.maxDuration)
	defer cancel()
	scanner := newLanguageScanner(ctx, "", options)
	if !scanner.contextAvailable() {
		return scanner.result()
	}
	return scanner.scan(scanner.exactScanRoots(roots))
}

func newLanguageScanner(ctx context.Context, root string, options discoveryOptions) *languageScanner {
	fileNames, extensions := discoverySignalIndexes(installer.DiscoverySignals())
	return &languageScanner{
		ctx: ctx, root: root, options: options, fileNames: fileNames, extensions: extensions,
		languages: make(map[string]struct{}), reasons: make(map[string]struct{}), visited: make(map[string]struct{}),
	}
}

func (s *languageScanner) scan(roots []string) sharedlsp.DiscoveryResult {
	for _, scanRoot := range roots {
		if !s.contextAvailable() || s.entriesScanned >= s.options.maxEntries {
			if s.entriesScanned >= s.options.maxEntries {
				s.addReason(DiscoveryReasonEntryLimit)
			}
			break
		}
		s.scanDirectory(scanRoot, 0)
	}
	return s.result()
}

func normalizeDiscoveryOptions(options discoveryOptions) discoveryOptions {
	if options.maxDuration <= 0 {
		options.maxDuration = defaultDiscoveryDuration
	}
	if options.maxEntries <= 0 {
		options.maxEntries = defaultDiscoveryEntries
	}
	if options.maxDepth < 0 {
		options.maxDepth = defaultDiscoveryDepth
	}
	return options
}

func (s *languageScanner) scanRoots(repositorySubpaths []string) []string {
	roots := []string{s.root}
	seen := map[string]struct{}{s.root: {}}
	for _, subpath := range repositorySubpaths {
		candidate, ok := containedDiscoveryRoot(s.root, subpath)
		if !ok || containsDirectorySymlink(s.root, candidate) {
			s.addReason(DiscoveryReasonInvalidRoot)
			continue
		}
		if _, duplicate := seen[candidate]; duplicate {
			continue
		}
		seen[candidate] = struct{}{}
		roots = append(roots, candidate)
	}
	return roots
}

func (s *languageScanner) exactScanRoots(roots []string) []string {
	seen := make(map[string]struct{}, len(roots))
	result := make([]string, 0, len(roots))
	for _, root := range roots {
		if root == "" || !filepath.IsAbs(root) {
			s.addReason(DiscoveryReasonInvalidRoot)
			continue
		}
		root = filepath.Clean(root)
		if filepath.Dir(root) == root {
			s.addReason(DiscoveryReasonInvalidRoot)
			continue
		}
		if _, duplicate := seen[root]; duplicate {
			continue
		}
		seen[root] = struct{}{}
		result = append(result, root)
	}
	if len(result) == 0 {
		s.addReason(DiscoveryReasonInvalidRoot)
	}
	sort.Strings(result)
	return result
}

func (s *languageScanner) scanDirectory(directory string, depth int) {
	if !s.contextAvailable() {
		return
	}
	directory = filepath.Clean(directory)
	if _, duplicate := s.visited[directory]; duplicate {
		return
	}
	s.visited[directory] = struct{}{}
	remaining := s.options.maxEntries - s.entriesScanned
	if remaining <= 0 {
		s.addReason(DiscoveryReasonEntryLimit)
		return
	}
	entries, hasMore, err := readDirectoryBounded(directory, remaining)
	if err != nil {
		s.addReason(DiscoveryReasonUnreadable)
		return
	}
	s.readableRoot = true
	sort.Slice(entries, func(left, right int) bool { return entries[left].Name() < entries[right].Name() })
	for _, entry := range entries {
		if !s.contextAvailable() {
			return
		}
		if s.entriesScanned >= s.options.maxEntries {
			s.addReason(DiscoveryReasonEntryLimit)
			return
		}
		s.entriesScanned++
		s.inspectEntry(directory, depth, entry)
	}
	if hasMore {
		s.addReason(DiscoveryReasonEntryLimit)
	}
}

func (s *languageScanner) inspectEntry(directory string, depth int, entry os.DirEntry) {
	if entry.Type()&os.ModeSymlink != 0 {
		return
	}
	name := strings.ToLower(entry.Name())
	if entry.IsDir() {
		if ignoredDiscoveryDirectory(name) {
			return
		}
		if depth >= s.options.maxDepth {
			s.addReason(DiscoveryReasonDepthLimit)
			return
		}
		s.scanDirectory(filepath.Join(directory, entry.Name()), depth+1)
		return
	}
	for _, language := range s.fileNames[name] {
		s.languages[language] = struct{}{}
	}
	for _, language := range s.extensions[strings.ToLower(filepath.Ext(name))] {
		s.languages[language] = struct{}{}
	}
}

func (s *languageScanner) contextAvailable() bool {
	if err := s.ctx.Err(); err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			s.addReason(DiscoveryReasonDeadline)
		} else {
			s.addReason(DiscoveryReasonCanceled)
		}
		return false
	}
	return true
}

func (s *languageScanner) addReason(reason string) {
	s.reasons[reason] = struct{}{}
}

func (s *languageScanner) result() sharedlsp.DiscoveryResult {
	languages := make([]string, 0, len(s.languages))
	for language := range s.languages {
		languages = append(languages, language)
	}
	sort.Strings(languages)
	reasons := make([]string, 0, len(s.reasons))
	for reason := range s.reasons {
		reasons = append(reasons, reason)
	}
	sort.Strings(reasons)
	state := sharedlsp.DetectionComplete
	if len(reasons) != 0 {
		state = sharedlsp.DetectionPartial
		if !s.readableRoot && s.entriesScanned == 0 {
			state = sharedlsp.DetectionUnavailable
		}
	}
	return sharedlsp.DiscoveryResult{
		Languages:      languages,
		State:          state,
		Truncated:      len(reasons) != 0,
		EntriesScanned: s.entriesScanned,
		ScannedAt:      time.Now().UTC(),
		Reasons:        reasons,
	}
}

func discoverySignalIndexes(
	signals []installer.DiscoverySignal,
) (map[string][]string, map[string][]string) {
	fileNames := make(map[string][]string)
	extensions := make(map[string][]string)
	for _, signal := range signals {
		for _, fileName := range signal.FileNames {
			key := strings.ToLower(fileName)
			fileNames[key] = append(fileNames[key], signal.Language)
		}
		for _, extension := range signal.Extensions {
			key := strings.ToLower(extension)
			extensions[key] = append(extensions[key], signal.Language)
		}
	}
	return fileNames, extensions
}

func containedDiscoveryRoot(root, subpath string) (string, bool) {
	if subpath == "" || filepath.IsAbs(subpath) {
		return "", false
	}
	candidate := filepath.Clean(filepath.Join(root, subpath))
	relative, err := filepath.Rel(root, candidate)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", false
	}
	return candidate, true
}

func containsDirectorySymlink(root, target string) bool {
	relative, err := filepath.Rel(root, target)
	if err != nil {
		return true
	}
	current := root
	for _, part := range strings.Split(relative, string(filepath.Separator)) {
		if part == "" || part == "." {
			continue
		}
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if err != nil {
			return false
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return true
		}
	}
	return false
}

func readDirectoryBounded(directory string, remaining int) ([]os.DirEntry, bool, error) {
	validated, err := os.Lstat(directory)
	if err != nil || !validated.IsDir() || validated.Mode()&os.ModeSymlink != 0 {
		return nil, false, errors.Join(err, errors.New("discovery root is not a readable directory"))
	}
	handle, err := os.Open(directory)
	if err != nil {
		return nil, false, err
	}
	defer func() { _ = handle.Close() }()
	opened, err := handle.Stat()
	if err != nil || !os.SameFile(validated, opened) {
		return nil, false, errors.Join(err, errors.New("discovery directory changed while opening"))
	}
	entries := make([]os.DirEntry, 0, min(remaining+1, 128))
	for len(entries) <= remaining {
		batchSize := min(128, remaining+1-len(entries))
		batch, readErr := handle.ReadDir(batchSize)
		entries = append(entries, batch...)
		if errors.Is(readErr, io.EOF) {
			return entries, false, nil
		}
		if readErr != nil {
			return nil, false, readErr
		}
	}
	return entries[:remaining], true, nil
}

func ignoredDiscoveryDirectory(name string) bool {
	switch name {
	case ".git", ".gradle", ".hg", ".idea", ".next", ".svn", ".turbo", ".venv",
		"__pycache__", "build", "coverage", "dist", "env", "node_modules", "out", "target",
		"vendor", "venv":
		return true
	default:
		return strings.HasPrefix(name, "bazel-")
	}
}

func cleanAbsolutePath(path string) string {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return filepath.Clean(path)
	}
	return filepath.Clean(absolute)
}
