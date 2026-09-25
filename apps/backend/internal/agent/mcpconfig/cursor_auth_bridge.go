package mcpconfig

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

const (
	cursorMCPAuthFilename        = "mcp-auth.json"
	cursorMCPAuthUnifiedFilename = "kandev-mcp-auth-unified.json"
	cursorTaskProjectMarker      = "kandev-tasks"
)

var cursorMCPAuthMutex sync.Mutex

type cursorMCPAuthSource struct {
	path    string
	modTime int64
	servers map[string]json.RawMessage
}

type cursorMCPAuthSnapshot struct {
	data       []byte
	hasSources bool
}

// DeriveCursorProjectSlug derives Cursor's project directory name from a path.
func DeriveCursorProjectSlug(workspacePath string) string {
	var slug strings.Builder
	previousWasSeparator := true
	for _, char := range workspacePath {
		if !isCursorSlugASCIIAlphaNumeric(char) {
			previousWasSeparator = true
			continue
		}
		if slug.Len() > 0 && previousWasSeparator {
			slug.WriteByte('-')
		}
		slug.WriteRune(char)
		previousWasSeparator = false
	}
	return slug.String()
}

func isCursorSlugASCIIAlphaNumeric(char rune) bool {
	return char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || char >= '0' && char <= '9'
}

// AggregateCursorMCPAuth publishes the latest valid project auth snapshot.
// It leaves the existing master unchanged when no eligible source is valid.
func AggregateCursorMCPAuth(cursorHome string, excludedWorkspaceRoots ...string) error {
	cursorMCPAuthMutex.Lock()
	defer cursorMCPAuthMutex.Unlock()

	snapshot, err := aggregateCursorMCPAuth(cursorHome, excludedWorkspaceRoots...)
	if err != nil || !snapshot.hasSources {
		return err
	}
	return publishCursorMCPAuth(cursorHome, snapshot.data)
}

// LinkCursorMCPAuth refreshes the shared snapshot and links a canonical
// workspace's Cursor project auth file to it when at least one source is valid.
func LinkCursorMCPAuth(workspacePath, cursorHome string, excludedWorkspaceRoots ...string) error {
	cursorMCPAuthMutex.Lock()
	defer cursorMCPAuthMutex.Unlock()
	return linkCursorMCPAuth(workspacePath, cursorHome, excludedWorkspaceRoots...)
}

// PrepareCursorMCPAuth applies the enabled or disabled bridge behavior for a
// local Cursor launch.
func PrepareCursorMCPAuth(workspacePath, cursorHome string, enabled bool, excludedWorkspaceRoots ...string) error {
	cursorMCPAuthMutex.Lock()
	defer cursorMCPAuthMutex.Unlock()
	if !enabled {
		return disableCursorMCPAuth(workspacePath, cursorHome)
	}
	return linkCursorMCPAuth(workspacePath, cursorHome, excludedWorkspaceRoots...)
}

func linkCursorMCPAuth(workspacePath, cursorHome string, excludedWorkspaceRoots ...string) error {
	workspaceRoots := append([]string{workspacePath}, excludedWorkspaceRoots...)
	snapshot, err := aggregateCursorMCPAuth(cursorHome, workspaceRoots...)
	if err != nil {
		return err
	}
	if !snapshot.hasSources {
		return nil
	}
	masterPath, err := cursorMasterPath(cursorHome)
	if err != nil {
		return err
	}
	if err := publishCursorMCPAuth(cursorHome, snapshot.data); err != nil {
		return err
	}
	destination, err := cursorProjectAuthPath(workspacePath, cursorHome, true)
	if err != nil {
		return err
	}
	return linkCursorAuthFile(destination, masterPath)
}

func disableCursorMCPAuth(workspacePath, cursorHome string) error {
	destination, err := cursorProjectAuthPath(workspacePath, cursorHome, false)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	masterPath, err := cursorMasterPath(cursorHome)
	if err != nil {
		return err
	}
	info, err := os.Lstat(destination)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil || info.Mode()&os.ModeSymlink == 0 {
		if err != nil {
			return err
		}
		return nil
	}
	target, err := os.Readlink(destination)
	if err != nil {
		return err
	}
	if !samePath(resolveLinkTarget(filepath.Dir(destination), target), masterPath) {
		return nil
	}
	return os.Remove(destination)
}

func aggregateCursorMCPAuth(cursorHome string, excludedWorkspaceRoots ...string) (cursorMCPAuthSnapshot, error) {
	projectsPath := filepath.Join(cursorHome, "projects")
	projectsInfo, err := os.Lstat(projectsPath)
	if errors.Is(err, os.ErrNotExist) {
		return cursorMCPAuthSnapshot{}, nil
	}
	if err != nil {
		return cursorMCPAuthSnapshot{}, err
	}
	if projectsInfo.Mode()&os.ModeSymlink != 0 || !projectsInfo.IsDir() {
		return cursorMCPAuthSnapshot{}, errors.New("cursor projects path is not a regular directory")
	}
	entries, err := os.ReadDir(projectsPath)
	if err != nil {
		return cursorMCPAuthSnapshot{}, err
	}
	excludedSlugs, err := cursorAuthExcludedProjectSlugs(excludedWorkspaceRoots)
	if err != nil {
		return cursorMCPAuthSnapshot{}, err
	}
	sources := collectCursorMCPAuthSources(projectsPath, entries, excludedSlugs)
	if len(sources) == 0 {
		return cursorMCPAuthSnapshot{}, nil
	}
	sort.Slice(sources, func(i, j int) bool {
		if sources[i].modTime != sources[j].modTime {
			return sources[i].modTime > sources[j].modTime
		}
		return sources[i].path < sources[j].path
	})
	servers := make(map[string]json.RawMessage)
	for _, source := range sources {
		for name, value := range source.servers {
			if _, exists := servers[name]; !exists {
				servers[name] = value
			}
		}
	}
	data, err := json.MarshalIndent(servers, "", "  ")
	if err != nil {
		return cursorMCPAuthSnapshot{}, err
	}
	return cursorMCPAuthSnapshot{data: append(data, '\n'), hasSources: true}, nil
}

func collectCursorMCPAuthSources(projectsPath string, entries []os.DirEntry, excludedSlugs []string) []cursorMCPAuthSource {
	sources := make([]cursorMCPAuthSource, 0, len(entries))
	for _, entry := range entries {
		if strings.Contains(entry.Name(), cursorTaskProjectMarker) || isCursorTaskProjectSlug(entry.Name(), excludedSlugs) {
			continue
		}
		projectPath := filepath.Join(projectsPath, entry.Name())
		projectInfo, err := os.Lstat(projectPath)
		if err != nil || !projectInfo.IsDir() {
			continue
		}
		source, ok := readCursorMCPAuthSource(filepath.Join(projectPath, cursorMCPAuthFilename))
		if ok {
			sources = append(sources, source)
		}
	}
	return sources
}

func cursorAuthExcludedProjectSlugs(workspaceRoots []string) ([]string, error) {
	slugs := make([]string, 0, len(workspaceRoots))
	for _, root := range workspaceRoots {
		if root == "" {
			continue
		}
		canonicalRoot, err := canonicalPathIncludingMissingSuffix(root)
		if err != nil {
			return nil, errors.New("could not resolve Cursor MCP auth exclusion root")
		}
		slug := DeriveCursorProjectSlug(canonicalRoot)
		if slug == "" {
			return nil, errors.New("cursor MCP auth exclusion root has an empty project slug")
		}
		slugs = append(slugs, slug)
	}
	return slugs, nil
}

func isCursorTaskProjectSlug(projectSlug string, taskRootSlugs []string) bool {
	for _, rootSlug := range taskRootSlugs {
		if projectSlug == rootSlug || strings.HasPrefix(projectSlug, rootSlug+"-") {
			return true
		}
	}
	return false
}

func readCursorMCPAuthSource(path string) (cursorMCPAuthSource, bool) {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() {
		return cursorMCPAuthSource{}, false
	}
	file, err := os.Open(path)
	if err != nil {
		return cursorMCPAuthSource{}, false
	}
	defer func() { _ = file.Close() }()
	openedInfo, err := file.Stat()
	if err != nil || !openedInfo.Mode().IsRegular() || !os.SameFile(info, openedInfo) {
		return cursorMCPAuthSource{}, false
	}
	data, err := io.ReadAll(file)
	if err != nil {
		return cursorMCPAuthSource{}, false
	}
	servers, valid := decodeCursorMCPAuth(data)
	if !valid {
		return cursorMCPAuthSource{}, false
	}
	return cursorMCPAuthSource{path: path, modTime: info.ModTime().UnixNano(), servers: servers}, true
}

func decodeCursorMCPAuth(data []byte) (map[string]json.RawMessage, bool) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) < 2 || trimmed[0] != '{' {
		return nil, false
	}
	var root map[string]json.RawMessage
	if err := json.Unmarshal(trimmed, &root); err != nil || root == nil {
		return nil, false
	}
	for _, server := range root {
		if !isJSONObject(server) {
			return nil, false
		}
	}
	return root, true
}

func isJSONObject(data []byte) bool {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) < 2 || trimmed[0] != '{' {
		return false
	}
	var object map[string]json.RawMessage
	return json.Unmarshal(trimmed, &object) == nil && object != nil
}

func publishCursorMCPAuth(cursorHome string, data []byte) error {
	masterPath, err := cursorMasterPath(cursorHome)
	if err != nil {
		return err
	}
	if err := ensureReplaceableMaster(masterPath); err != nil {
		return err
	}
	tempFile, err := os.CreateTemp(cursorHome, ".kandev-mcp-auth-*")
	if err != nil {
		return err
	}
	tempPath := tempFile.Name()
	defer func() { _ = os.Remove(tempPath) }()
	if err := tempFile.Chmod(0o600); err != nil {
		_ = tempFile.Close()
		return err
	}
	if _, err := tempFile.Write(data); err != nil {
		_ = tempFile.Close()
		return err
	}
	if err := tempFile.Close(); err != nil {
		return err
	}
	if err := ensureReplaceableMaster(masterPath); err != nil {
		return err
	}
	return os.Rename(tempPath, masterPath)
}

func ensureReplaceableMaster(path string) error {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return errors.New("cursor unified auth path is not a regular file")
	}
	return nil
}

func cursorProjectAuthPath(workspacePath, cursorHome string, create bool) (string, error) {
	canonicalPath, err := canonicalWorkspacePath(workspacePath)
	if err != nil {
		return "", err
	}
	slug := DeriveCursorProjectSlug(canonicalPath)
	if slug == "" {
		return "", errors.New("cursor project path has an empty slug")
	}
	projectsPath := filepath.Join(cursorHome, "projects")
	projectsInfo, err := os.Lstat(projectsPath)
	if err != nil {
		return "", err
	}
	if projectsInfo.Mode()&os.ModeSymlink != 0 || !projectsInfo.IsDir() {
		return "", errors.New("cursor projects path is not a regular directory")
	}
	projectPath, err := cursorProjectDirectoryPath(projectsPath, slug, create)
	if err != nil {
		return "", err
	}
	return filepath.Join(projectPath, cursorMCPAuthFilename), nil
}

func cursorProjectDirectoryPath(projectsPath, slug string, create bool) (string, error) {
	projectPath := filepath.Join(projectsPath, slug)
	projectInfo, err := os.Lstat(projectPath)
	if errors.Is(err, os.ErrNotExist) && create {
		if err := os.Mkdir(projectPath, 0o755); err != nil && !errors.Is(err, os.ErrExist) {
			return "", err
		}
		projectInfo, err = os.Lstat(projectPath)
	}
	if err != nil {
		return "", err
	}
	if projectInfo.Mode()&os.ModeSymlink != 0 || !projectInfo.IsDir() {
		return "", errors.New("cursor project path is not a regular directory")
	}
	return projectPath, nil
}

func canonicalWorkspacePath(workspacePath string) (string, error) {
	absolutePath, err := filepath.Abs(workspacePath)
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(absolutePath)
}

// canonicalPathIncludingMissingSuffix resolves symlinked ancestors when a task root has been removed.
func canonicalPathIncludingMissingSuffix(path string) (string, error) {
	absolutePath, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	for existingPath := absolutePath; ; existingPath = filepath.Dir(existingPath) {
		resolvedPath, resolveErr := filepath.EvalSymlinks(existingPath)
		if resolveErr == nil {
			suffix, relErr := filepath.Rel(existingPath, absolutePath)
			if relErr != nil {
				return "", relErr
			}
			return filepath.Join(resolvedPath, suffix), nil
		}
		if !errors.Is(resolveErr, os.ErrNotExist) {
			return "", resolveErr
		}
		parent := filepath.Dir(existingPath)
		if parent == existingPath {
			return "", resolveErr
		}
	}
}

func cursorMasterPath(cursorHome string) (string, error) {
	absoluteHome, err := filepath.Abs(cursorHome)
	if err != nil {
		return "", err
	}
	return filepath.Join(absoluteHome, cursorMCPAuthUnifiedFilename), nil
}

func linkCursorAuthFile(destination, masterPath string) error {
	info, err := os.Lstat(destination)
	if errors.Is(err, os.ErrNotExist) {
		return os.Symlink(masterPath, destination)
	}
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink == 0 {
		return nil
	}
	target, err := os.Readlink(destination)
	if err != nil {
		return err
	}
	if samePath(resolveLinkTarget(filepath.Dir(destination), target), masterPath) {
		return nil
	}
	return replaceCursorAuthSymlink(destination, masterPath)
}

func replaceCursorAuthSymlink(destination, masterPath string) error {
	file, err := os.CreateTemp(filepath.Dir(destination), ".kandev-mcp-auth-link-*")
	if err != nil {
		return err
	}
	tempPath := file.Name()
	if err := file.Close(); err != nil {
		_ = os.Remove(tempPath)
		return err
	}
	if err := os.Remove(tempPath); err != nil {
		return err
	}
	defer func() { _ = os.Remove(tempPath) }()
	if err := os.Symlink(masterPath, tempPath); err != nil {
		return err
	}
	info, err := os.Lstat(destination)
	if errors.Is(err, os.ErrNotExist) {
		return os.Rename(tempPath, destination)
	}
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink == 0 {
		return nil
	}
	return os.Rename(tempPath, destination)
}

func resolveLinkTarget(linkDirectory, target string) string {
	if !filepath.IsAbs(target) {
		target = filepath.Join(linkDirectory, target)
	}
	absolute, err := filepath.Abs(target)
	if err != nil {
		return filepath.Clean(target)
	}
	return filepath.Clean(absolute)
}

func samePath(left, right string) bool {
	return filepath.Clean(left) == filepath.Clean(right)
}
