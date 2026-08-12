package lsp

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"path/filepath"
	goruntime "runtime"
	"strings"
	"sync"
)

var errDocumentOutsideWorkspace = errors.New("document URI is outside the task workspace")

// documentWorkspace authorizes browser-originated document URIs against the
// task workspace projection captured by the task-host generation.
type documentWorkspace struct {
	mu          sync.RWMutex
	roots       []documentRoot
	resolvePath func(string) (string, error)
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
	return &documentWorkspace{resolvePath: resolvePath}
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
		canonical, err := w.resolvePath(lexical)
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
	canonicalPath, err := w.resolvePath(documentPath)
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
	if goruntime.GOOS != "windows" {
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
