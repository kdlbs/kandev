package lsp

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	goruntime "runtime"
	"strings"
	"sync"
)

var errDocumentOutsideWorkspace = errors.New("document URI is outside the task workspace")

const windowsOS = "windows"

// documentWorkspace authorizes browser-originated document URIs against the
// task workspace projection captured by the task-host generation.
type documentWorkspace struct {
	mu                     sync.RWMutex
	roots                  []documentRoot
	resolveTrustedRootPath func(string) (string, error)
}

type documentRoot struct {
	lexical   string
	canonical string
}

func newDocumentWorkspace() *documentWorkspace {
	return newDocumentWorkspaceWithResolver(canonicalFilesystemPath)
}

func newDocumentWorkspaceWithResolver(
	resolvePath func(string) (string, error),
) *documentWorkspace {
	return &documentWorkspace{resolveTrustedRootPath: resolvePath}
}

func (w *documentWorkspace) SetSnapshot(snapshot Snapshot) {
	w.set(snapshot.WorkspacePath, snapshot.WorkspaceURI, snapshot.WorkspaceFolders)
}

func (w *documentWorkspace) SetConfig(config Config) {
	w.set(config.WorkDir, config.WorkspaceURI, config.WorkspaceFolders)
}

func (w *documentWorkspace) set(
	workspacePath, workspaceURI string,
	folders []WorkspaceFolder,
) {
	paths := make([]string, 0, len(folders)+2)
	paths = append(paths, workspacePath)
	if uriPath, err := localFileURIPath(workspaceURI); err == nil {
		paths = append(paths, uriPath)
	}
	for _, folder := range folders {
		if folderPath, err := localFileURIPath(folder.URI); err == nil {
			paths = append(paths, folderPath)
		}
	}

	roots := make([]documentRoot, 0, len(paths))
	seen := make(map[string]struct{}, len(paths))
	for _, candidate := range paths {
		lexical, err := cleanAbsoluteFilesystemPath(candidate)
		if err != nil {
			continue
		}
		canonical, err := w.resolveTrustedRootPath(lexical)
		if err != nil {
			continue
		}
		key := lexical + "\x00" + canonical
		if _, duplicate := seen[key]; duplicate {
			continue
		}
		seen[key] = struct{}{}
		roots = append(roots, documentRoot{lexical: lexical, canonical: canonical})
	}
	w.mu.Lock()
	w.roots = roots
	w.mu.Unlock()
}

func (w *documentWorkspace) CanonicalURI(raw string) (string, error) {
	documentPath, err := localFileURIPath(raw)
	if err != nil {
		return "", err
	}
	w.mu.RLock()
	roots := append([]documentRoot(nil), w.roots...)
	w.mu.RUnlock()
	if !lexicallyAuthorizedDocumentPath(documentPath, roots) {
		return "", fmt.Errorf("%w: %s", errDocumentOutsideWorkspace, raw)
	}
	canonicalPath, err := canonicalDocumentPath(documentPath, roots)
	if err != nil {
		return "", err
	}
	for _, root := range roots {
		if pathWithinRoot(canonicalPath, root.canonical) {
			return WorkspaceFileURI(canonicalPath), nil
		}
	}
	return "", fmt.Errorf("%w: %s", errDocumentOutsideWorkspace, raw)
}

type documentFilesystem struct {
	lstat    func(string) (fs.FileInfo, error)
	readlink func(string) (string, error)
}

var localDocumentFilesystem = documentFilesystem{
	lstat:    os.Lstat,
	readlink: os.Readlink,
}

type documentPathProjection struct {
	root     string
	relative string
	score    int
}

func canonicalDocumentPath(path string, roots []documentRoot) (string, error) {
	return canonicalDocumentPathWithFilesystem(path, roots, localDocumentFilesystem)
}

func canonicalDocumentPathWithFilesystem(
	path string,
	roots []documentRoot,
	filesystem documentFilesystem,
) (string, error) {
	projection, ok := projectDocumentPath(path, roots)
	if !ok {
		return "", errDocumentOutsideWorkspace
	}
	return walkDocumentPath(projection, roots, filesystem)
}

func walkDocumentPath(
	projection documentPathProjection,
	roots []documentRoot,
	filesystem documentFilesystem,
) (string, error) {
	pending := pathComponents(projection.relative)
	current := projection.root
	linksWalked := 0
	for len(pending) != 0 {
		component := pending[0]
		pending = pending[1:]
		candidate := filepath.Join(current, component)
		if !pathWithinCanonicalRoot(candidate, roots) {
			return "", errDocumentOutsideWorkspace
		}
		info, err := filesystem.lstat(candidate)
		if errors.Is(err, fs.ErrNotExist) {
			return unresolvedDocumentTail(candidate, pending, roots)
		}
		if err != nil {
			return "", fmt.Errorf("inspect file document path: %w", err)
		}
		if !isDocumentLink(info) {
			if len(pending) != 0 && !info.IsDir() {
				return "", errors.New("file document ancestor is not a directory")
			}
			current = candidate
			continue
		}
		linksWalked++
		if linksWalked > 255 {
			return "", errors.New("resolve file document path: too many links")
		}
		target, err := filesystem.readlink(candidate)
		if err != nil {
			return "", fmt.Errorf("read file document link: %w", err)
		}
		targetPath, err := linkTargetPath(candidate, target)
		if err != nil {
			return "", err
		}
		nextProjection, ok := projectDocumentPath(targetPath, roots)
		if !ok {
			return "", errDocumentOutsideWorkspace
		}
		pending = append(pathComponents(nextProjection.relative), pending...)
		current = nextProjection.root
	}
	return filepath.Clean(current), nil
}

func isDocumentLink(info fs.FileInfo) bool {
	mode := info.Mode()
	return mode&fs.ModeSymlink != 0 ||
		goruntime.GOOS == windowsOS && mode&fs.ModeIrregular != 0
}

func projectDocumentPath(path string, roots []documentRoot) (documentPathProjection, bool) {
	best := documentPathProjection{score: -1}
	for _, root := range roots {
		for _, source := range []string{root.lexical, root.canonical} {
			relative, err := filepath.Rel(source, path)
			if err != nil || pathLeavesRoot(relative) || len(source) <= best.score {
				continue
			}
			best = documentPathProjection{
				root: root.canonical, relative: relative, score: len(source),
			}
		}
	}
	return best, best.score >= 0
}

func pathComponents(path string) []string {
	if path == "." || path == "" {
		return nil
	}
	return strings.Split(filepath.Clean(path), string(filepath.Separator))
}

func linkTargetPath(linkPath, target string) (string, error) {
	if filepath.IsAbs(target) {
		return filepath.Clean(target), nil
	}
	if filepath.VolumeName(target) != "" {
		return "", errDocumentOutsideWorkspace
	}
	if goruntime.GOOS == windowsOS && len(target) != 0 && os.IsPathSeparator(target[0]) {
		volume := filepath.VolumeName(linkPath)
		if volume == "" {
			return "", errDocumentOutsideWorkspace
		}
		return filepath.Clean(volume + target), nil
	}
	return filepath.Clean(filepath.Join(filepath.Dir(linkPath), target)), nil
}

func unresolvedDocumentTail(
	candidate string,
	pending []string,
	roots []documentRoot,
) (string, error) {
	result := filepath.Join(append([]string{candidate}, pending...)...)
	if !pathWithinCanonicalRoot(result, roots) {
		return "", errDocumentOutsideWorkspace
	}
	return filepath.Clean(result), nil
}

func pathWithinCanonicalRoot(path string, roots []documentRoot) bool {
	for _, root := range roots {
		if pathWithinRoot(path, root.canonical) {
			return true
		}
	}
	return false
}

func lexicallyAuthorizedDocumentPath(path string, roots []documentRoot) bool {
	for _, root := range roots {
		if pathWithinRoot(path, root.lexical) || pathWithinRoot(path, root.canonical) {
			return true
		}
	}
	return false
}

func pathWithinRoot(path, root string) bool {
	relative, err := filepath.Rel(root, path)
	return err == nil && !pathLeavesRoot(relative)
}

func (w *documentWorkspace) CanonicalizeTextDocumentParams(
	raw json.RawMessage,
) (json.RawMessage, error) {
	var params map[string]json.RawMessage
	if err := json.Unmarshal(raw, &params); err != nil || params == nil {
		return nil, errors.New("textDocument request params must be an object")
	}
	var document map[string]json.RawMessage
	if err := json.Unmarshal(params[fieldTextDocument], &document); err != nil || document == nil {
		return nil, errors.New("textDocument request is missing textDocument")
	}
	var rawURI string
	if err := json.Unmarshal(document[fieldURI], &rawURI); err != nil || rawURI == "" {
		return nil, errors.New("textDocument request is missing a file URI")
	}
	canonicalURI, err := w.CanonicalURI(rawURI)
	if err != nil {
		return nil, err
	}
	encodedURI, _ := json.Marshal(canonicalURI)
	document[fieldURI] = encodedURI
	encodedDocument, _ := json.Marshal(document)
	params[fieldTextDocument] = encodedDocument
	return json.Marshal(params)
}

func localFileURIPath(raw string) (string, error) {
	parsed, err := url.Parse(raw)
	if err != nil || !strings.EqualFold(parsed.Scheme, "file") || parsed.Path == "" ||
		parsed.RawQuery != "" || parsed.Fragment != "" || parsed.User != nil || parsed.Port() != "" {
		return "", fmt.Errorf("invalid file document URI: %q", raw)
	}
	localPath, err := localPathFromFileURL(parsed)
	if err != nil {
		return "", fmt.Errorf("invalid file document URI %q: %w", raw, err)
	}
	return cleanAbsoluteFilesystemPath(localPath)
}

func localPathFromFileURL(parsed *url.URL) (string, error) {
	host := parsed.Hostname()
	if goruntime.GOOS != windowsOS {
		if host != "" && !strings.EqualFold(host, "localhost") {
			return "", errors.New("remote file URI hosts are unsupported")
		}
		return filepath.FromSlash(parsed.Path), nil
	}

	path := strings.ReplaceAll(parsed.Path, "/", `\`)
	if host != "" && !strings.EqualFold(host, "localhost") {
		return `\\` + host + path, nil
	}
	if len(path) >= 3 && path[0] == '\\' && path[2] == ':' {
		path = path[1:]
	}
	return path, nil
}

func canonicalFilesystemPath(path string) (string, error) {
	path, err := cleanAbsoluteFilesystemPath(path)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err == nil {
		return filepath.Clean(resolved), nil
	}
	if !errors.Is(err, fs.ErrNotExist) {
		return "", fmt.Errorf("resolve file document path: %w", err)
	}
	return resolveMissingFilesystemPath(path)
}

func cleanAbsoluteFilesystemPath(path string) (string, error) {
	path = filepath.Clean(path)
	if !filepath.IsAbs(path) {
		return "", errors.New("file document path must be absolute")
	}
	return path, nil
}

func resolveMissingFilesystemPath(path string) (string, error) {
	current := path
	var tail []string
	for {
		resolved, err := filepath.EvalSymlinks(current)
		if err == nil {
			for index := len(tail) - 1; index >= 0; index-- {
				resolved = filepath.Join(resolved, tail[index])
			}
			return filepath.Clean(resolved), nil
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return "", fmt.Errorf("resolve file document ancestor: %w", err)
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", fmt.Errorf("resolve file document path %q: %w", path, err)
		}
		tail = append(tail, filepath.Base(current))
		current = parent
	}
}

func pathLeavesRoot(relative string) bool {
	return relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator))
}
